package vfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
)

// MemoryWarnTotal 是记忆总量的提醒线：超过后写入结果与整理指令都会提醒精简（12.2）。
const MemoryWarnTotal = 1 << 20

// 写入相关的错误类别（errors.Is 判断）。
var (
	ErrReadOnly = errors.New("只读")
	ErrTooLarge = errors.New("超过大小上限")
)

// Writer 是可写的来源（只有 /memory）。写入只进入回合的覆盖层，随回合提交或作废。
type Writer interface {
	Write(ctx context.Context, p string, data []byte) error
	Remove(ctx context.Context, p string) error
	// TotalSize 返回当前全部文件的字节数（检查总量上限）。
	TotalSize() int
}

// WriteResult 是一次写入的结果。
type WriteResult struct {
	Path      string
	Created   bool // 新建（否则为覆盖）
	Lines     int
	TotalSize int // 写入后该挂载点的总字节数
}

// Write 写入文件（新建或覆盖）。路径须满足 Writable，单个文件与总量受 MaxMemoryFile / MaxMemoryTotal 限制。
func (t *Table) Write(ctx context.Context, raw string, data []byte) (*WriteResult, error) {
	p, m, w, tr, err := t.writable(ctx, raw, true)
	if err != nil {
		return nil, err
	}
	if _, isDir := tr.dirs[p]; isDir {
		return nil, &PathError{Path: p, Err: ErrIsDir, Children: childNames(tr, tr.dirs[p])}
	}
	for dir := path.Dir(p); dir != m.Root; dir = path.Dir(dir) {
		if _, isFile := tr.files[dir]; isFile {
			return nil, &PathError{Path: p, Err: ErrInvalid, Reason: dir + " 是文件，不能作为目录"}
		}
	}
	if !isText(data) {
		return nil, &PathError{Path: p, Err: ErrInvalid, Reason: "内容需要是文本"}
	}
	if len(data) > MaxMemoryFile {
		return nil, &PathError{Path: p, Err: ErrTooLarge,
			Reason: fmt.Sprintf("单个文件最多 %s（这次约 %s），可以拆成几个文件", FormatSize(MaxMemoryFile), FormatSize(len(data)))}
	}
	old, existed := tr.files[p]
	total := w.TotalSize() - old.Size + len(data)
	if total > MaxMemoryTotal {
		return nil, &PathError{Path: p, Err: ErrTooLarge,
			Reason: fmt.Sprintf("记忆总量最多 %s（写入后约 %s），可以先合并精简一些文件", FormatSize(MaxMemoryTotal), FormatSize(total))}
	}
	if err := w.Write(ctx, p, data); err != nil {
		return nil, err
	}
	return &WriteResult{Path: p, Created: !existed, Lines: len(splitLines(data)), TotalSize: total}, nil
}

// Remove 删除文件（目录不能直接删除）。
func (t *Table) Remove(ctx context.Context, raw string) error {
	p, m, w, tr, err := t.writable(ctx, raw, false)
	if err != nil {
		return err
	}
	if _, ok := tr.files[p]; !ok {
		if d, isDir := tr.dirs[p]; isDir {
			return &PathError{Path: p, Err: ErrIsDir, Children: childNames(tr, d)}
		}
		return t.notFound(ctx, p, m, tr)
	}
	return w.Remove(ctx, p)
}

// ReadFile 返回文本文件的完整内容（不分行、不截断），供 edit 这类需要原文的操作使用。
func (t *Table) ReadFile(ctx context.Context, raw string) ([]byte, error) {
	p, m, err := t.resolve(ctx, raw)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, &PathError{Path: "/", Err: ErrIsDir, Children: dirNames(t.Roots())}
	}
	tr, err := t.load(ctx, m)
	if err != nil {
		return nil, err
	}
	f, ok := tr.files[p]
	if !ok {
		if d, isDir := tr.dirs[p]; isDir {
			return nil, &PathError{Path: p, Err: ErrIsDir, Children: childNames(tr, d)}
		}
		return nil, t.notFound(ctx, p, m, tr)
	}
	if kind, _ := mediaOf(p); kind != "" {
		return nil, &PathError{Path: p, Err: ErrBinary, Reason: "是" + kind + "，不能以文字读取", Summary: f.Summary}
	}
	data, err := m.Source.Open(ctx, f)
	if err != nil {
		return nil, err
	}
	if !isText(data) {
		return nil, &PathError{Path: p, Err: ErrBinary, Reason: "不是文本文件，不能以文字读取", Summary: f.Summary}
	}
	return bytes.TrimPrefix(data, []byte(bom)), nil
}

// writable 校验写操作的路径：位于可写的挂载点下；names 为 true 时还要满足 Writable 的命名约束（删除时不检查）。
func (t *Table) writable(ctx context.Context, raw string, names bool) (string, *Mount, Writer, *tree, error) {
	p, m, err := t.resolve(ctx, raw)
	if err != nil {
		return "", nil, nil, nil, err
	}
	var w Writer
	if m != nil {
		w, _ = m.Source.(Writer)
	}
	if w == nil {
		return "", nil, nil, nil, &PathError{Path: p, Err: ErrReadOnly, Reason: "只能写入 /memory 下的文件"}
	}
	if err := Writable(p); err != nil && names {
		return "", nil, nil, nil, &PathError{Path: p, Err: ErrInvalid, Reason: err.Error()}
	}
	tr, err := t.load(ctx, m)
	if err != nil {
		return "", nil, nil, nil, err
	}
	return p, m, w, tr, nil
}

// FormatSize 把字节数写成便于阅读的大小（字节 / KiB / MiB）。
func FormatSize(n int) string {
	switch {
	case n >= 1<<20:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/(1<<20)), ".0") + " MiB"
	case n >= 1<<10:
		return fmt.Sprintf("%d KiB", (n+512)>>10)
	}
	return fmt.Sprintf("%d 字节", n)
}
