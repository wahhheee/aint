package fake

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"

	"github.com/wahhheee/aint/internal/llm"
)

// Script 是假模型脚本。
//
//	responses:
//	  - text: |            # 流式输出的正文，可含叙事标记；{{input}} 替换为玩家最近一次输入
//	      雾是从傍晚开始漫上来的……
//	    chunk_runes: 3       # 每个增量的字数（默认 4）
//	    delay_ms: 25         # 增量间隔（默认 30）
//	    first_delay_ms: 600  # 首字延迟（默认 0），用于观察"等待首个字"的界面
//	    tool_calls:          # 正文之后发出的工具调用
//	      - name: offer_choices
//	        args: {choices: [{id: a, label: 推开门}]}
//	    stop: tool_use       # 缺省：有工具调用为 tool_use，否则 end；可设 max_tokens 测续写
//	  - error: rate_limited  # 模拟错误（llm.ErrorKind），用于测试重试与错误提示
//	ooc:                     # 可选：玩家场外发言（<ooc>）时使用，不计入 responses 的循环
//	  text: "<ooc>收到。</ooc>"
//	compact:                 # 可选：上下文整理（请求用途为 compact）时依次使用，不计入 responses 的循环
//	  - tool_calls: [{name: write, args: {path: /memory/chronicle/01.md, content: "……"}}]
//	  - text: "## 故事至今 ……"   # 最后一项是交接摘要；整理中的调用次数超过列表长度时一直用最后一项
//
// 每个存档独立计数，依次循环使用 responses。正文与工具参数中的字符串都可以写 {{input}}。
type Script struct {
	Responses []Response `yaml:"responses"`
	OOC       *Response  `yaml:"ooc"`
	Compact   []Response `yaml:"compact"`
}

// Response 是一次调用的脚本。
type Response struct {
	Text         string     `yaml:"text"`
	ChunkRunes   int        `yaml:"chunk_runes"`
	DelayMS      int        `yaml:"delay_ms"`
	FirstDelayMS int        `yaml:"first_delay_ms"`
	ToolCalls    []ToolCall `yaml:"tool_calls"`
	Stop         string     `yaml:"stop"`
	Error        string     `yaml:"error"`
	Usage        *llm.Usage `yaml:"usage"`
}

// ToolCall 是脚本中的工具调用。
type ToolCall struct {
	Name string         `yaml:"name"`
	Args map[string]any `yaml:"args"`
}

func (r Response) chunk() int {
	if r.ChunkRunes > 0 {
		return r.ChunkRunes
	}
	return 4
}

func (r Response) delay() int {
	if r.DelayMS > 0 {
		return r.DelayMS
	}
	return 30
}

func (r Response) stop() llm.StopReason {
	if r.Stop != "" {
		return llm.StopReason(r.Stop)
	}
	if len(r.ToolCalls) > 0 {
		return llm.StopToolUse
	}
	return llm.StopEnd
}

//go:embed demo.yaml
var demoScript []byte

// LoadScript 加载脚本。name 为 "demo" 或空时使用内置演示脚本，否则视为文件路径。
func LoadScript(name string) (*Script, error) {
	data := demoScript
	if name != "" && name != "demo" {
		var err error
		if data, err = os.ReadFile(name); err != nil {
			return nil, fmt.Errorf("读取假模型脚本失败: %w", err)
		}
	}
	var s Script
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("解析假模型脚本失败:\n%s", yaml.FormatError(err, false, true))
	}
	if len(s.Responses) == 0 {
		return nil, fmt.Errorf("假模型脚本 %q 中没有 responses", name)
	}
	return &s, nil
}
