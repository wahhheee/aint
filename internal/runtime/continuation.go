package runtime

// maxOverlap 是续写去重检测的最大重叠长度（字符）。
const maxOverlap = 200

// dedupOverlap 返回续写开头需要丢弃的字节数：取最大 k（≤ 200 字符）使 prev 的末尾 k 个字符
// 等于 next 的开头 k 个字符（《02-技术方案》9.4 第 3 步）。
//
// TODO(M4-5)：接入 callOnce——续写时先缓冲前 maxOverlap 个字符再决定推送哪些，
// 显示上合并为连续正文；LLM 消息中保留真实结构（部分输出 + 续写提示 + 续写）。
func dedupOverlap(prev, next string) int {
	pr := []rune(prev)
	nr := []rune(next)
	limit := min(maxOverlap, len(pr), len(nr))
	for k := limit; k > 0; k-- {
		if string(pr[len(pr)-k:]) == string(nr[:k]) {
			return len(string(nr[:k]))
		}
	}
	return 0
}
