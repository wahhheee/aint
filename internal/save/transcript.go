package save

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/wahhheee/aint/internal/markup"
)

// 原文（《02-技术方案》12.4）：每个回合一个虚拟文件 /transcript/NNNN，内容是
//
//	# 第 42 回合
//	> 玩家（说话）：你昨晚去哪了？
//
//	林舟：去码头看了看……
//
// 首行之后的部分（玩家输入行 + 正文纯文本）同时写入原文索引 transcript_fts，读取与检索看到的是同一份文字。
// 场外内容（<ooc>）不属于故事，不写入原文（D-30）。

// TranscriptBody 生成一个回合的原文正文：玩家输入行（开场与场外发言没有）、空行、正文纯文本。
// name 把 <say who> 中的人物 ID 换成人物名（work.Model.SpeakerName），可以为 nil。
func TranscriptBody(display []DisplayItem, name func(string) string) string {
	var input string
	var text strings.Builder
	for _, it := range display {
		switch it.T {
		case "input":
			input = inputLine(it)
		case "text":
			text.WriteString(it.Markup)
			text.WriteString("\n\n")
		}
	}
	body := markup.PlainTextWith(text.String(), name)
	switch {
	case input == "":
		return body
	case body == "":
		return input
	}
	return input + "\n\n" + body
}

// TranscriptFile 生成 /transcript/NNNN 的完整内容。
func TranscriptFile(turnNo int, body string) string {
	head := fmt.Sprintf("# 第 %d 回合", turnNo)
	if body == "" {
		return head + "\n"
	}
	return head + "\n" + body + "\n"
}

// TranscriptPath 返回回合的原文路径。
func TranscriptPath(turnNo int) string { return fmt.Sprintf("/transcript/%04d", turnNo) }

// inputLine 把玩家输入写成原文中的一行。
func inputLine(it DisplayItem) string {
	text := strings.Join(strings.Fields(it.Text), " ")
	var line string
	switch it.Kind {
	case InputOpening, InputOOC:
		return ""
	case InputChoice:
		line = "> 玩家选择：" + text
	case InputCommand:
		line = "> 玩家使用命令：" + text
	case InputContinue:
		line = "> （玩家请你接着写）"
	default:
		if it.Mode != "" {
			line = "> 玩家（" + it.Mode + "）：" + text
		} else {
			line = "> 玩家：" + text
		}
	}
	if n := len(it.Attachments); n > 0 {
		line += fmt.Sprintf("（附带 %d 张图片）", n)
	}
	return line
}

// InputSummary 是回合输入的一行说明（/transcript 列表、分支摘要），只依据节点上的输入记录。
func InputSummary(in *TurnInput) string {
	if in == nil {
		return ""
	}
	text := strings.Join(strings.Fields(in.Text), " ")
	var s string
	switch in.Kind {
	case InputOpening:
		return "开场"
	case InputContinue:
		return "（继续）"
	case InputOOC:
		return "（场外发言）"
	case InputChoice:
		s = "玩家选择：" + in.ChoiceLabel
	case InputCommand:
		if in.Command == nil {
			return ""
		}
		s = "玩家使用命令：/" + in.Command.Name
		for _, k := range slices.Sorted(maps.Keys(in.Command.Args)) {
			s += " " + in.Command.Args[k]
		}
		if text != "" {
			s += " " + text
		}
	default:
		s = "玩家：" + text
	}
	return clipRunes(s, 40)
}

func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// TranscriptTexts 返回存档中全部回合的原文正文（节点 ID → 正文）。调用方按进度线过滤（分支隔离）。
func (s *Service) TranscriptTexts(ctx context.Context, saveID string) (map[string]string, error) {
	return s.queryTranscript(ctx, `SELECT node_id, text FROM transcript_fts WHERE save_id = ?`, saveID)
}

// MatchTranscript 用全文索引（trigram）找出含有任一词的回合原文（节点 ID → 正文）。
// 每个词至少 3 个字（trigram 的下限），调用方保证；结果同样需要按进度线过滤。
func (s *Service) MatchTranscript(ctx context.Context, saveID string, terms []string) (map[string]string, error) {
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	return s.queryTranscript(ctx, `SELECT node_id, text FROM transcript_fts WHERE text MATCH ? AND save_id = ?`,
		strings.Join(quoted, " OR "), saveID)
}

func (s *Service) queryTranscript(ctx context.Context, query string, args ...any) (map[string]string, error) {
	rows, err := s.db.Read().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询原文索引失败: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, err
		}
		out[id] = text
	}
	return out, rows.Err()
}
