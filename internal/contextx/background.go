package contextx

import (
	"bytes"
	"text/template"

	"github.com/wahhheee/yanyi/internal/prompts"
)

// RenderBackgroundPrompt 按当前模型能力渲染助手或标题提示词，不携带主笔对话。
func RenderBackgroundPrompt(source string, features prompts.FeatureSet, data any) (string, error) {
	t, err := template.New("background").Funcs(template.FuncMap{"has": func(f string) bool { return features.Has(prompts.Feature(f)) }}).Parse(source)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}
