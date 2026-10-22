package lru

import "testing"

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	if _, ok := c.Get("a"); !ok { // a 变为最近使用
		t.Fatal("a 应在缓存中")
	}
	c.Put("c", 3) // 淘汰 b
	if _, ok := c.Get("b"); ok {
		t.Fatal("b 应被淘汰")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatal("a 应保留")
	}
	c.Put("a", 10)
	if v, _ := c.Get("a"); v != 10 || c.Len() != 2 {
		t.Fatalf("更新已有项：%d, len=%d", v, c.Len())
	}
}
