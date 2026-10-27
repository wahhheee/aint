package runtime_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/work"
)

func TestDeleteWorkWithSaves(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(map[bool]string{false: "删除全部存档与分支", true: "失败时完整回滚"}[rollback], func(t *testing.T) {
			e := setup(t, quickScript())
			ctx := context.Background()
			old := upgradeVersion(t, e, "1.0.0", oldUpgradeState, true)
			versions := []string{old.VersionID, upgradeVersion(t, e, "2.0.0", newUpgradeState, true).VersionID}
			for _, version := range versions {
				sv, err := e.saves.Create(ctx, save.CreateParams{WorkID: old.WorkID, VersionID: version})
				if err != nil {
					t.Fatal(err)
				}
				for _, text := range []string{"分支一", "分支二"} {
					if _, err := e.saves.EditMemory(ctx, sv.ID, "/memory/branch.md", &text); err != nil {
						t.Fatal(err)
					}
					if _, err := e.saves.SetHead(ctx, sv.ID, sv.HeadNodeID, save.HeadHere); err != nil {
						t.Fatal(err)
					}
				}
			}
			other, err := e.saves.Create(ctx, save.CreateParams{WorkID: "aint.misty-harbor"})
			if err != nil {
				t.Fatal(err)
			}
			counts := func() []int {
				t.Helper()
				queries := []string{
					`SELECT COUNT(*) FROM works WHERE id = ?`,
					`SELECT COUNT(*) FROM work_versions WHERE work_id = ?`,
					`SELECT COUNT(*) FROM work_files WHERE version_id IN (SELECT id FROM work_versions WHERE work_id = ?)`,
					`SELECT COUNT(*) FROM saves WHERE work_id = ?`,
					`SELECT COUNT(*) FROM nodes WHERE save_id != ?`,
					`SELECT COUNT(*) FROM transcript_fts WHERE save_id != ?`,
				}
				result := make([]int, len(queries))
				for i, query := range queries {
					id := old.WorkID
					if i >= 4 {
						id = other.ID
					}
					if err := e.db.Read().QueryRowContext(ctx, query, id).Scan(&result[i]); err != nil {
						t.Fatal(err)
					}
				}
				return result
			}
			before := counts()
			for _, n := range before {
				if n == 0 {
					t.Fatal("测试数据缺少作品、存档、分支或原文索引", before)
				}
			}
			if rollback {
				if err := e.db.Tx(ctx, func(tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx, `CREATE TRIGGER reject_work_delete BEFORE DELETE ON works BEGIN SELECT RAISE(ABORT, '测试删除失败'); END`)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			works := work.NewService(e.db, e.blobs)
			err = e.mgr.WhenIdle(func() error { return works.Delete(ctx, old.WorkID, "") })
			if (err != nil) != rollback {
				t.Fatal("删除结果不符", err)
			}
			for i, got := range counts() {
				want := 0
				if rollback {
					want = before[i]
				}
				if got != want {
					t.Fatalf("第 %d 项记录数 = %d，期望 %d", i, got, want)
				}
			}
			if _, _, err := e.saves.ProgressLine(ctx, other.ID); err != nil {
				t.Fatal("其他作品的存档受到影响", err)
			}
		})
	}
}
