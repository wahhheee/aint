package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/wahhheee/yanyi/internal/vfs"
)

func memoryEnv() *fakeEnv {
	o := vfs.NewOverlay(map[string]vfs.FileRef{}, 3)
	src := vfs.NewMemorySource(o, func(context.Context, string) ([]byte, error) { return nil, nil })
	return &fakeEnv{fs: vfs.NewTable(vfs.Mount{Root: "/memory", Title: "记忆", Source: src})}
}

func TestMemoryTools(t *testing.T) {
	env := memoryEnv()

	if got := run(t, handleWrite, env, `{"path":"/memory/characters/lin.md","content":"# 林舟\n信任：低\n信任：低\n"}`); got != "# write /memory/characters/lin.md\n已新建（3 行）" {
		t.Fatalf("write：%q", got)
	}
	if got := run(t, handleWrite, env, `{"path":"/memory/characters/lin.md","content":"# 林舟\n信任：低\n信任：低\n"}`); got != "# write /memory/characters/lin.md\n已覆盖（3 行）" {
		t.Fatalf("覆盖：%q", got)
	}
	if got := run(t, handleEdit, env, `{"path":"/memory/characters/lin.md","old_text":"信任：低","new_text":"信任：中"}`); !strings.HasPrefix(got, "未执行：old_text 在 /memory/characters/lin.md 中出现了 2 次") {
		t.Fatalf("多处匹配：%q", got)
	}
	if got := run(t, handleEdit, env, `{"path":"/memory/characters/lin.md","old_text":"信任：低","new_text":"信任：中","replace_all":true}`); got != "# edit /memory/characters/lin.md\n已替换 2 处（现共 3 行）" {
		t.Fatalf("全部替换：%q", got)
	}
	if got := run(t, handleEdit, env, `{"path":"/memory/characters/lin.md","old_text":"不存在","new_text":"x"}`); !strings.HasPrefix(got, "未执行：/memory/characters/lin.md 中没有找到 old_text") {
		t.Fatalf("没有匹配：%q", got)
	}
	if got := run(t, handleEdit, env, `{"path":"/memory/characters/lin.md","old_text":"# 林舟\r\n信任：中","new_text":"# 林舟（邮差）"}`); got != "# edit /memory/characters/lin.md\n已替换 1 处（现共 2 行）" {
		t.Fatalf("CRLF 统一为 LF：%q", got)
	}
	if got := run(t, handleRead, env, `{"path":"/memory/characters/lin.md"}`); got != "# read /memory/characters/lin.md（第 1–2 行，共 2 行）\n# 林舟（邮差）\n信任：中" {
		t.Fatalf("read：%q", got)
	}
	if got := run(t, handleWrite, env, `{"path":"/work/x.md","content":"x"}`); !strings.HasPrefix(got, "未执行：/work/x.md") {
		t.Fatalf("只读：%q", got)
	}
	if got := run(t, handleWrite, env, `{"path":"/memory/x.json","content":"x"}`); got != "未执行：/memory/x.json：记忆文件可以用 .md、.txt 或 .yaml 扩展名。" {
		t.Fatalf("扩展名：%q", got)
	}
	if got := run(t, handleRemove, env, `{"path":"/memory/characters/lin.md"}`); got != "# remove /memory/characters/lin.md\n已删除" {
		t.Fatalf("remove：%q", got)
	}
	if got := run(t, handleRemove, env, `{"path":"/memory/characters/lin.md"}`); !strings.HasPrefix(got, "未执行：/memory/characters/lin.md 不存在") {
		t.Fatalf("删除不存在的文件：%q", got)
	}
}
