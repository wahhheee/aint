package server

import (
	"context"
	"net/http"
	"strconv"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/save"
)

// recentSaves 返回最近游玩的存档（书架"继续阅读"）：GET /saves?limit=6
func (s *Server) recentSaves(w http.ResponseWriter, r *http.Request) error {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 6
	}
	list, err := s.d.Saves.Recent(r.Context(), limit)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) createSave(w http.ResponseWriter, r *http.Request) error {
	var p save.CreateParams
	g, err := s.d.Settings.Get(r.Context())
	if err != nil {
		return err
	}
	p.Settings.Helpers = g.Play.Helpers
	if err := decode(r, &p); err != nil {
		return err
	}
	if p.WorkID == "" {
		return errs.Invalid("缺少 work_id")
	}
	settings, err := s.newSaveSettings(r.Context(), p.Settings)
	if err != nil {
		return err
	}
	p.Settings = settings
	sv, err := s.d.Saves.Create(r.Context(), p)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, sv)
	return nil
}

// newSaveSettings 用全局设置补全新存档设置中未填写的项。
func (s *Server) newSaveSettings(ctx context.Context, in save.Settings) (save.Settings, error) {
	g, err := s.d.Settings.Get(ctx)
	if err != nil {
		return in, err
	}
	if in.NarratorModel == "" {
		in.NarratorModel = g.Models.Narrator
	}
	if s.d.Providers.FakeMode() {
		in.NarratorModel = "fake"
	}
	if in.NarratorModel == "" {
		return in, errs.New(errs.CodeModelMissing, "开始之前，需要连接一个模型")
	}
	if in.Pace == "" {
		in.Pace = g.Play.Pace
	}
	if in.Reasoning == "" {
		in.Reasoning = g.Play.Reasoning
	}
	if in.CacheTTL == "" {
		in.CacheTTL = g.Play.CacheTTL
	}
	return in, in.Validate()
}

// getSave 返回阅读页所需的呈现信息（存档 + 作品界面配置 + 模型摘要）。
func (s *Server) getSave(w http.ResponseWriter, r *http.Request) error {
	p, err := s.d.Runtime.Presentation(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (s *Server) patchSave(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Title    *string     `json:"title"`
		Settings jsonx.Value `json:"settings"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	id := r.PathValue("id")
	if len(body.Settings) > 0 {
		if err := s.d.Runtime.PatchSettings(r.Context(), id, body.Settings); err != nil {
			return err
		}
	}
	if body.Title != nil {
		if err := s.d.Saves.Rename(r.Context(), id, *body.Title); err != nil {
			return err
		}
	}
	sv, err := s.d.Saves.Get(r.Context(), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, sv)
	return nil
}

func (s *Server) deleteSave(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if err := s.d.Runtime.Exclusive(r.Context(), id, "delete", func() error { return s.d.Saves.Delete(r.Context(), id) }); err != nil {
		return err
	}
	s.collectAfterDelete(r)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// timeline：GET /saves/{id}/timeline?before=<node_id>&limit=50
func (s *Server) timeline(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	tl, err := s.d.Saves.Timeline(r.Context(), r.PathValue("id"), q.Get("before"), limit)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, tl)
	return nil
}

func (s *Server) panels(w http.ResponseWriter, r *http.Request) error {
	panels, err := s.d.Runtime.Panels(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, event.PanelsData{Panels: panels})
	return nil
}

// saveState：GET /saves/{id}/state，head 处的状态与状态定义（状态抽屉、调试）。
func (s *Server) saveState(w http.ResponseWriter, r *http.Request) error {
	v, err := s.d.Runtime.State(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) saveEvents(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	if _, err := s.d.Saves.Get(r.Context(), id); err != nil {
		return err
	}
	return s.serveEvents(w, r, event.SaveTopic(id))
}

// submitTurn 提交玩家输入，回合在后台运行，进度经 SSE 推送；立即返回回合 ID（202）。
func (s *Server) submitTurn(w http.ResponseWriter, r *http.Request) error {
	var in save.TurnInput
	if err := decode(r, &in); err != nil {
		return err
	}
	turnID, err := s.d.Runtime.Submit(r.Context(), r.PathValue("id"), in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"turn_id": turnID})
	return nil
}

func (s *Server) cancelTurn(w http.ResponseWriter, r *http.Request) error {
	if err := s.d.Runtime.Cancel(r.PathValue("id"), r.PathValue("tid")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) generateTitle(w http.ResponseWriter, r *http.Request) error {
	if err := s.d.Runtime.GenerateTitle(r.Context(), r.PathValue("id"), false); err != nil {
		return err
	}
	w.WriteHeader(http.StatusAccepted)
	return nil
}

func (s *Server) exitSave(w http.ResponseWriter, r *http.Request) error {
	if err := s.d.Runtime.GenerateTitle(r.Context(), r.PathValue("id"), true); err != nil {
		return err
	}
	w.WriteHeader(http.StatusAccepted)
	return nil
}
