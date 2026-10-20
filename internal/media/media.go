// Package media 处理图片：解码校验、缩放派生、元数据清除（《02-技术方案》第 15 节）。
//
//   - 作品素材：导入时解码校验尺寸与格式；发送给模型前按 Caps.ImageMaxEdge（默认长边 1568px）缩放；
//   - 派生图按 (原图哈希, 尺寸, 格式) 缓存在 <数据目录>/cache/derived/，保证每次发送字节一致（前缀稳定）；
//   - 玩家上传：类型嗅探（png/jpeg/webp/gif 首帧）、≤ 20 MiB、长边 ≤ 8192；重新编码以去除 EXIF 等元数据；
//   - 只用标准库 image/* 与 golang.org/x/image（webp 解码、draw 缩放），不引入 CGO。
//
// TODO(M2-10)：实现。AI 生图（P2）预留：显示项 image 已含 source / status 字段。
package media

// DefaultMaxEdge 是发送给模型前缩放的默认长边。
const DefaultMaxEdge = 1568

// 上传限制。
const (
	MaxUploadBytes = 20 << 20
	MaxUploadEdge  = 8192
)
