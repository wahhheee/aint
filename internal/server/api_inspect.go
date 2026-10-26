package server

import (
	"net/http"
	"strconv"

	"github.com/wahhheee/aint/internal/pkg/errs"
)

// 调试接口按存档隔离，响应不缓存；记录仅存在于当前进程。
func (s *Server) debugSave(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	if _, err := s.d.Saves.Get(r.Context(), id); err != nil {
		return err
	}
	switch r.Method {
	case http.MethodPatch:
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if err := decode(r, &body); err != nil {
			return err
		}
		if body.Enabled == nil {
			return errs.Invalid("缺少 enabled")
		}
		s.d.Runtime.SetDebug(id, *body.Enabled)
	case http.MethodDelete:
		s.d.Runtime.ClearDebug(id)
	}
	writeJSON(w, http.StatusOK, s.d.Runtime.Debug(id))
	return nil
}

func (s *Server) debugCall(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	if _, err := s.d.Saves.Get(r.Context(), id); err != nil {
		return err
	}
	call, err := s.d.Runtime.DebugCall(id, r.PathValue("cid"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, call)
	return nil
}

func (s *Server) debugFile(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	if _, err := s.d.Saves.Get(r.Context(), id); err != nil {
		return err
	}
	q := r.URL.Query()
	offset := 1
	if q.Has("offset") {
		var err error
		offset, err = strconv.Atoi(q.Get("offset"))
		if err != nil || offset < 1 {
			return errs.Invalid("offset 必须是正整数")
		}
	}
	file, err := s.d.Runtime.DebugFile(r.Context(), id, r.PathValue("cid"), q.Get("phase"), q.Get("path"), offset)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, file)
	return nil
}

func (s *Server) debugImage(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	if _, err := s.d.Saves.Get(r.Context(), id); err != nil {
		return err
	}
	hash := blobHash(r.PathValue("hash"))
	data, err := s.d.Runtime.DebugImage(r.Context(), id, r.PathValue("cid"), hash)
	if err != nil {
		return err
	}
	return s.serveMedia(w, r, hash, data)
}
