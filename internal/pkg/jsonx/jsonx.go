// Package jsonx 集中管理项目内的 JSON 编解码选项。
//
// 项目约定：所有 JSON 编解码都经过本包，不直接调用 encoding/json(/v2)。
// 原因：
//   - 存储、哈希、发给模型的内容必须字节稳定（前缀缓存依赖于此），统一使用 Deterministic；
//   - 以后若需调整全局选项（如未知字段处理），只改这一处。
//
// 结构体字段统一写显式的 snake_case 标签；可省略字段用 omitzero（v2 语义）。
package jsonx

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
)

// Value 是原样保存的 JSON 片段（如工具参数、协议原生数据）。
type Value = jsontext.Value

// Marshal 以确定性方式序列化（map 键有序）。存储、哈希、发给模型的内容都用它。
func Marshal(v any) ([]byte, error) {
	return json.Marshal(v, json.Deterministic(true))
}

// MustMarshal 用于序列化"不可能失败"的内部值（如常量结构）；失败即程序错误。
func MustMarshal(v any) []byte {
	b, err := Marshal(v)
	if err != nil {
		panic("jsonx: " + err.Error())
	}
	return b
}

// MarshalIndent 仅用于调试输出（日志、trace 文件、CLI 打印），不得用于存储。
func MarshalIndent(v any) ([]byte, error) {
	return json.Marshal(v, json.Deterministic(true), jsontext.WithIndent("  "))
}

// MarshalWrite 把 v 序列化写入 w（HTTP 响应等流式场景）。
func MarshalWrite(w io.Writer, v any) error {
	return json.MarshalWrite(w, v, json.Deterministic(true))
}

// Unmarshal 反序列化。未知字段默认忽略（作品文件的未知字段由校验器单独报告）。
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// UnmarshalRead 从 r 读取并反序列化（HTTP 请求体等）。
func UnmarshalRead(r io.Reader, v any) error {
	return json.UnmarshalRead(r, v)
}

// UnmarshalStrict 反序列化并拒绝未知字段。用于 API 请求体，尽早暴露前后端契约不一致。
func UnmarshalStrict(r io.Reader, v any) error {
	return json.UnmarshalRead(r, v, json.RejectUnknownMembers(true))
}
