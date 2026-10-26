package server

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/wahhheee/aint/internal/store"
	"github.com/wahhheee/aint/internal/work"
)

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) error {
	vs, err := s.d.Works.Versions(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, vs)
	return nil
}

func (s *Server) exportVersion(w http.ResponseWriter, r *http.Request) error {
	var buf bytes.Buffer
	if err := s.d.Works.Export(r.Context(), r.PathValue("id"), r.PathValue("vid"), &buf); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="work.zip"`)
	_, err := w.Write(buf.Bytes())
	return err
}

func (s *Server) deleteWork(w http.ResponseWriter, r *http.Request) error {
	if err := s.storageMaintenance(func() error { return s.d.Works.Delete(r.Context(), r.PathValue("id"), r.PathValue("vid")) }); err != nil {
		return err
	}
	s.collectAfterDelete(r)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) collectAfterDelete(r *http.Request) {
	if s.d.Saves == nil {
		return
	}
	if err := s.storageMaintenance(func() error { _, err := s.collect(r); return err }); err != nil {
		slog.WarnContext(r.Context(), "删除后的存储清理失败，可在设置中重试", "err", err)
	}
}

func (s *Server) collectStorage(w http.ResponseWriter, r *http.Request) error {
	var result store.GCResult
	err := s.storageMaintenance(func() error { var err error; result, err = s.collect(r); return err })
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, result)
	return nil
}

func (s *Server) storageMaintenance(fn func() error) error {
	if s.d.Runtime == nil {
		return fn()
	}
	return s.d.Runtime.WhenIdle(fn)
}

func (s *Server) collect(r *http.Request) (store.GCResult, error) {
	result, err := s.d.Saves.CollectStorage(r.Context(), false)
	if err != nil || s.d.Images == nil {
		return result, err
	}
	cache, err := s.d.Images.CollectCache(r.Context())
	result.Blobs += cache.Blobs
	result.Bytes += cache.Bytes
	return result, err
}

func (s *Server) linkDev(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Path string `json:"path"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	p, err := s.d.Imports.PreviewDirectory(r.Context(), req.Path, true)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, p)
	return nil
}

func (s *Server) upgradeSave(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		VersionID string `json:"version_id"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := s.d.Runtime.Upgrade(r.Context(), r.PathValue("id"), req.VersionID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusAccepted)
	return nil
}

func (s *Server) checkWorkUpdate(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		VersionID string `json:"version_id"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.VersionID == "" {
		var err error
		req.VersionID, err = s.d.Works.LatestVersionID(r.Context(), r.PathValue("id"))
		if err != nil {
			return err
		}
	}
	var res *work.UpdateCheck
	res, err := s.d.Imports.CheckUpdate(r.Context(), r.PathValue("id"), req.VersionID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, res)
	return nil
}
