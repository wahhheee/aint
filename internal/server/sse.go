package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// heartbeat 是 SSE 心跳间隔：保持连接（代理、浏览器的空闲超时），也让客户端及时发现断线。
const heartbeat = 15 * time.Second

// serveEvents 以 SSE 推送某个主题的事件（《02-技术方案》18.3）。
//
//   - 客户端断线重连时带 Last-Event-ID（EventSource 自动携带；也接受 ?last_event_id=），服务端从环形缓冲补发；
//   - 无法补发时发送 resync 事件，客户端应重新拉取时间线后继续订阅；
//   - 订阅者消费过慢被总线断开时同样发送 resync。
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request, topic string) error {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{}) // 长连接：取消写超时

	lastID, _ := strconv.ParseUint(r.Header.Get("Last-Event-ID"), 10, 64)
	if q := r.URL.Query().Get("last_event_id"); q != "" && lastID == 0 {
		lastID, _ = strconv.ParseUint(q, 10, 64)
	}
	sub, replay, resync := s.d.Bus.Subscribe(topic, lastID)
	defer sub.Close()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "retry: 3000\n: connected %s\n\n", topic)

	if resync {
		writeResync(w)
	}
	for _, ev := range replay {
		writeEvent(w, ev)
	}
	if err := rc.Flush(); err != nil {
		return nil
	}
	slog.DebugContext(r.Context(), "SSE 已连接", "topic", topic, "last_event_id", lastID, "replay", len(replay), "resync", resync)

	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case ev, ok := <-sub.C:
			if !ok {
				if sub.Lagged() {
					writeResync(w)
					_ = rc.Flush()
				}
				return nil
			}
			writeEvent(w, ev)
		drain: // 合并发送已积压的事件，减少 Flush 次数（流式正文增量很密集）
			for {
				select {
				case ev, ok := <-sub.C:
					if !ok {
						break drain
					}
					writeEvent(w, ev)
				default:
					break drain
				}
			}
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		if err := rc.Flush(); err != nil {
			return nil // 客户端已断开
		}
	}
}

func writeEvent(w http.ResponseWriter, ev event.Event) {
	data, err := jsonx.Marshal(ev.Data)
	if err != nil {
		slog.Error("SSE 事件序列化失败", "type", ev.Type, "err", err)
		return
	}
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Type, data)
}

func writeResync(w http.ResponseWriter) {
	fmt.Fprintf(w, "event: %s\ndata: {}\n\n", event.Resync)
}
