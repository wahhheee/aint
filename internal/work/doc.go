// Package work 负责作品：解析、编译（→ Model）、校验报告、导入、版本管理、开发模式。
//
// 数据流（《02-技术方案》5.1）：
//
//	来源（zip / 目录 / GitHub / URL / 开发目录 / 内置）
//	  → FileSet（安全读取：路径、数量、体积检查；忽略规则）      fileset.go
//	  → Compile → Model + Report（error / warning / info）        compile.go、report.go
//	  → 有 error：拒绝；否则按文件树哈希去重，写 blob 与版本记录   service.go、repo.go
//
// 关键概念：
//   - 版本身份 = 文件树哈希（TreeHash）：同作品同哈希即同一版本，与 zip 文件名、导入方式无关；
//   - Model 是只读的编译结果，按版本 ID 缓存，供上下文构建、VFS、面板渲染使用；
//   - 作品内容永不执行（不变量 6）：只解析 Markdown / YAML / Liquid / CSS 与素材。
package work
