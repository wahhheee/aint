// Package store 是持久化基础设施：SQLite 连接、事务、结构迁移、内容寻址 blob。
//
// 分工约定：
//   - 本包只负责"怎么存"，不认识作品、存档等业务概念；
//   - 表结构（DDL）集中在 migrations/*.sql，是全库结构的唯一事实来源；
//   - 各业务包在自己的 repo.go 中编写 SQL（如 work/repo.go、save/repo.go），
//     通过 Querier 接口同时支持"直接查询"与"在事务中执行"。
//
// 并发模型（依赖"同一数据目录单实例运行"的保证，见 app/instance.go）：
//   - 写：单连接池 + BEGIN IMMEDIATE，所有写事务天然串行，不会出现 SQLITE_BUSY 竞争；
//   - 读：多连接池 + query_only，WAL 模式下读写互不阻塞；误在读连接上写会直接报错，便于发现问题。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite" // 纯 Go 的 SQLite 驱动，支持 CGO_ENABLED=0 交叉编译
)

// Querier 是 *sql.DB 与 *sql.Tx 的公共子集。repo 函数接受它，调用方决定是否在事务中执行。
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DB 封装读写两个连接池。
type DB struct {
	path  string
	write *sql.DB
	read  *sql.DB
}

// Open 打开（必要时创建）数据库。调用方随后应调用 Migrate。
func Open(ctx context.Context, path string) (*DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// synchronous=FULL：写入频率低（只在回合提交时写），换取断电持久性。
	common := "_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	base := "file:" + filepath.ToSlash(abs) + "?" + common

	write, err := sql.Open("sqlite", base+"&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("打开数据库（写）失败: %w", err)
	}
	write.SetMaxOpenConns(1)
	write.SetConnMaxIdleTime(0)

	read, err := sql.Open("sqlite", base+"&_pragma=query_only(1)")
	if err != nil {
		_ = write.Close()
		return nil, fmt.Errorf("打开数据库（读）失败: %w", err)
	}
	read.SetMaxOpenConns(max(4, runtime.NumCPU()))

	db := &DB{path: abs, write: write, read: read}
	if err := write.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("数据库不可用（%s）: %w", abs, err)
	}
	return db, nil
}

// Path 返回数据库文件的绝对路径。
func (db *DB) Path() string { return db.path }

// Read 返回只读连接池，用于查询。
func (db *DB) Read() *sql.DB { return db.read }

// Tx 在写事务中执行 fn：fn 返回 nil 则提交，否则回滚。
// 事务内不要做网络请求或长时间计算（会阻塞所有写入）；大 blob 文件应在事务前写好。
func (db *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	start := time.Now()
	tx, err := db.write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		if err = tx.Commit(); err != nil {
			err = fmt.Errorf("提交事务失败: %w", err)
		}
		if d := time.Since(start); d > 200*time.Millisecond {
			slog.WarnContext(ctx, "写事务耗时较长", "duration", d)
		}
	}()
	return fn(tx)
}

// Close 关闭连接池。
func (db *DB) Close() error {
	var first error
	for _, c := range []*sql.DB{db.read, db.write} {
		if c == nil {
			continue
		}
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// NowMillis 返回当前 Unix 毫秒时间戳。库中所有时间列统一使用 Unix 毫秒。
func NowMillis() int64 { return time.Now().UnixMilli() }
