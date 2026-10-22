package wire

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// maxEventSize 是单个 SSE 事件的上限（OpenAI response.completed 会带上全部输出与加密推理，可能较大）。
const maxEventSize = 32 << 20

// Event 是一个 Server-Sent Event。Name 为 "event:" 行的值（Gemini 没有该行，为空）。
type Event struct {
	Name string
	Data []byte
}

// SSE 逐个读取事件。不是并发安全的。
type SSE struct {
	r    *bufio.Reader
	name string
	data bytes.Buffer
}

// NewSSE 创建读取器。
func NewSSE(r io.Reader) *SSE { return &SSE{r: bufio.NewReaderSize(r, 64<<10)} }

// ErrEventTooLarge 表示单个事件超过上限。
var ErrEventTooLarge = errors.New("SSE 事件过大")

// Next 返回下一个事件；流结束返回 io.EOF。
// 规则（WHATWG）：空行分隔事件；以 ":" 开头为注释；多个 data 行以换行拼接；
// 只有 data 的事件也会返回（Gemini）；没有 data 的事件（只有 event 行）忽略。
func (s *SSE) Next() (Event, error) {
	for {
		line, err := s.readLine()
		if err != nil {
			if errors.Is(err, io.EOF) && s.data.Len() > 0 {
				return s.emit(), nil // 最后一个事件后没有空行
			}
			return Event{}, err
		}
		if len(line) == 0 {
			if s.data.Len() > 0 {
				return s.emit(), nil
			}
			s.name = ""
			continue
		}
		if line[0] == ':' {
			continue
		}
		field, value, _ := bytes.Cut(line, []byte{':'})
		value = bytes.TrimPrefix(value, []byte{' '})
		switch string(field) {
		case "event":
			s.name = string(value)
		case "data":
			if s.data.Len() > 0 {
				s.data.WriteByte('\n')
			}
			if s.data.Len()+len(value) > maxEventSize {
				return Event{}, ErrEventTooLarge
			}
			s.data.Write(value)
		}
	}
}

func (s *SSE) emit() Event {
	ev := Event{Name: s.name, Data: bytes.Clone(s.data.Bytes())}
	s.name = ""
	s.data.Reset()
	return ev
}

// readLine 读取一行（去掉 \r\n / \n），不限长度但受 maxEventSize 约束。
func (s *SSE) readLine() ([]byte, error) {
	var buf []byte
	for {
		chunk, err := s.r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if err == nil {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			if len(buf) > maxEventSize {
				return nil, ErrEventTooLarge
			}
			continue
		}
		if errors.Is(err, io.EOF) && len(buf) > 0 {
			break
		}
		return nil, err
	}
	buf = bytes.TrimSuffix(buf, []byte{'\n'})
	buf = bytes.TrimSuffix(buf, []byte{'\r'})
	return buf, nil
}

// IsDone 判断 OpenAI 风格的 "data: [DONE]" 结束标记。
func IsDone(ev Event) bool { return string(bytes.TrimSpace(ev.Data)) == "[DONE]" }
