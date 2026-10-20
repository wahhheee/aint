// Package event 是进程内事件总线：运行器、开发模式等生产事件，SSE 端点消费事件。
//
// 设计（《02-技术方案》1.2、18.3）：
//   - 按主题（topic）隔离：每个存档一个主题（SaveTopic），另有一个全局主题（GlobalTopic）；
//   - 每个主题一个环形缓冲（默认 2,000 条），事件带递增 ID；
//   - 订阅时带上 Last-Event-ID 即可补发断线期间的事件；缓冲已覆盖或 ID 对不上（如进程重启）时
//     返回 resync，客户端应重新拉取时间线后继续订阅；
//   - 发布永不阻塞：订阅者消费太慢时会被断开并标记 Lagged，由 SSE 端点通知客户端 resync。
//
// 本包是基础层，不依赖任何业务包；事件类型与载荷结构见 types.go（前端契约）。
package event

import (
	"sync"
	"time"
)

// GlobalTopic 是全局主题：作品重载、存档标题生成等与具体阅读页无关的事件。
const GlobalTopic = "global"

// SaveTopic 返回存档主题名。
func SaveTopic(saveID string) string { return "save:" + saveID }

// DefaultBufferSize 是每个主题环形缓冲的容量。
const DefaultBufferSize = 2000

// subscriberBuffer 是每个订阅者通道的缓冲；满了即视为消费过慢。
const subscriberBuffer = 512

// Event 是一条事件。Data 会被序列化为 JSON 发给前端。
type Event struct {
	ID   uint64
	Type string
	Data any
	At   time.Time // 仅用于调试与日志
}

// Bus 是事件总线。零值不可用，请用 NewBus。
type Bus struct {
	mu     sync.Mutex
	size   int
	topics map[string]*topic
}

type topic struct {
	lastID uint64
	ring   []Event // 环形缓冲
	head   int     // 下一个写入位置
	count  int
	subs   map[*Subscription]struct{}
}

// NewBus 创建事件总线。size<=0 时使用 DefaultBufferSize。
func NewBus(size int) *Bus {
	if size <= 0 {
		size = DefaultBufferSize
	}
	return &Bus{size: size, topics: make(map[string]*topic)}
}

func (b *Bus) topicLocked(name string) *topic {
	t, ok := b.topics[name]
	if !ok {
		t = &topic{ring: make([]Event, b.size), subs: make(map[*Subscription]struct{})}
		b.topics[name] = t
	}
	return t
}

// Publish 发布事件并返回其 ID。不会阻塞。
func (b *Bus) Publish(topicName, typ string, data any) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.topicLocked(topicName)
	t.lastID++
	ev := Event{ID: t.lastID, Type: typ, Data: data, At: time.Now()}
	t.ring[t.head] = ev
	t.head = (t.head + 1) % len(t.ring)
	if t.count < len(t.ring) {
		t.count++
	}
	for s := range t.subs {
		select {
		case s.ch <- ev:
		default:
			// 消费过慢：断开该订阅，SSE 端点会通知客户端 resync。
			s.lagged = true
			delete(t.subs, s)
			close(s.ch)
		}
	}
	return ev.ID
}

// Subscription 是一个订阅。从 C 读取事件；通道关闭后检查 Lagged。
type Subscription struct {
	C      <-chan Event
	ch     chan Event
	bus    *Bus
	topic  string
	lagged bool
	closed bool
}

// Lagged 表示订阅因消费过慢被断开（只在 C 关闭后读取才有意义）。
func (s *Subscription) Lagged() bool {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	return s.lagged
}

// Close 取消订阅。可重复调用。
func (s *Subscription) Close() {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if t, ok := s.bus.topics[s.topic]; ok {
		if _, ok := t.subs[s]; ok {
			delete(t.subs, s)
			close(s.ch)
		}
	}
}

// Subscribe 订阅主题。lastID 为客户端最后收到的事件 ID（首次连接传 0）。
//
// 返回：
//   - replay：需要先补发的事件（ID > lastID）；
//   - resync：无法补齐（缓冲已覆盖，或 lastID 来自重启前的进程），客户端应重新拉取时间线。
func (b *Bus) Subscribe(topicName string, lastID uint64) (sub *Subscription, replay []Event, resync bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.topicLocked(topicName)

	if lastID > 0 {
		oldest := t.lastID - uint64(t.count) + 1 // 缓冲中最早一条的 ID
		switch {
		case lastID > t.lastID:
			resync = true // 客户端的 ID 比当前还新：进程重启过
		case lastID+1 < oldest:
			resync = true // 断线期间的事件已被覆盖
		default:
			for i := t.count - 1; i >= 0; i-- {
				ev := t.ring[(t.head-1-i+len(t.ring))%len(t.ring)]
				if ev.ID > lastID {
					replay = append(replay, ev)
				}
			}
		}
	}

	ch := make(chan Event, subscriberBuffer)
	sub = &Subscription{C: ch, ch: ch, bus: b, topic: topicName}
	t.subs[sub] = struct{}{}
	return sub, replay, resync
}

// LastID 返回主题当前最后一条事件的 ID（调试用）。
func (b *Bus) LastID(topicName string) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if t, ok := b.topics[topicName]; ok {
		return t.lastID
	}
	return 0
}

// TODO(M1)：长时间无订阅且无活动的存档主题应被回收（目前常驻内存，单机少量存档时影响很小）。
