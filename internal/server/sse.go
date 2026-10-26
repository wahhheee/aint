package server

import (
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wahhheee/aint/internal/event"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
)

// heartbeat 是 SSE 心跳间隔：保持连接（代理、浏览器的空闲超时），也让客户端及时发现断线。
const heartbeat = 15 * time.Second

// serveEvents 以 SSE 推送某个主题的事件（《02-技术方案》18.3）。
//
//   - 客户端断线重连时带 Last-Event-ID（EventSource 自动携带；也接受 ?last_event_id=），服务端从环形缓冲补发；
//   - 存档首次连接或无法补发时发送 session.snapshot，全局主题无法补发时发送 resync；
//   - 订阅者消费过慢被总线断开时发送 resync，客户端重新连接并取得快照。
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request, topic string) error {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{}) // 长连接：取消写超时

	lastID, _ := strconv.ParseUint(r.Header.Get("Last-Event-ID"), 10, 64)
	if q := r.URL.Query().Get("last_event_id"); q != "" && lastID == 0 {
		lastID, _ = strconv.ParseUint(q, 10, 64)
	}
	var sub *event.Subscription
	var replay []event.Event
	var resync bool
	if saveID, ok := strings.CutPrefix(topic, "save:"); ok {
		sub, replay, resync = s.d.Runtime.SubscribeEvents(saveID, lastID)
	} else {
		sub, replay, resync = s.d.Bus.Subscribe(topic, lastID)
	}
	defer sub.Close()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// 写失败说明客户端已断开：结束本次连接即可（客户端会带 Last-Event-ID 重连）。
	err := writeRaw(w, "retry: 3000\n: connected\n\n")
	if err == nil && resync && (len(replay) == 0 || replay[0].Type != event.SessionSnapshot) {
		err = writeResync(w)
	}
	for _, ev := range replay {
		if err == nil {
			err = writeEvent(w, ev)
		}
	}
	if err != nil || rc.Flush() != nil {
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
				if sub.Lagged() && writeResync(w) == nil {
					_ = rc.Flush()
				}
				return nil
			}
			err = writeEvent(w, ev)
		drain: // 合并发送已积压的事件，减少 Flush 次数（流式正文增量很密集）
			for err == nil {
				select {
				case ev, ok := <-sub.C:
					if !ok {
						break drain
					}
					err = writeEvent(w, ev)
				default:
					break drain
				}
			}
		case <-ticker.C:
			err = writeRaw(w, ": ping\n\n")
		}
		if err != nil || rc.Flush() != nil {
			return nil // 客户端已断开
		}
	}
}

// writeEvent 写一条事件。序列化失败只记日志并跳过该事件（返回 nil）；返回的错误都是写连接失败。
func writeEvent(w http.ResponseWriter, ev event.Event) error {
	data, err := jsonx.Marshal(ev.Data)
	if err != nil {
		slog.Error("SSE 事件序列化失败", "type", ev.Type, "err", err)
		return nil
	}
	return writeRaw(w, "id: "+strconv.FormatUint(ev.ID, 10)+"\nevent: "+ev.Type+"\ndata: "+string(data)+"\n\n")
}

func writeResync(w http.ResponseWriter) error {
	return writeRaw(w, "event: "+event.Resync+"\ndata: {}\n\n")
}

func writeRaw(w http.ResponseWriter, s string) error {
	_, err := io.WriteString(w, s)
	return err
}
