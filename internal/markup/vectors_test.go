package markup

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// 共享测试向量（testdata/markup/*.json，格式见 testdata/README.md）：前端 parse.test.ts 运行同一份。

type vectorFile struct {
	Description string       `json:"description"`
	Tags        []string     `json:"tags"`
	Drafting    bool         `json:"drafting"`
	Cases       []vectorCase `json:"cases"`
}

type vectorCase struct {
	Name     string   `json:"name"`
	Input    any      `json:"input"` // 字符串，或按行给出的字符串数组（以 "\n" 连接）
	Tags     []string `json:"tags"`
	Drafting *bool    `json:"drafting"`
	AST      any      `json:"ast"`
	Plain    any      `json:"plain"`
}

func loadVectors(t *testing.T) map[string]vectorFile {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "markup", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("没有找到测试向量：%v", err)
	}
	files := map[string]vectorFile{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var f vectorFile
		if err := jsonx.UnmarshalStrict(bytes.NewReader(data), &f); err != nil {
			t.Fatalf("%s：%v", p, err)
		}
		files[filepath.Base(p)] = f
	}
	return files
}

func joinLines(t *testing.T, v any) string {
	t.Helper()
	switch v := v.(type) {
	case string:
		return v
	case []any:
		lines := make([]string, len(v))
		for i, l := range v {
			s, ok := l.(string)
			if !ok {
				t.Fatalf("按行给出的文本只能是字符串：%v", l)
			}
			lines[i] = s
		}
		return strings.Join(lines, "\n")
	}
	t.Fatalf("文本应为字符串或字符串数组：%v", v)
	return ""
}

func TestVectors(t *testing.T) {
	for file, f := range loadVectors(t) {
		for _, c := range f.Cases {
			t.Run(file+"/"+c.Name, func(t *testing.T) {
				tags := f.Tags
				if c.Tags != nil {
					tags = c.Tags
				}
				opts := Options{Tags: map[string]bool{}, Drafting: f.Drafting}
				for _, name := range tags {
					opts.Tags[name] = true
				}
				if c.Drafting != nil {
					opts.Drafting = *c.Drafting
				}
				src := joinLines(t, c.Input)
				blocks := Parse(src, opts)
				if got := compactBlocks(blocks); !reflect.DeepEqual(got, c.AST) {
					t.Errorf("输入 %q\n得到 %s\n期望 %s", src, mustJSON(got), mustJSON(c.AST))
				}
				if c.Plain != nil {
					if got, want := Plain(blocks), joinLines(t, c.Plain); got != want {
						t.Errorf("纯文本 %q，期望 %q", got, want)
					}
				}
			})
		}
	}
}

// compactBlocks 把 AST 转为向量中的紧凑写法（见 testdata/README.md）。
func compactBlocks(blocks []Node) []any {
	out := make([]any, 0, len(blocks))
	for _, b := range blocks {
		e := []any{string(b.Type)}
		switch b.Type {
		case NodeScene, NodeCG:
			e = appendAttrs(e, b.Attrs)
		case NodeOOC:
			e = append(e, compactBlocks(b.Children)...)
		default:
			e = append(e, compactInline(b.Children)...)
		}
		out = append(out, e)
	}
	return out
}

func compactInline(nodes []Node) []any {
	out := make([]any, 0, len(nodes))
	for _, n := range nodes {
		switch n.Type {
		case NodeText:
			out = append(out, n.Text)
		case NodeRaw:
			out = append(out, []any{"raw", n.Text})
		case NodeEm:
			out = append(out, append([]any{"*"}, compactInline(n.Children)...))
		case NodeStrong:
			out = append(out, append([]any{"**"}, compactInline(n.Children)...))
		case NodeTag:
			e := appendAttrs([]any{"<" + n.Name + ">"}, n.Attrs)
			out = append(out, append(e, compactInline(n.Children)...))
		default:
			out = append(out, []any{"?" + string(n.Type)})
		}
	}
	return out
}

func appendAttrs(e []any, attrs map[string]string) []any {
	if len(attrs) == 0 {
		return e
	}
	m := make(map[string]any, len(attrs))
	for k, v := range attrs {
		m[k] = v
	}
	return append(e, m)
}

func mustJSON(v any) string {
	b, err := jsonx.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(b)
}
