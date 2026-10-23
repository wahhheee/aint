package vfs

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
)

// FileRef 是记忆文件在清单中的引用（与 save.MemoryFile 字段一致；本包不依赖 save 以保持层次）。
type FileRef struct {
	Blob string
	Size int
	Turn int
}

// Overlay 是记忆的写时复制覆盖层：回合内的写操作只进覆盖层，
// 回合提交时生成新清单，回合被丢弃则写操作作废（12.2）。不并发安全（工具写操作串行执行）。
type Overlay struct {
	base    map[string]FileRef
	turn    int // 本回合的回合号，写入的文件记为在这一回合修改
	changed map[string]FileRef
	removed map[string]bool
	content map[string][]byte // 本回合新写入的内容（提交时写 blob）
}

// NewOverlay 以父节点的记忆清单为底创建覆盖层；turn 是写入时记录的回合号。base 不会被修改。
func NewOverlay(base map[string]FileRef, turn int) *Overlay {
	return &Overlay{base: base, turn: turn, changed: map[string]FileRef{}, removed: map[string]bool{}, content: map[string][]byte{}}
}

// Get 返回文件引用与本回合写入的内容（若有）。
func (o *Overlay) Get(p string) (ref FileRef, data []byte, ok bool) {
	if o.removed[p] {
		return FileRef{}, nil, false
	}
	if r, ok := o.changed[p]; ok {
		return r, o.content[p], true
	}
	r, ok := o.base[p]
	return r, nil, ok
}

// Put 写入文件。引用中的 Blob 与 blob 存储的哈希格式相同（'sha256:<hex>'），提交时内容按同一哈希写入。
func (o *Overlay) Put(p string, data []byte) {
	delete(o.removed, p)
	o.changed[p] = FileRef{Blob: ContentHash(data), Size: len(data), Turn: o.turn}
	o.content[p] = data
}

// Remove 删除文件。
func (o *Overlay) Remove(p string) {
	delete(o.changed, p)
	delete(o.content, p)
	if _, ok := o.base[p]; ok {
		o.removed[p] = true
	}
}

// Dirty 表示本回合是否改过记忆（未改时节点直接复用父节点的 memory_hash）。
func (o *Overlay) Dirty() bool { return len(o.changed) > 0 || len(o.removed) > 0 }

// Paths 返回当前全部文件路径（已排序）。
func (o *Overlay) Paths() []string {
	return slices.Sorted(maps.Keys(o.files()))
}

// TotalSize 返回当前全部记忆文件的字节数。
func (o *Overlay) TotalSize() int {
	n := 0
	for _, f := range o.files() {
		n += f.Size
	}
	return n
}

// Result 返回合并后的清单与本回合新写入的内容（供提交）。
func (o *Overlay) Result() (files map[string]FileRef, written map[string][]byte) {
	return o.files(), maps.Clone(o.content)
}

// Changes 返回本回合改动过的路径（写入的与删除的，各自排序）。
func (o *Overlay) Changes() (written, removed []string) {
	return slices.Sorted(maps.Keys(o.changed)), slices.Sorted(maps.Keys(o.removed))
}

func (o *Overlay) files() map[string]FileRef {
	all := maps.Clone(o.base)
	if all == nil {
		all = map[string]FileRef{}
	}
	maps.Copy(all, o.changed)
	for p := range o.removed {
		delete(all, p)
	}
	return all
}

// ContentHash 计算内容哈希，格式与 blob 存储一致（'sha256:<hex>'）。
func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
