package work

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

func TestJSONSchemas(t *testing.T) {
	compiled := map[string]*jsonschema.Schema{}
	for name, definition := range JSONSchemas() {
		b, err := jsonx.MarshalIndent(definition)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := os.ReadFile(filepath.Join("../../schemas", name+".schema.json"))
		if err != nil || !bytes.Equal(stored, append(b, '\n')) {
			t.Fatalf("%s 定义未同步，请运行 go run ./cmd/schema-gen：%v", name, err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		c := jsonschema.NewCompiler()
		id := definition["$id"].(string)
		if err := c.AddResource(id, doc); err != nil {
			t.Fatal(err)
		}
		compiled[name], err = c.Compile(id)
		if err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for _, root := range []string{"../../examples/works", "../../testdata/works"} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			name := strings.TrimSuffix(d.Name(), ".yaml")
			b, err := os.ReadFile(p) //nolint:gosec // 测试仅读取仓库内受版本控制的示例文件，不处理外部目录。
			if err != nil {
				return err
			}
			if strings.HasSuffix(p, ".md") {
				switch {
				case strings.Contains(filepath.ToSlash(p), "/lore/"):
					name = "lore-frontmatter"
				case strings.Contains(filepath.ToSlash(p), "/commands/"):
					name = "command-frontmatter"
				case strings.Contains(filepath.ToSlash(p), "/memory/"):
					name = "memory-frontmatter"
				case strings.Contains(filepath.ToSlash(p), "/prompts/openings/"):
					name = "opening-frontmatter"
				default:
					return nil
				}
				b, _, _ = splitFrontmatter(b)
				if len(b) == 0 {
					return nil
				}
			}
			schema := compiled[name]
			if schema == nil {
				return nil
			}
			var v any
			if err := yaml.Unmarshal(b, &v); err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			raw, err := jsonx.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			v, err = jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(v); err != nil {
				t.Errorf("%s: %v", p, err)
			}
			count++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if count < 12 {
		t.Fatalf("校验覆盖不足：%d", count)
	}
	for name, raw := range map[string]string{"work": `{"format":1,"id":"bad/id","title":"标题","version":"1.0.0"}`, "state": `{"fields":{"bad":{"type":"imaginary"}}}`, "layout": `{"panels":[{"id":"a","template":"a","place":"invalid"}]}`} {
		var v any
		if err := jsonx.Unmarshal([]byte(raw), &v); err != nil {
			t.Fatal(err)
		}
		if compiled[name].Validate(v) == nil {
			t.Errorf("%s 未拒绝非法结构", name)
		}
	}
}
