package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wahhheee/yanyi/internal/pkg/fsx"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migration 是一个结构迁移脚本：migrations/0001_init.sql → {1, "init"}。
type migration struct {
	version int
	name    string
	sql     string
}

// Migrate 把数据库结构升级到当前程序所需的版本。
//
//   - 新库：直接执行全部脚本；
//   - 旧库有待执行的迁移：先用 VACUUM INTO 备份到 backupDir（保留最近 3 份），再逐个执行；
//   - 库的版本比程序新（用旧程序打开新数据）：拒绝启动，避免破坏数据。
//
// 新增迁移：在 migrations/ 下添加 NNNN_说明.sql，编号递增，已发布的脚本不得修改。
func (db *DB) Migrate(ctx context.Context, backupDir string) error {
	all, err := loadMigrations()
	if err != nil {
		return err
	}
	if _, err := db.write.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("创建迁移记录表失败: %w", err)
	}

	var current int
	if err := db.write.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("读取结构版本失败: %w", err)
	}
	latest := all[len(all)-1].version
	if current > latest {
		return fmt.Errorf("数据目录的结构版本（%d）比当前程序支持的版本（%d）新：请使用更新版本的 yanyi 打开", current, latest)
	}

	pending := slices.DeleteFunc(slices.Clone(all), func(m migration) bool { return m.version <= current })
	if len(pending) == 0 {
		return nil
	}
	if current > 0 {
		if err := db.backup(ctx, backupDir, current); err != nil {
			return fmt.Errorf("迁移前备份失败（为安全起见已停止迁移）: %w", err)
		}
	}
	for _, m := range pending {
		slog.InfoContext(ctx, "执行数据库迁移", "version", m.version, "name", m.name)
		err := db.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, m.sql); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
				m.version, m.name, NowMillis())
			return err
		})
		if err != nil {
			return fmt.Errorf("迁移 %04d_%s 失败: %w", m.version, m.name, err)
		}
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		name := e.Name()
		num, rest, ok := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil {
			return nil, fmt.Errorf("迁移文件名不合法：%s（应为 NNNN_说明.sql）", name)
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: rest, sql: string(body)})
	}
	slices.SortFunc(out, func(a, b migration) int { return a.version - b.version })
	for i, m := range out {
		if m.version != i+1 {
			return nil, fmt.Errorf("迁移编号不连续：期望 %04d，实际 %04d", i+1, m.version)
		}
	}
	return out, nil
}

// backup 用 VACUUM INTO 生成一致性快照（WAL 模式下也安全），只保留最近 3 份。
func (db *DB) backup(ctx context.Context, dir string, version int) error {
	if err := os.MkdirAll(dir, fsx.DirPerm); err != nil {
		return err
	}
	name := fmt.Sprintf("yanyi-v%04d-%s.db", version, time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if _, err := db.write.ExecContext(ctx, `VACUUM INTO ?`, dst); err != nil {
		return err
	}
	slog.InfoContext(ctx, "已备份数据库", "path", dst)

	matches, _ := filepath.Glob(filepath.Join(dir, "yanyi-v*.db"))
	slices.SortFunc(matches, func(a, b string) int {
		ia, _ := os.Stat(a)
		ib, _ := os.Stat(b)
		if ia == nil || ib == nil {
			return 0
		}
		return ia.ModTime().Compare(ib.ModTime())
	})
	for len(matches) > 3 {
		_ = os.Remove(matches[0])
		matches = matches[1:]
	}
	return nil
}
