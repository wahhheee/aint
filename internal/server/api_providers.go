package server

import (
	"errors"
	"io"
	"net/http"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/provider"
)

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) error {
	list, err := s.d.Providers.List(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) createProvider(w http.ResponseWriter, r *http.Request) error {
	var in provider.ProviderInput
	if err := decode(r, &in); err != nil {
		return err
	}
	v, err := s.d.Providers.Create(r.Context(), in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, v)
	return nil
}

func (s *Server) updateProvider(w http.ResponseWriter, r *http.Request) error {
	var in provider.ProviderInput
	if err := decode(r, &in); err != nil {
		return err
	}
	v, err := s.d.Providers.Update(r.Context(), r.PathValue("id"), in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func (s *Server) deleteProvider(w http.ResponseWriter, r *http.Request) error {
	if err := s.d.Providers.Delete(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// testProvider 执行连接测试；请求体可省略（用该供应商的第一个模型）。
func (s *Server) testProvider(w http.ResponseWriter, r *http.Request) error {
	var in provider.TestInput
	body, err := readBody(r)
	if err != nil {
		return err
	}
	if len(body) > 0 {
		if err := jsonx.Unmarshal(body, &in); err != nil && !errors.Is(err, io.EOF) {
			return errs.Invalid("请求格式不正确：%v", err)
		}
	}
	report, err := s.d.Providers.Test(r.Context(), r.PathValue("id"), in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, report)
	return nil
}

func (s *Server) remoteModels(w http.ResponseWriter, r *http.Request) error {
	list, err := s.d.Providers.RemoteModels(r.Context(), r.PathValue("id"))
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

func (s *Server) createModel(w http.ResponseWriter, r *http.Request) error {
	var in provider.ModelInput
	if err := decode(r, &in); err != nil {
		return err
	}
	m, err := s.d.Providers.CreateModel(r.Context(), in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, m)
	return nil
}

func (s *Server) updateModel(w http.ResponseWriter, r *http.Request) error {
	var in provider.ModelInput
	if err := decode(r, &in); err != nil {
		return err
	}
	m, err := s.d.Providers.UpdateModel(r.Context(), r.PathValue("id"), in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, m)
	return nil
}

func (s *Server) deleteModel(w http.ResponseWriter, r *http.Request) error {
	if err := s.d.Providers.DeleteModel(r.Context(), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
