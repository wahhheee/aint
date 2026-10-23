package vfs

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// 检索的默认值与上限（《02-技术方案》12.5）。
const (
	DefaultSearchLimit = 20
	MaxSearchLimit     = 50
	maxHitsPerFile     = 5
	snippetRadius      = 60   // 片段取命中处前后各约 60 字
	maxSearchRunes     = 6000 // 结果正文的总字数上限
)

// Query 是检索参数。
type Query struct {
	Text   string
	Prefix string // 路径前缀，默认 /
	Regex  bool
	Limit  int
}

// Hit 是一条检索结果：文件中的一行。
type Hit struct {
	Path    string
	Line    int    // 1 起
	Snippet string // 命中处前后的文字，命中部分用【】标出
	Order   int    // 来源内的排序键：原文按回合号倒序（新→旧），其余为 0（同一来源内按路径、行号排序）
}

// SearchResult 是检索结果。
type SearchResult struct {
	Query     string
	Aliases   []string // 别名扩展：查询词是某份设定文档的标题或别名时，同时检索的其他叫法
	Hits      []Hit    // 已排序、截断后的结果
	Total     int      // 全部命中行数（截断前）
	Truncated bool     // 因条数或字数上限截断
	Corpora   int      // 结果来自几类语料（记忆 / 作品资料 / 原文）
}

// Matcher 是编译好的检索条件。字面检索不区分大小写；正则使用 RE2（线性时间，没有回溯风险）。
type Matcher struct {
	re *regexp.Regexp
	// Terms 是字面检索的词（含别名）；正则检索时为空。来源可以用它们走全文索引。
	Terms []string
}

// Find 返回一行中全部命中的字节区间。
func (m *Matcher) Find(line string) [][]int { return m.re.FindAllStringIndex(line, -1) }

// Searcher 是能自行检索的来源（如原文走全文索引）；没有实现它的来源由 Table 逐个文件扫描。
// 返回的 Hit.Snippet 可以留空，由 Table 统一截取片段。
type Searcher interface {
	Search(ctx context.Context, m *Matcher, prefix string) ([]Hit, error)
}

// ErrBadQuery 表示检索条件不合法（空查询、正则写错）。
var ErrBadQuery = errors.New("检索条件不合法")

// QueryError 是检索条件的错误。
type QueryError struct{ Reason string }

func (e *QueryError) Error() string { return e.Reason }
func (e *QueryError) Unwrap() error { return ErrBadQuery }

// corpusRank 是语料的排序：记忆 > 作品资料 > 原文；其他挂载点（图片）没有可检索的文字。
var corpusRank = map[string]int{"/memory": 0, "/work": 1, "/transcript": 2}

// Search 在记忆、作品资料与原文中检索（12.5）。结果按 记忆 > 作品资料 > 原文（新→旧）排序，
// 同一文件按行序、最多 5 条；总计不超过 limit 条、约 6,000 字。
func (t *Table) Search(ctx context.Context, q Query) (*SearchResult, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return nil, &QueryError{Reason: "query 是空的"}
	}
	prefix := "/"
	if p := strings.TrimSpace(q.Prefix); p != "" && p != "/" {
		var err error
		if prefix, err = Clean(p); err != nil {
			return nil, &PathError{Path: p, Err: ErrInvalid, Reason: err.Error()}
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	limit = min(limit, MaxSearchLimit)

	res := &SearchResult{Query: text}
	m := &Matcher{}
	if q.Regex {
		re, err := regexp.Compile("(?i)" + text)
		if err != nil {
			return nil, &QueryError{Reason: "正则写法有误（" + regexErr(err) + "）"}
		}
		m.re = re
	} else {
		m.Terms = []string{text}
		res.Aliases = t.aliases(ctx, text)
		m.Terms = append(m.Terms, res.Aliases...)
		quoted := make([]string, len(m.Terms))
		for i, s := range m.Terms {
			quoted[i] = regexp.QuoteMeta(s)
		}
		m.re = regexp.MustCompile("(?i)" + strings.Join(quoted, "|"))
	}

	mounts := slices.Clone(t.mounts)
	slices.SortStableFunc(mounts, func(a, b Mount) int { return rankOf(a.Root) - rankOf(b.Root) })
	var all []Hit
	corpora := 0
	matchedMount := false
	for i := range mounts {
		mt := &mounts[i]
		if _, ok := corpusRank[mt.Root]; !ok || mt.Source == nil {
			continue
		}
		if !overlaps(prefix, mt.Root) {
			continue
		}
		matchedMount = true
		hits, err := t.searchMount(ctx, mt, m, prefix)
		if err != nil {
			return nil, err
		}
		if len(hits) > 0 {
			corpora++
		}
		all = append(all, hits...)
	}
	if !matchedMount && prefix != "/" {
		if mt := t.mountOf(prefix); mt == nil {
			return nil, &PathError{Path: prefix, Err: ErrNotFound, Roots: t.Roots()}
		} else if mt.Source == nil {
			return nil, &PathError{Path: prefix, Err: ErrUnavailable, Reason: mt.Root + "（" + mt.Title + "）暂时不可用"}
		}
	}
	res.Total, res.Corpora = len(all), corpora

	// 每个文件最多 5 条；总条数与总字数受限
	perFile := map[string]int{}
	runes := 0
	for _, h := range all {
		if perFile[h.Path] >= maxHitsPerFile {
			res.Truncated = true
			continue
		}
		n := utf8.RuneCountInString(h.Snippet) + utf8.RuneCountInString(h.Path) + 8
		if len(res.Hits) >= limit || runes+n > maxSearchRunes {
			res.Truncated = true
			break
		}
		perFile[h.Path]++
		runes += n
		res.Hits = append(res.Hits, h)
	}
	return res, nil
}

func rankOf(root string) int {
	if r, ok := corpusRank[root]; ok {
		return r
	}
	return len(corpusRank)
}

// overlaps 判断检索前缀与挂载点是否有交集：前缀在挂载点之内，或挂载点在前缀之内。
func overlaps(prefix, root string) bool {
	return prefix == "/" || within(prefix, root) || within(root, prefix)
}

func within(p, dir string) bool { return p == dir || strings.HasPrefix(p, dir+"/") }

// searchMount 检索一个挂载点：来源能自行检索时交给它，否则逐个文本文件扫描。
func (t *Table) searchMount(ctx context.Context, mt *Mount, m *Matcher, prefix string) ([]Hit, error) {
	if s, ok := mt.Source.(Searcher); ok {
		hits, err := s.Search(ctx, m, prefix)
		if err != nil {
			return nil, err
		}
		slices.SortStableFunc(hits, func(a, b Hit) int {
			if a.Order != b.Order {
				return a.Order - b.Order
			}
			if a.Path != b.Path {
				return strings.Compare(a.Path, b.Path)
			}
			return a.Line - b.Line
		})
		return hits, nil
	}
	tr, err := t.load(ctx, mt)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(tr.files))
	for p := range tr.files {
		if prefix == "/" || within(mt.Root, prefix) || within(p, prefix) {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	var hits []Hit
	for _, p := range paths {
		if kind, _ := mediaOf(p); kind != "" {
			continue
		}
		data, err := mt.Source.Open(ctx, tr.files[p])
		if err != nil || !isText(data) {
			continue // 读不出文字的文件不参与检索
		}
		hits = append(hits, ScanLines(p, splitLines(data), m)...)
	}
	return hits, nil
}

// ScanLines 在文本行中检索，每个命中行一条结果（片段已截取）。
func ScanLines(p string, lines []string, m *Matcher) []Hit {
	var hits []Hit
	for i, line := range lines {
		if loc := m.Find(line); len(loc) > 0 {
			hits = append(hits, Hit{Path: p, Line: i + 1, Snippet: snippet(line, loc)})
		}
	}
	return hits
}

// snippet 截取命中处前后各约 snippetRadius 字，命中部分用【】标出；截掉的部分以"…"表示。
func snippet(line string, loc [][]int) string {
	first := loc[0]
	start := backRunes(line, first[0], snippetRadius)
	end := fwdRunes(line, first[1], snippetRadius)
	var b strings.Builder
	if start > 0 {
		b.WriteString("…")
	}
	pos := start
	for _, l := range loc {
		if l[0] < pos || l[1] > end {
			continue
		}
		b.WriteString(line[pos:l[0]])
		b.WriteString("【" + line[l[0]:l[1]] + "】")
		pos = l[1]
	}
	b.WriteString(line[pos:end])
	if end < len(line) {
		b.WriteString("…")
	}
	return strings.TrimSpace(b.String())
}

// backRunes 从字节位置 i 向前数 n 个字，返回新的字节位置。
func backRunes(s string, i, n int) int {
	for ; n > 0 && i > 0; n-- {
		_, size := utf8.DecodeLastRuneInString(s[:i])
		i -= size
	}
	return i
}

// fwdRunes 从字节位置 i 向后数 n 个字，返回新的字节位置。
func fwdRunes(s string, i, n int) int {
	for ; n > 0 && i < len(s); n-- {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return i
}

// aliases 返回别名扩展：查询词与某份设定文档的标题或别名完全相同（不区分大小写）时，该文档的其他叫法。
// 文档 ID 只在查询词就是 ID 时才视为叫法之一（ID 多为英文缩写，作为检索词容易误中）。
func (t *Table) aliases(ctx context.Context, q string) []string {
	mt := t.mountOf("/work")
	if mt == nil || mt.Source == nil {
		return nil
	}
	tr, err := t.load(ctx, mt)
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range sortedKeys(tr.files) {
		f := tr.files[p]
		if !strings.HasPrefix(p, "/work/lore/") || len(f.Names) < 2 {
			continue
		}
		names := f.Names[1:] // Names[0] 是文档 ID
		if !slices.ContainsFunc(f.Names, func(n string) bool { return strings.EqualFold(n, q) }) {
			continue
		}
		for _, n := range names {
			if n != "" && !strings.EqualFold(n, q) && !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// regexErr 取正则错误中给人看的部分（去掉 "error parsing regexp: " 前缀）。
func regexErr(err error) string {
	return strings.TrimPrefix(err.Error(), "error parsing regexp: ")
}
