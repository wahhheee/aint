package work

import (
	"reflect"
	"strings"
)

type schemaObject = map[string]any

// JSONSchemas 返回全部作品 YAML 与 Markdown frontmatter 的编辑器定义。
// 字段类型与编译器共用输入结构；跨文件引用、模板与素材的检查由 work validate 完成。
func JSONSchemas() map[string]schemaObject {
	manifest := schemaOf(reflect.TypeFor[Manifest]())
	manifest["required"] = []string{"format", "id", "version", "title"}
	prop(manifest, "format")["const"] = 1
	prop(manifest, "id")["pattern"] = reWorkID.String()
	prop(manifest, "version")["pattern"] = `^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`
	prop(manifest, "title")["minLength"] = 1
	enum(prop(manifest, "content_rating"), "general", "teen", "mature")
	enum(prop(manifest, "requires", "vision"), "none", "preferred", "required")
	enum(prop(manifest, "narrator", "reasoning"), "off", "low", "medium", "high")
	enum(prop(manifest, "opening", "mode"), "static", "generate")
	enum(prop(manifest, "presentation", "dialogue"), "plain", "tagged")
	enum(prop(manifest, "docs", "default_load"), "always", "index", "none")
	form := prop(manifest, "new_game", "fields")["items"].(schemaObject)
	form["required"] = []string{"key", "label", "type"}
	enum(prop(form, "type"), "text", "textarea", "number", "select", "multiselect", "toggle")
	doc := schemaOf(reflect.TypeFor[docFront]())
	enum(prop(doc, "load"), "always", "index", "none")
	command := schemaOf(reflect.TypeFor[cmdFront]())
	enum(prop(command, "present")["items"].(schemaObject), "slash", "mode", "button")
	layout := schemaOf(reflect.TypeFor[Layout]())
	panel := prop(layout, "panels")["items"].(schemaObject)
	panel["required"] = []string{"id", "template"}
	enum(prop(panel, "place"), "sidebar", "topbar", "drawer", "float")
	enum(prop(panel, "format"), "markdown", "html")
	assets := schemaOf(reflect.TypeFor[[]meta]())
	asset := assets["items"].(schemaObject)
	asset["required"] = []string{"path"}
	enum(prop(asset, "role"), "background", "cg", "avatar", "bgm", "sfx", "font", "reference")
	tag := schemaOf(reflect.TypeFor[tagRaw]())
	enum(prop(tag, "style", "weight"), "normal", "bold")
	enum(prop(tag, "style", "underline"), "none", "solid", "dotted", "wavy")
	enum(prop(tag, "style", "effect"), "glow", "shake", "flicker", "fade-in", "blur-reveal", "whisper", "glitch")
	markup := object(schemaObject{"tags": schemaObject{"type": "object", "additionalProperties": tag}})
	opening := object(schemaObject{"choices": schemaOf(reflect.TypeFor[[]Choice]())})
	choice := prop(opening, "choices")["items"].(schemaObject)
	choice["required"] = []string{"id", "label"}
	theme := schemaOf(reflect.TypeFor[themeFile]())
	prop(theme, "background", "opacity")["minimum"] = 0
	prop(theme, "background", "opacity")["maximum"] = 1
	stateTypes := schemaObject{"enum": []string{"boolean", "number", "string", "list", "map", "object"}}
	field := object(schemaObject{
		"type": stateTypes, "default": schemaObject{}, "desc": schemaObject{"type": "string"},
		"items": schemaObject{"$ref": "#/$defs/field"}, "values": schemaObject{"$ref": "#/$defs/field"},
		"fields": schemaObject{"type": "object", "additionalProperties": schemaObject{"$ref": "#/$defs/field"}},
	})
	field["required"] = []string{"type"}
	stateDoc := object(schemaObject{"fields": schemaObject{"type": "object", "additionalProperties": schemaObject{"$ref": "#/$defs/field"}}})
	stateDoc["required"] = []string{"fields"}
	stateDoc["$defs"] = schemaObject{"field": schemaObject{"anyOf": []any{stateTypes, field}}}
	schemas := map[string]schemaObject{
		"work": manifest, "state": stateDoc, "assets": assets, "layout": layout, "markup": markup, "theme": theme,
		"lore-frontmatter": doc, "command-frontmatter": command, "opening-frontmatter": opening,
		"memory-frontmatter": object(schemaObject{"summary": schemaObject{"type": "string"}}),
	}
	for name, schema := range schemas {
		schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"
		schema["$id"] = "https://raw.githubusercontent.com/wahhheee/yanyi/main/schemas/" + name + ".schema.json"
		schema["title"] = "演义作品：" + name
		schema["description"] = "编辑器补全与结构校验；完整校验请运行 yanyi work validate。未知字段由运行时报告警告。"
	}
	return schemas
}

func object(properties schemaObject) schemaObject {
	return schemaObject{"type": "object", "properties": properties, "additionalProperties": true}
}
func enum(schema schemaObject, values ...string) { schema["enum"] = values }
func prop(schema schemaObject, keys ...string) schemaObject {
	for _, key := range keys {
		schema = schema["properties"].(schemaObject)[key].(schemaObject)
	}
	return schema
}

func schemaOf(t reflect.Type) schemaObject {
	if t.Kind() == reflect.Pointer {
		return schemaOf(t.Elem())
	}
	switch t.Kind() {
	case reflect.String:
		return schemaObject{"type": "string"}
	case reflect.Bool:
		return schemaObject{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return schemaObject{"type": "integer"}
	case reflect.Float64:
		return schemaObject{"type": "number"}
	case reflect.Slice:
		return schemaObject{"type": "array", "items": schemaOf(t.Elem())}
	case reflect.Map:
		return schemaObject{"type": "object", "additionalProperties": schemaOf(t.Elem())}
	case reflect.Struct:
		properties := schemaObject{}
		for i := range t.NumField() {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("yaml"), ",")[0]
			if name == "" || name == "-" {
				continue
			}
			properties[name] = schemaOf(f.Type)
		}
		return object(properties)
	default:
		return schemaObject{}
	}
}
