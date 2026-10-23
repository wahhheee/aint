package server

import (
	"net/http"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/state"
)

// 分支操作（《02-技术方案》6.2）与玩家对记忆、状态的修改。直接改存档的操作经 Runtime.Exclusive 执行：
// 回合进行中返回 409（单一写者），完成后推送 timeline.changed 与面板。

// tree：GET /saves/{id}/tree
func (s *Server) tree(w http.ResponseWriter, r *http.Request) error {
	t, err := s.d.Saves.Tree(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, t)
	return nil
}

// setHead：POST /saves/{id}/head {node_id, mode: here | latest-leaf}
func (s *Server) setHead(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		NodeID string `json:"node_id"`
		Mode   string `json:"mode"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.NodeID == "" {
		return errs.Invalid("缺少 node_id")
	}
	id := r.PathValue("id")
	var head string
	err := s.d.Runtime.Exclusive(r.Context(), id, "head", func() (err error) {
		head, err = s.d.Saves.SetHead(r.Context(), id, body.NodeID, body.Mode)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]string{"head_id": head})
	return nil
}

// regenerate：POST /saves/{id}/nodes/{nid}/regenerate {input?}——省略 input 为重新生成，带 input 为修改玩家输入。
func (s *Server) regenerate(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Input *save.TurnInput `json:"input"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &body); err != nil {
			return err
		}
	}
	turnID, err := s.d.Runtime.Regenerate(r.Context(), r.PathValue("id"), r.PathValue("nid"), body.Input)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"turn_id": turnID})
	return nil
}

// rewrite：POST /saves/{id}/nodes/{nid}/rewrite {text}——修改 AI 正文，生成新版本并成为 head。
func (s *Server) rewrite(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Text string `json:"text"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	id := r.PathValue("id")
	var node *save.Node
	err := s.d.Runtime.Exclusive(r.Context(), id, "rewrite", func() (err error) {
		node, err = s.d.Saves.Rewrite(r.Context(), id, r.PathValue("nid"), body.Text)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]string{"node_id": node.ID, "head_id": node.ID})
	return nil
}

// discard：POST /saves/{id}/nodes/{nid}/discard——丢弃中断的回合。
func (s *Server) discard(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var head string
	err := s.d.Runtime.Exclusive(r.Context(), id, "discard", func() (err error) {
		head, err = s.d.Saves.Discard(r.Context(), id, r.PathValue("nid"))
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]string{"head_id": head})
	return nil
}

// deleteBranch：DELETE /saves/{id}/branches/{nid}——删除不含 head 的分支。
func (s *Server) deleteBranch(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	var n int
	err := s.d.Runtime.Exclusive(r.Context(), id, "delete_branch", func() (err error) {
		n, err = s.d.Saves.DeleteBranch(r.Context(), id, r.PathValue("nid"))
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
	return nil
}

// copySave：POST /saves/{id}/nodes/{nid}/copy——复制为新存档，返回新存档。
func (s *Server) copySave(w http.ResponseWriter, r *http.Request) error {
	sv, err := s.d.Saves.CopyAsNew(r.Context(), r.PathValue("id"), r.PathValue("nid"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, sv)
	return nil
}

// memory：GET /saves/{id}/memory
func (s *Server) memory(w http.ResponseWriter, r *http.Request) error {
	v, err := s.d.Saves.Memory(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

// memoryFile：GET /saves/{id}/memory/{path...}（路径相对 /memory）
func (s *Server) memoryFile(w http.ResponseWriter, r *http.Request) error {
	f, err := s.d.Saves.MemoryFileAt(r.Context(), r.PathValue("id"), "/memory/"+r.PathValue("path"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, f)
	return nil
}

// putMemory：PUT /saves/{id}/memory/{path...} {content}——新建或修改记忆文件（产生编辑节点，下一回合告知主笔）。
func (s *Server) putMemory(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Content *string `json:"content"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Content == nil {
		return errs.Invalid("缺少 content")
	}
	return s.editMemory(w, r, body.Content)
}

// deleteMemory：DELETE /saves/{id}/memory/{path...}
func (s *Server) deleteMemory(w http.ResponseWriter, r *http.Request) error {
	return s.editMemory(w, r, nil)
}

func (s *Server) editMemory(w http.ResponseWriter, r *http.Request, content *string) error {
	id := r.PathValue("id")
	var node *save.Node
	err := s.d.Runtime.Exclusive(r.Context(), id, "edit_memory", func() (err error) {
		node, err = s.d.Saves.EditMemory(r.Context(), id, "/memory/"+r.PathValue("path"), content)
		return err
	})
	if err != nil {
		return err
	}
	out := map[string]string{}
	if node != nil {
		out["node_id"] = node.ID
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// patchState：PATCH /saves/{id}/state {ops}——玩家修改状态（开发者工具，D-31），返回实际发生的变化。
func (s *Server) patchState(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Ops []state.Op `json:"ops"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	id := r.PathValue("id")
	var changes []state.Change
	err := s.d.Runtime.Exclusive(r.Context(), id, "edit_state", func() (err error) {
		_, changes, err = s.d.Saves.EditState(r.Context(), id, body.Ops)
		return err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": changes})
	return nil
}
