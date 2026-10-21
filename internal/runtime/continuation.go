package runtime

import (
	"strings"
	"unicode/utf8"
)

// 续写去重（《02-技术方案》9.4 第 3 步）：模型续写时常把上一段末尾重复一遍，显示前去掉重叠部分。
// 只影响显示（turn.text 增量与显示项）；LLM 消息保留模型的真实输出，保证上下文与发送过的内容一致。
const (
	// maxOverlap 是重叠检测的最大长度（字符）。
	maxOverlap = 200
	// minOverlap 是认定为"重复"的最小长度（字符）。技术方案未规定下限：太短的重合多半是巧合
	// （如上一段以"好"结尾、续写以"好像"开头），宁可少去一点重复，也不能吞掉正文。
	// TODO(待确认)：下限取值，见 docs/notes/open-questions.md 第 1 条。
	minOverlap = 8
)

// dedupOverlap 返回续写开头需要丢弃的字节数：取最大 k（minOverlap ≤ k ≤ maxOverlap 个字符）
// 使 prev 的末尾 k 个字符等于 next 的开头 k 个字符；没有这样的 k 时返回 0。
func dedupOverlap(prev, next string) int {
	pr := []rune(prev)
	nr := []rune(next)
	limit := min(maxOverlap, len(pr), len(nr))
	for k := limit; k >= minOverlap; k-- {
		if string(pr[len(pr)-k:]) == string(nr[:k]) {
			return len(string(nr[:k]))
		}
	}
	return 0
}

// overlapFilter 在续写开头先缓冲 maxOverlap 个字符（或直到流结束），去掉与上一段末尾重叠的部分后再放行；
// 之后的增量原样通过。nil 表示不需要去重，push 原样返回。
type overlapFilter struct {
	prev  string // 上一段正文的末尾
	buf   strings.Builder
	runes int
	done  bool
}

// newOverlapFilter 为一次续写创建过滤器；prev 为空（上一段没有正文）时返回 nil。
func newOverlapFilter(prev string) *overlapFilter {
	if prev == "" {
		return nil
	}
	if r := []rune(prev); len(r) > maxOverlap {
		prev = string(r[len(r)-maxOverlap:])
	}
	return &overlapFilter{prev: prev}
}

// push 送入一个正文增量，返回现在可以显示的文本（缓冲期间为空）。
func (f *overlapFilter) push(delta string) string {
	if f == nil || f.done {
		return delta
	}
	f.buf.WriteString(delta)
	f.runes += utf8.RuneCountInString(delta)
	if f.runes < maxOverlap {
		return ""
	}
	return f.flush()
}

// flush 结束缓冲，返回去掉重叠后的文本。流结束时必须调用一次（缓冲可能还没满）。
func (f *overlapFilter) flush() string {
	if f == nil || f.done {
		return ""
	}
	f.done = true
	s := f.buf.String()
	return s[dedupOverlap(f.prev, s):]
}
