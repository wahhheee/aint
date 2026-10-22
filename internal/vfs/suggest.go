package vfs

import (
	"cmp"
	"context"
	"path"
	"slices"
	"strings"
)

// similar 在全部可用的挂载点中找出与 target 相近的路径（最多 maxSuggestion 个，目录以 / 结尾）：
// 只差大小写、漏写扩展名、同名文件在别的目录、用了文档的标题或别名、拼写差一两个字母。
func (t *Table) similar(ctx context.Context, target string) []string {
	type hit struct {
		path  string
		score int
	}
	var hits []hit
	add := func(p string, isDir bool, names []string) {
		if s := similarity(target, p, names); s > 0 {
			if isDir {
				p += "/"
			}
			hits = append(hits, hit{p, s})
		}
	}
	for i := range t.mounts {
		m := &t.mounts[i]
		if m.Source == nil {
			continue
		}
		tr, err := t.load(ctx, m)
		if err != nil {
			continue // 建议只是锦上添花，来源出错时不影响主错误
		}
		for p := range tr.dirs {
			add(p, true, nil)
		}
		for p, f := range tr.files {
			add(p, false, f.Names)
		}
	}
	slices.SortFunc(hits, func(a, b hit) int {
		return cmp.Or(b.score-a.score, strings.Compare(a.path, b.path))
	})
	var out []string
	for _, h := range hits {
		if len(out) == maxSuggestion {
			break
		}
		out = append(out, h.path)
	}
	return out
}

// similarity 给出候选路径与目标的相近程度，0 表示不相近。
func similarity(target, cand string, names []string) int {
	if cand == target {
		return 0
	}
	tStem, cStem := stem(target), stem(cand)
	switch {
	case strings.EqualFold(cand, target):
		return 100 // 只差大小写
	case strings.EqualFold(strings.TrimSuffix(cand, path.Ext(cand)), target):
		return 95 // 漏写扩展名
	case tStem == cStem:
		return 80 // 同名文件在别的目录，或扩展名不同
	case slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, tStem) }):
		return 80 // 用了文档 ID、标题、别名或素材 ID
	}
	if n := len([]rune(tStem)); n >= 3 {
		limit := 1
		if n > 5 {
			limit = 2
		}
		if d := distance(tStem, cStem, limit); d <= limit {
			return 60 - d
		}
	}
	return 0
}

// stem 返回小写、去掉扩展名的文件名。
func stem(p string) string {
	b := strings.ToLower(path.Base(p))
	return strings.TrimSuffix(b, path.Ext(b))
}

// distance 计算编辑距离；超过 limit 时提前返回 limit+1。
func distance(a, b string, limit int) int {
	ra, rb := []rune(a), []rune(b)
	if d := len(ra) - len(rb); d > limit || -d > limit {
		return limit + 1
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		best := cur[0]
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			best = min(best, cur[j])
		}
		if best > limit {
			return limit + 1
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
