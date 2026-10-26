package server

// routes 注册全部路由（《02-技术方案》18.2，前缀 /api/v1）。
func (s *Server) routes() {
	m := s.mux

	// 系统
	m.HandleFunc("GET /api/v1/system/info", s.h(s.systemInfo))
	m.HandleFunc("GET /api/v1/system/health", s.h(s.health))
	m.HandleFunc("GET /api/v1/events", s.h(s.globalEvents)) // 全局事件：作品重载、存档标题（技术方案未单列，补充）

	// 设置
	m.HandleFunc("GET /api/v1/settings", s.h(s.getSettings))
	m.HandleFunc("PATCH /api/v1/settings", s.h(s.patchSettings))
	m.HandleFunc("POST /api/v1/storage/gc", s.h(s.collectStorage))

	// 作品
	m.HandleFunc("GET /api/v1/works", s.h(s.listWorks))
	m.HandleFunc("GET /api/v1/works/{id}", s.h(s.getWork))
	m.HandleFunc("GET /api/v1/works/{id}/saves", s.h(s.listWorkSaves))
	m.HandleFunc("POST /api/v1/works/import", s.h(s.importWork)) // zip / 文件夹 / GitHub / URL / 本机目录
	m.HandleFunc("POST /api/v1/works/import/{task}/confirm", s.h(s.confirmImport))
	m.HandleFunc("DELETE /api/v1/works/import/{task}", s.h(s.cancelImport)) // 放弃导入（技术方案未单列，补充）
	m.HandleFunc("GET /api/v1/works/{id}/versions", s.h(s.listVersions))
	m.HandleFunc("POST /api/v1/works/{id}/check-update", s.h(s.checkWorkUpdate))
	m.HandleFunc("GET /api/v1/works/{id}/versions/{vid}/export", s.h(s.exportVersion))
	m.HandleFunc("GET /api/v1/works/{id}/versions/{vid}/report", s.h(s.versionReport))
	m.HandleFunc("DELETE /api/v1/works/{id}", s.h(s.deleteWork))
	m.HandleFunc("DELETE /api/v1/works/{id}/versions/{vid}", s.h(s.deleteWork))
	m.HandleFunc("POST /api/v1/works/dev-link", s.h(s.linkDev))
	m.HandleFunc("GET /api/v1/assets/{hash}", s.h(s.asset))
	m.HandleFunc("POST /api/v1/uploads", s.h(s.createUpload))
	m.HandleFunc("GET /api/v1/uploads/{hash}", s.h(s.upload))

	// 存档
	m.HandleFunc("GET /api/v1/saves", s.h(s.recentSaves))
	m.HandleFunc("POST /api/v1/saves", s.h(s.createSave))
	m.HandleFunc("GET /api/v1/saves/{id}", s.h(s.getSave))
	m.HandleFunc("PATCH /api/v1/saves/{id}", s.h(s.patchSave))
	m.HandleFunc("PUT /api/v1/saves/{id}/opening", s.h(s.switchOpening))
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
	m.HandleFunc("POST /api/v1/saves/{id}/exit", s.h(s.exitSave))
	m.HandleFunc("POST /api/v1/saves/{id}/title", s.h(s.generateTitle))
	m.HandleFunc("POST /api/v1/saves/{id}/compact", s.h(s.compact))
	m.HandleFunc("POST /api/v1/saves/{id}/upgrade", s.h(s.upgradeSave))
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
	m.HandleFunc("GET /api/v1/auth/status", s.h(s.authStatus))
	m.HandleFunc("POST /api/v1/auth/login", s.h(s.login))
	m.HandleFunc("POST /api/v1/auth/logout", s.h(s.logout))
	m.HandleFunc("PUT /api/v1/auth/password", s.h(s.password))
	m.HandleFunc("DELETE /api/v1/auth/sessions", s.h(s.revokeSessions))
	m.HandleFunc("GET /api/v1/settings/github-token", s.h(s.githubToken))
	m.HandleFunc("PUT /api/v1/settings/github-token", s.h(s.githubToken))

	// 未匹配的 /api 路径返回 JSON 404；其余交给前端（SPA）
	m.HandleFunc("/api/", s.h(s.apiNotFound))
	m.Handle("/", s.staticHandler())
}
