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
	m.HandleFunc("POST /api/v1/works/import", s.h(s.importWork)) // zip / 文件夹；GitHub、链接见 M6-1
	m.HandleFunc("POST /api/v1/works/import/{task}/confirm", s.h(s.confirmImport))
	m.HandleFunc("DELETE /api/v1/works/import/{task}", s.h(s.cancelImport)) // 放弃导入（技术方案未单列，补充）
	m.HandleFunc("GET /api/v1/works/{id}/versions", todo("版本列表（M6-5）"))
	m.HandleFunc("POST /api/v1/works/{id}/check-update", todo("检查更新（M6-1）"))
	m.HandleFunc("GET /api/v1/works/{id}/versions/{vid}/export", todo("导出 zip（M6-5）"))
	m.HandleFunc("GET /api/v1/works/{id}/versions/{vid}/report", s.h(s.versionReport))
	m.HandleFunc("DELETE /api/v1/works/{id}", todo("删除作品（M6-5）"))
	m.HandleFunc("DELETE /api/v1/works/{id}/versions/{vid}", todo("删除版本（M6-5）"))
	m.HandleFunc("POST /api/v1/works/dev-link", todo("链接开发目录（M6-2）"))
	m.HandleFunc("GET /api/v1/assets/{hash}", s.h(s.asset))
	m.HandleFunc("POST /api/v1/uploads", s.h(s.createUpload))
	m.HandleFunc("GET /api/v1/uploads/{hash}", s.h(s.upload))

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
	m.HandleFunc("GET /api/v1/saves/{id}/tree", s.h(s.tree))
	m.HandleFunc("POST /api/v1/saves/{id}/head", s.h(s.setHead))
	m.HandleFunc("DELETE /api/v1/saves/{id}/branches/{nid}", s.h(s.deleteBranch))
	m.HandleFunc("POST /api/v1/saves/{id}/nodes/{nid}/copy", s.h(s.copySave))
	m.HandleFunc("POST /api/v1/saves/{id}/nodes/{nid}/regenerate", s.h(s.regenerate))
	m.HandleFunc("POST /api/v1/saves/{id}/nodes/{nid}/rewrite", s.h(s.rewrite))
	m.HandleFunc("POST /api/v1/saves/{id}/nodes/{nid}/discard", s.h(s.discard))
	m.HandleFunc("GET /api/v1/saves/{id}/debug", s.h(s.debugSave))
	m.HandleFunc("PATCH /api/v1/saves/{id}/debug", s.h(s.debugSave))
	m.HandleFunc("DELETE /api/v1/saves/{id}/debug", s.h(s.debugSave))
	m.HandleFunc("GET /api/v1/saves/{id}/debug/calls/{cid}", s.h(s.debugCall))
	m.HandleFunc("GET /api/v1/saves/{id}/debug/calls/{cid}/file", s.h(s.debugFile))
	m.HandleFunc("GET /api/v1/saves/{id}/debug/calls/{cid}/images/{hash}", s.h(s.debugImage))
	m.HandleFunc("POST /api/v1/saves/{id}/exit", todo("退出存档与标题生成（M5-13）"))
	m.HandleFunc("POST /api/v1/saves/{id}/title", todo("生成存档标题（M5-13）"))
	m.HandleFunc("POST /api/v1/saves/{id}/compact", s.h(s.compact))
	m.HandleFunc("GET /api/v1/saves/{id}/state", s.h(s.saveState))
	m.HandleFunc("PATCH /api/v1/saves/{id}/state", s.h(s.patchState))
	m.HandleFunc("GET /api/v1/saves/{id}/memory", s.h(s.memory))
	m.HandleFunc("GET /api/v1/saves/{id}/memory/{path...}", s.h(s.memoryFile))
	m.HandleFunc("PUT /api/v1/saves/{id}/memory/{path...}", s.h(s.putMemory))
	m.HandleFunc("DELETE /api/v1/saves/{id}/memory/{path...}", s.h(s.deleteMemory))

	// 供应商与模型
	m.HandleFunc("GET /api/v1/providers", s.h(s.listProviders))
	m.HandleFunc("POST /api/v1/providers", s.h(s.createProvider))
	m.HandleFunc("PATCH /api/v1/providers/{id}", s.h(s.updateProvider))
	m.HandleFunc("DELETE /api/v1/providers/{id}", s.h(s.deleteProvider))
	m.HandleFunc("POST /api/v1/providers/{id}/test", s.h(s.testProvider))
	m.HandleFunc("GET /api/v1/providers/{id}/remote-models", s.h(s.remoteModels))
	m.HandleFunc("GET /api/v1/models", s.h(s.listModels))
	m.HandleFunc("POST /api/v1/models", s.h(s.createModel))
	m.HandleFunc("PATCH /api/v1/models/{id}", s.h(s.updateModel))
	m.HandleFunc("GET /api/v1/models/{id}/preset", s.h(s.modelPreset))
	m.HandleFunc("DELETE /api/v1/models/{id}", s.h(s.deleteModel))

	// 用量与访问控制
	m.HandleFunc("GET /api/v1/usage", s.h(s.usageSummary))
	m.HandleFunc("POST /api/v1/auth/login", todo("登录（M6-9）"))
	m.HandleFunc("POST /api/v1/auth/logout", todo("退出登录（M6-9）"))
	m.HandleFunc("PUT /api/v1/auth/password", todo("设置访问密码（M6-9）"))

	// 未匹配的 /api 路径返回 JSON 404；其余交给前端（SPA）
	m.HandleFunc("/api/", s.h(s.apiNotFound))
	m.Handle("/", s.staticHandler())
}
