package server

import (
	"net/http"
)

func (s *Server) listWorks(w http.ResponseWriter, r *http.Request) error {
	list, err := s.d.Works.List(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func (s *Server) getWork(w http.ResponseWriter, r *http.Request) error {
	d, err := s.d.Works.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

func (s *Server) listWorkSaves(w http.ResponseWriter, r *http.Request) error {
	list, err := s.d.Saves.ListByWork(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}
