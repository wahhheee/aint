package save

import (
	"context"
	"database/sql"
	"strings"

	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/pkg/errs"
	"github.com/wahhheee/aint/internal/pkg/ids"
	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/state"
	"github.com/wahhheee/aint/internal/store"
	"github.com/wahhheee/aint/internal/work"
)

// openingContent 在写库之前完成校验与模板渲染；失败时存档保持原样。
type openingContent struct {
	state    map[string]any
	form     map[string]any
	memory   MemoryManifest
	display  []DisplayItem
	messages []llm.Message
}

// HasUnfinishedInteraction 检查所有分支，避免回退后切换开场时遗漏清除提示。
func (s *Service) HasUnfinishedInteraction(ctx context.Context, saveID string) (bool, error) {
	return hasUnfinishedInteraction(ctx, s.db.Read(), saveID)
}

func (s *Service) prepareOpening(m *work.Model, op *work.Opening, form map[string]any) (*openingContent, error) {
	st, err := op.State(m.State)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeWorkInvalid, "开场初始状态不合法")
	}
	init := &openingContent{state: st, form: map[string]any{}, memory: MemoryManifest{V: 1, Files: map[string]MemoryFile{}}}
	for _, f := range m.Manifest.NewGame.Fields {
		v, ok := form[f.Key]
		if !ok || v == nil {
			if f.Required {
				return nil, errs.Invalid("请填写「%s」", f.Label)
			}
			v = f.Default
		}
		if v == nil {
			continue
		}
		if err := state.Set(m.State, st, f.Key, v); err != nil {
			return nil, errs.Invalid("「%s」填写不正确：%v", f.Label, err)
		}
		init.form[f.Key] = v
	}
	for path, ref := range m.Files {
		if rest, ok := strings.CutPrefix(path, "memory/"); ok {
			init.memory.Files["/memory/"+rest] = MemoryFile{Blob: ref.Blob, Size: ref.Size}
		}
	}
	if op.Mode != "static" {
		return init, nil
	}
	body, err := s.tmpl.RenderWith(op.File, op.Body, map[string]any{"state": st}, m.Partials)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeWorkInvalid, "开场模板渲染失败")
	}
	markup := strings.TrimSpace(body)
	init.display = []DisplayItem{{T: "text", Markup: markup}}
	parts := []llm.Part{llm.Text(markup)}
	if len(op.Choices) > 0 {
		choices := DisplayItem{T: "choices", Free: m.Manifest.Features.FreeInput}
		for _, c := range op.Choices {
			choices.Items = append(choices.Items, ChoiceItem{ID: c.ID, Label: c.Label, Detail: c.Detail})
		}
		init.display = append(init.display, choices)
		parts = append(parts, llm.Part{Kind: llm.PartToolCall, ToolCall: &llm.ToolCall{
			ID: "static_opening_choices", Name: "offer_choices",
			Args: jsonx.MustMarshal(struct {
				Choices []ChoiceItem `json:"choices"`
			}{Choices: choices.Items}),
		}})
	}
	init.messages = []llm.Message{
		{Role: llm.RoleUser, Parts: []llm.Part{llm.Text(prompts.StaticOpeningInput)}},
		{Role: llm.RoleAssistant, Parts: parts},
	}
	return init, nil
}

func (s *Service) startOpening(ctx context.Context, tx *sql.Tx, sv *Save, m *work.Model, init *openingContent) error {
	stateHash, err := s.blobs.PutJSON(ctx, tx, init.state)
	if err != nil {
		return err
	}
	memoryHash, err := s.blobs.PutJSON(ctx, tx, init.memory)
	if err != nil {
		return err
	}
	root := &Node{ID: ids.New(), SaveID: sv.ID, Kind: KindRoot, Status: StatusCommitted,
		StateHash: stateHash, MemoryHash: memoryHash, WorkVersionID: sv.WorkVersionID, CreatedAt: store.NowMillis()}
	if err := insertNode(ctx, tx, root); err != nil {
		return err
	}
	sv.HeadNodeID, sv.TurnCount = root.ID, 0
	if len(init.display) == 0 {
		return setHead(ctx, tx, sv.ID, root.ID, true)
	}
	node, err := s.commitTx(ctx, tx, TurnCommit{
		SaveID: sv.ID, Parent: root, TurnNo: 1, Input: TurnInput{Kind: InputOpening},
		Messages: init.messages, Display: init.display, StateHash: stateHash, MemoryHash: memoryHash,
		WorkVersionID: sv.WorkVersionID, Status: StatusCommitted, PlainText: TranscriptBody(init.display, m.SpeakerName),
		Meta: NodeMeta{Stop: "static_opening", Opening: true},
	})
	if err != nil {
		return err
	}
	sv.HeadNodeID, sv.TurnCount = node.ID, 1
	return nil
}

// SwitchOpening 在首轮完成之前重建开场。调用方必须持有 runtime 的写者槽位。
// expectedHead 防止旧页面清除尚未展示给玩家的内容；用量保留在原存档。
func (s *Service) SwitchOpening(ctx context.Context, saveID, openingID, expectedHead string) error {
	sv, err := s.Get(ctx, saveID)
	if err != nil {
		return err
	}
	if sv.OpeningLocked {
		return errs.Conflict("首轮对话已完成，不能再切换开场")
	}
	if openingID == "" {
		return errs.Invalid("请选择开场")
	}
	if sv.HeadNodeID != expectedHead {
		return errs.Conflict("存档已变化，请刷新后重新选择开场")
	}
	if sv.OpeningID == openingID {
		return nil
	}
	m, err := s.works.Model(ctx, sv.WorkVersionID)
	if err != nil {
		return err
	}
	op, err := m.Opening(openingID)
	if err != nil {
		return errs.Invalid("%v", err)
	}
	init, err := s.prepareOpening(m, op, sv.InitialForm)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx *sql.Tx) error {
		if err := resetOpening(ctx, tx, saveID, openingID, expectedHead); err != nil {
			return err
		}
		sv.OpeningID = openingID
		return s.startOpening(ctx, tx, sv, m, init)
	})
}
