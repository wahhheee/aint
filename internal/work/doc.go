// Package work 负责作品：解析、编译（→ Model）、校验报告、导入、版本管理、开发目录。
//
// 数据流（《02-技术方案》5.1）：
//
//	来源（zip / 浏览器文件夹 / 本机目录 / 内置 / GitHub / URL / 开发目录）
//	  → FileSet：路径规范化（NFC）与冲突检查、定位作品根、忽略规则、    fileset.go、archive.go、upload.go、ignore.go
//	    按声明大小检查限制后才读取内容（zip 另查条目数、压缩比、加密）
//	  → Compile → Model + Report（error / warning / info）              compile.go、report.go
//	  → 两段式导入：预览（校验报告 + 暂存）→ 确认                       importer.go
//	  → 有 error：拒绝；否则按文件树哈希去重，写 blob 与版本记录         service.go、repo.go
//
// 关键概念：
//   - 版本身份 = 文件树哈希（TreeHash）：同作品同哈希即同一版本，与 zip 文件名、导入方式无关；
//   - Model 是只读的编译结果，按版本 ID 缓存，供上下文构建、VFS、面板渲染使用；
//   - 作品内容永不执行（不变量 6）：只解析 Markdown / YAML / Liquid / CSS 与素材。
package work
