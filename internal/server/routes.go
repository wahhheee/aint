package server

// routes 注册全部路由（《02-技术方案》18.2，前缀 /api/v1）。
// 尚未实现的端点也在这里列出并返回 501，作为"还剩什么"的一览表；实现后替换处理函数即可。
func (s *Server) routes() {
	m := s.mux
	todo := s.notImplemented

	// 系统
	m.HandleFunc("GET /api/v1/system/info", s.h(s.systemInfo))
	m.HandleFunc("GET /api/v1/system/health", s.h(s.health))
	m.HandleFunc("GET /api/v1/events", s.h(s.globalEvents)) // 全局事件：作品重载、存档标题（技术方案未单列，补充）

	// 设置
	m.HandleFunc("GET /api/v1/settings", s.h(s.getSettings))
	m.HandleFunc("PATCH /api/v1/settings", s.h(s.patchSettings))

	// 作品
	m.HandleFunc("GET /api/v1/works", s.h(s.listWorks))
	m.HandleFunc("GET /api/v1/works/{id}", s.h(s.getWork))
	m.HandleFunc("GET /api/v1/works/{id}/saves", s.h(s.listWorkSaves))
	m.HandleFunc("POST /api/v1/works/import", todo("导入作品（M1-4 / M6-1）"))
	m.HandleFunc("POST /api/v1/works/import/{task}/confirm", todo("确认导入（M1-4）"))
	m.HandleFunc("GET /api/v1/works/{id}/versions", todo("版本列表（M6-5）"))
	m.HandleFunc("POST /api/v1/works/{id}/check-update", todo("检查更新（M6-1）"))
	m.HandleFunc("GET /api/v1/works/{id}/versions/{vid}/export", todo("导出 zip（M6-5）"))
	m.HandleFunc("GET /api/v1/works/{id}/versions/{vid}/report", todo("校验报告（M2-12）"))
	m.HandleFunc("DELETE /api/v1/works/{id}", todo("删除作品（M6-5）"))
	m.HandleFunc("DELETE /api/v1/works/{id}/versions/{vid}", todo("删除版本（M6-5）"))
	m.HandleFunc("POST /api/v1/works/dev-link", todo("链接开发目录（M6-2）"))
	m.HandleFunc("GET /api/v1/assets/{hash}", s.h(s.asset))
	m.HandleFunc("POST /api/v1/uploads", todo("上传图片（M2-10）"))

	// 存档
	m.HandleFunc("GET /api/v1/saves", s.h(s.recentSaves))
	m.HandleFunc("POST /api/v1/saves", s.h(s.createSave))
	m.HandleFunc("GET /api/v1/saves/{id}", s.h(s.getSave))
	m.HandleFunc("PATCH /api/v1/saves/{id}", s.h(s.patchSave))
	m.HandleFunc("DELETE /api/v1/saves/{id}", s.h(s.deleteSave))
	m.HandleFunc("GET /api/v1/saves/{id}/timeline", s.h(s.timeline))
	m.HandleFunc("GET /api/v1/saves/{id}/panels", s.h(s.panels))
	m.HandleFunc("GET /api/v1/saves/{id}/events", s.h(s.saveEvents))
	m.HandleFunc("POST /api/v1/saves/{id}/turns", s.h(s.submitTurn))
	m.HandleFunc("POST /api/v1/saves/{id}/turns/{tid}/cancel", s.h(s.cancelTurn))
	m.HandleFunc("GET /api/v1/saves/{id}/tree", todo("分支结构（M3-9）"))
	m.HandleFunc("POST /api/v1/saves/{id}/head", todo("回到此处 / 切换分支（M3-5）"))
	m.HandleFunc("DELETE /api/v1/saves/{id}/branches/{nid}", todo("删除分支（M3-5，P1）"))
	m.HandleFunc("POST /api/v1/saves/{id}/nodes/{nid}/copy", todo("复制为新存档（M3-5，P1）"))
	m.HandleFunc("POST /api/v1/saves/{id}/nodes/{nid}/regenerate", todo("重新生成（M3-5）"))
	m.HandleFunc("POST /api/v1/saves/{id}/nodes/{nid}/rewrite", todo("修改 AI 正文（M3-5）"))
	m.HandleFunc("POST /api/v1/saves/{id}/nodes/{nid}/discard", todo("丢弃中断回合（M3-5）"))
	m.HandleFunc("GET /api/v1/saves/{id}/nodes/{nid}/inspect", todo("检查器（M5-9）"))
	m.HandleFunc("POST /api/v1/saves/{id}/exit", todo("退出存档与标题生成（M5-13）"))
	m.HandleFunc("POST /api/v1/saves/{id}/title", todo("生成存档标题（M5-13）"))
	m.HandleFunc("POST /api/v1/saves/{id}/compact", todo("手动整理（M3-6）"))
	m.HandleFunc("GET /api/v1/saves/{id}/state", todo("状态查看（M2-1）"))
	m.HandleFunc("PATCH /api/v1/saves/{id}/state", todo("状态编辑（M3-9）"))
	m.HandleFunc("GET /api/v1/saves/{id}/memory", todo("记忆列表（M3-9）"))
	m.HandleFunc("GET /api/v1/saves/{id}/memory/{path...}", todo("读取记忆（M3-9）"))
	m.HandleFunc("PUT /api/v1/saves/{id}/memory/{path...}", todo("修改记忆（M3-9）"))
	m.HandleFunc("DELETE /api/v1/saves/{id}/memory/{path...}", todo("删除记忆（M3-9）"))

	// 供应商与模型
	m.HandleFunc("GET /api/v1/providers", s.h(s.listProviders))
	m.HandleFunc("POST /api/v1/providers", todo("添加供应商（M1-11）"))
	m.HandleFunc("PATCH /api/v1/providers/{id}", todo("修改供应商（M4-7）"))
	m.HandleFunc("DELETE /api/v1/providers/{id}", todo("删除供应商（M4-7）"))
	m.HandleFunc("POST /api/v1/providers/{id}/test", todo("连接测试（M1-11）"))
	m.HandleFunc("GET /api/v1/providers/{id}/remote-models", todo("拉取模型列表（M4-7）"))
	m.HandleFunc("GET /api/v1/models", s.h(s.listModels))
	m.HandleFunc("POST /api/v1/models", todo("添加模型（M4-7）"))
	m.HandleFunc("PATCH /api/v1/models/{id}", todo("修改模型（M4-7）"))
	m.HandleFunc("DELETE /api/v1/models/{id}", todo("删除模型（M4-7）"))

	// 用量与访问控制
	m.HandleFunc("GET /api/v1/usage", todo("用量统计（M4-4）"))
	m.HandleFunc("POST /api/v1/auth/login", todo("登录（M6-9）"))
	m.HandleFunc("POST /api/v1/auth/logout", todo("退出登录（M6-9）"))
	m.HandleFunc("PUT /api/v1/auth/password", todo("设置访问密码（M6-9）"))

	// 未匹配的 /api 路径返回 JSON 404；其余交给前端（SPA）
	m.HandleFunc("/api/", s.h(s.apiNotFound))
	m.Handle("/", s.staticHandler())
}
