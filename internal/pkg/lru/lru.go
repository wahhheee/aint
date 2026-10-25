// Package lru 是一个并发安全的定长 LRU 缓存（只保留最近使用的 N 项）。
package lru

import (
	"container/list"
	"sync"
)

// Cache 是 LRU 缓存。零值不可用，用 New 创建。
type Cache[K comparable, V any] struct {
	mu    sync.Mutex
	max   int
	order *list.List // 前端是最近使用的项
	items map[K]*list.Element
}

type entry[K comparable, V any] struct {
	key K
	val V
}

// New 创建最多保留 size 项的缓存（size ≤ 0 时取 1）。
func New[K comparable, V any](size int) *Cache[K, V] {
	return &Cache[K, V]{max: max(size, 1), order: list.New(), items: map[K]*list.Element{}}
}

// Get 取出一项并标记为最近使用。
func (c *Cache[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		c.order.MoveToFront(e)
		return e.Value.(*entry[K, V]).val, true
	}
	var zero V
	return zero, false
}

// Put 放入一项；超过容量时淘汰最久未使用的项。
func (c *Cache[K, V]) Put(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		e.Value.(*entry[K, V]).val = v
		c.order.MoveToFront(e)
		return
	}
	c.items[k] = c.order.PushFront(&entry[K, V]{key: k, val: v})
	for c.order.Len() > c.max {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*entry[K, V]).key)
	}
}

// Len 返回当前项数。
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Clear 丢弃全部缓存项；持久内容回收后可据此移除悬空引用。
func (c *Cache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order.Init()
	clear(c.items)
}
