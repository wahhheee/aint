package work

import "fmt"

// Level 是校验问题的级别：error 阻止导入，warning 可继续，info 仅供参考。
type Level string

const (
	LevelError   Level = "error"
	LevelWarning Level = "warning"
	LevelInfo    Level = "info"
)

// 校验代码（《03-作品格式规范》第 17 节）。新增代码时同步规范文档与前端文案。
const (
	CodeManifest     = "E-MANIFEST"
	CodeEngine       = "E-ENGINE"
	CodeRef          = "E-REF"
	CodeTemplate     = "E-TEMPLATE"
	CodeName         = "E-NAME"
	CodeState        = "E-STATE"
	CodeLFS          = "E-LFS"
	CodeAsset        = "E-ASSET"
	CodeTheme        = "E-THEME" // ui/theme.yaml 取值或 custom.css 不合法
	CodeParse        = "E-PARSE" // YAML / frontmatter 语法错误（规范中未单列，归入对应文件）
	CodeUnknownField = "W-UNKNOWN-FIELD"
	CodePrefixLarge  = "W-PREFIX-LARGE"
	CodeIndexLarge   = "W-INDEX-LARGE"
	CodePinImage     = "W-PIN-IMAGE"
	CodeNoDesc       = "W-NO-DESC"
	CodeTokens       = "I-TOKENS"
)

// Issue 是一条校验问题。每条尽量带文件路径与行号，便于创作者定位。
type Issue struct {
	Level   Level  `json:"level"`
	Code    string `json:"code"`
	Path    string `json:"path,omitzero"`
	Line    int    `json:"line,omitzero"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitzero"`
}

func (i Issue) String() string {
	loc := i.Path
	if i.Line > 0 {
		loc = fmt.Sprintf("%s:%d", i.Path, i.Line)
	}
	s := fmt.Sprintf("[%s] %s %s", i.Code, loc, i.Message)
	if i.Hint != "" {
		s += "（" + i.Hint + "）"
	}
	return s
}

// Report 是校验报告。
type Report struct {
	Issues []Issue `json:"issues"`
	// TODO(M2-12)：Tokens 各层 token 估算（I-TOKENS），按实际启用的提示词片段计算。
}

func (r *Report) add(level Level, code, path string, line int, hint, format string, args ...any) {
	r.Issues = append(r.Issues, Issue{
		Level: level, Code: code, Path: path, Line: line,
		Message: fmt.Sprintf(format, args...), Hint: hint,
	})
}

// Errorf 添加 error 级问题。
func (r *Report) Errorf(code, path string, line int, hint, format string, args ...any) {
	r.add(LevelError, code, path, line, hint, format, args...)
}

// Warnf 添加 warning 级问题。
func (r *Report) Warnf(code, path string, line int, hint, format string, args ...any) {
	r.add(LevelWarning, code, path, line, hint, format, args...)
}

// HasErrors 报告中是否有 error。
func (r *Report) HasErrors() bool {
	for _, i := range r.Issues {
		if i.Level == LevelError {
			return true
		}
	}
	return false
}

// Count 返回各级别的问题数。
func (r *Report) Count() (errors, warnings int) {
	for _, i := range r.Issues {
		switch i.Level {
		case LevelError:
			errors++
		case LevelWarning:
			warnings++
		}
	}
	return
}
