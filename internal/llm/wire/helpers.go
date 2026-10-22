package wire

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"

	"github.com/wahhheee/yanyi/internal/llm"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ToolID 把工具调用 ID 映射为各协议都接受的形式（字母、数字、_、-，最长 64）。
// 合法的 ID 原样返回；否则做确定性映射（同一 ID 每次结果相同，保证前缀稳定）。
func ToolID(id string) string {
	if safeID.MatchString(id) {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "call_" + hex.EncodeToString(sum[:12])
}

// ImageData 读取图片并返回 base64 编码与媒体类型。
func ImageData(ctx context.Context, blobs llm.BlobReader, img *llm.ImageRef) (data, mime string, err error) {
	if img == nil {
		return "", "", fmt.Errorf("图片块缺少图片引用")
	}
	if blobs == nil {
		return "", "", &llm.APIError{Kind: llm.ErrInvalidRequest, Message: "请求中有图片，但适配器没有配置图片读取"}
	}
	b, err := blobs.ReadBlob(ctx, img.Blob)
	if err != nil {
		return "", "", fmt.Errorf("读取图片 %s: %w", img.Blob, err)
	}
	return base64.StdEncoding.EncodeToString(b), img.MIME, nil
}

// Queue 是流式事件的小队列：一个协议事件可能对应多个中立事件。
type Queue struct{ evs []llm.Event }

// Push 追加事件。
func (q *Queue) Push(ev ...llm.Event) { q.evs = append(q.evs, ev...) }

// Pop 取出队首事件。
func (q *Queue) Pop() (llm.Event, bool) {
	if len(q.evs) == 0 {
		return nil, false
	}
	ev := q.evs[0]
	q.evs = q.evs[1:]
	return ev, true
}
