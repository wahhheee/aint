package server

import (
	"net/http"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/pkg/errs"
)

func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, s.d.Info)
	return nil
}

// health 也用于单实例检测：新启动的进程据此判断已有实例是否仍在运行（app/instance.go）。
func (s *Server) health(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "instance_id": s.d.Info.InstanceID})
	return nil
}

func (s *Server) globalEvents(w http.ResponseWriter, r *http.Request) error {
	return s.serveEvents(w, r, event.GlobalTopic)
}

func (s *Server) apiNotFound(w http.ResponseWriter, r *http.Request) error {
	return errs.NotFound("没有这个接口：%s %s", r.Method, r.URL.Path)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) error {
	st, err := s.d.Settings.Get(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	st, err := s.d.Settings.Patch(r.Context(), body)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) error {
	list, err := s.d.Providers.List(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) error {
	list, err := s.d.Providers.Models(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}
