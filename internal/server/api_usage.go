package server

import (
	"net/http"

	"github.com/wahhheee/aint/internal/usage"
)

// usageSummary 统计累计用量：?save_id= 存档、?work_id= 作品、?node_id= 某个回合或整理节点，可组合；都省略时为全局累计。
func (s *Server) usageSummary(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	sum, err := s.d.Usage.Summarize(r.Context(), usage.Filter{SaveID: q.Get("save_id"), WorkID: q.Get("work_id"), NodeID: q.Get("node_id")})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, sum)
	return nil
}
