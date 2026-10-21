package event

import "testing"

func TestReplayAfterReconnect(t *testing.T) {
	b := NewBus(4)
	topic := SaveTopic("s1")
	for i := range 3 {
		b.Publish(topic, "turn.text", i)
	}
	_, replay, resync := b.Subscribe(topic, 1)
	if resync {
		t.Fatal("缓冲中仍有事件，不应要求 resync")
	}
	if len(replay) != 2 || replay[0].ID != 2 || replay[1].ID != 3 {
		t.Fatalf("补发内容不对: %+v", replay)
	}
}

func TestResyncWhenOverwritten(t *testing.T) {
	b := NewBus(4)
	topic := SaveTopic("s1")
	for i := range 10 {
		b.Publish(topic, "turn.text", i)
	}
	if _, _, resync := b.Subscribe(topic, 2); !resync {
		t.Fatal("事件已被覆盖，应要求 resync")
	}
	if _, _, resync := b.Subscribe(topic, 99); !resync {
		t.Fatal("客户端 ID 比服务端新（进程重启），应要求 resync")
	}
}

func TestSlowSubscriberIsDropped(t *testing.T) {
	b := NewBus(8)
	topic := SaveTopic("s1")
	sub, _, _ := b.Subscribe(topic, 0)
	for i := range subscriberBuffer + 1 {
		b.Publish(topic, "turn.text", i)
	}
	for range sub.C {
	}
	if !sub.Lagged() {
		t.Fatal("消费过慢的订阅者应被标记为 Lagged")
	}
	sub.Close() // 重复关闭不应 panic
}
