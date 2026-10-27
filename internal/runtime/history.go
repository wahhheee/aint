package runtime

import (
	"context"
	"fmt"

	"github.com/wahhheee/aint/internal/contextx"
	"github.com/wahhheee/aint/internal/llm"
	"github.com/wahhheee/aint/internal/prompts"
	"github.com/wahhheee/aint/internal/save"
	"github.com/wahhheee/aint/internal/state"
	"github.com/wahhheee/aint/internal/work"
)

// epochContext 是请求开头的阶段块（L4）与其后的历史回合（L5，《02-技术方案》8.1）。
//
//	第 0 阶段：  [开局阶段块（即时渲染）] [全部回合]
//	整理之后：  [最近一次整理的阶段块] [保留的回合（keep_from 起）] [整理之后的回合]
//
// 保留的回合产生于整理之前：它们之前的历史已被摘要替换，所以去掉其中的推理与协议原生内容
// （Claude 等模型的推理签名依赖完整的历史，历史改变后原样回传会被拒绝），文字与工具调用保留。
// 历史伪调用在发送副本中清理；相同的已提交历史经清理后得到同样的字节。
// 清理位置起的原生内容与推理被丢弃，存储消息不变。
type epochContext struct {
	n          int             // 阶段序号：进度线上整理节点的个数
	block      string          // L4 阶段块（第 0 阶段可能为空）
	history    []llm.Message   // 阶段块之后的历史消息（发送形态）
	turns      []*save.Node    // 上下文中的回合（保留的 + 阶段内的），按回合号升序
	msgs       [][]llm.Message // 各回合的消息（已去除伪调用示范），与 turns 一一对应
	sizes      []int           // 各回合消息的 token 估算，与 turns 一一对应
	compaction *save.Node      // 最近的整理节点；nil 表示第 0 阶段
}

// loadContext 按进度线组装阶段块与历史消息。fs 用于第 0 阶段块的裁切（与请求的能力画像一致）。
func (m *Manager) loadContext(ctx context.Context, w *work.Model, fs prompts.FeatureSet, line []*save.Node) (*epochContext, error) {
	ec := &epochContext{}
	last := -1
	for i, n := range line {
		if n.Kind == save.KindCompaction {
			last, ec.n = i, ec.n+1
		}
	}
	if last >= 0 {
		ec.compaction = line[last]
		data, err := m.d.Saves.Blob(ctx, ec.compaction.EpochHash)
		if err != nil {
			return nil, fmt.Errorf("读取阶段块失败: %w", err)
		}
		ec.block = string(data)
		if w.IsDev && ec.compaction.Meta.WorkHash != w.TreeHash {
			st, err := m.d.Saves.LoadState(ctx, ec.compaction.StateHash)
			if err != nil {
				return nil, err
			}
			st, _ = state.Migrate(w.State, st)
			memory, err := m.d.Saves.MemoryIndex(ctx, ec.compaction.MemoryHash)
			if err != nil {
				return nil, err
			}
			ec.block = contextx.RenderEpoch(fs, w.State, contextx.EpochInput{
				N: ec.n, FromTurn: ec.compaction.TurnNo + 1, KeptFrom: ec.compaction.Meta.KeepFromTurn,
				KeptTo: ec.compaction.TurnNo, Summary: between(ec.block, "<summary>\n", "\n</summary>"), State: st, Memory: memory,
			})
		}
	} else {
		root := line[0]
		memIndex, err := m.d.Saves.MemoryIndex(ctx, root.MemoryHash)
		if err != nil {
			return nil, err
		}
		initState, err := m.d.Saves.LoadState(ctx, root.StateHash)
		if err != nil {
			return nil, err
		}
		initState, _ = state.Migrate(w.State, initState)
		ec.block = contextx.RenderEpoch0(fs, w.State, initState, memIndex)
	}

	keeping := last < 0 // 第 0 阶段：全部回合都在上下文中
	cleaner := toolHistoryCleaner{specs: m.d.Tools.Specs(fs, w), tags: w.Tags}
	for i, n := range line {
		if n.Kind != save.KindTurn {
			continue
		}
		kept := i < last
		if kept {
			if n.ID == ec.compaction.KeepFrom {
				keeping = true
			}
			if !keeping {
				continue
			}
		}
		msgs, err := m.d.Saves.LoadMessages(ctx, n.MessagesHash)
		if err != nil {
			return nil, err
		}
		msgs = cleaner.clean(msgs)
		ec.msgs = append(ec.msgs, msgs)
		if kept {
			msgs = stripNative(msgs)
		}
		ec.history = append(ec.history, msgs...)
		ec.turns = append(ec.turns, n)
		ec.sizes = append(ec.sizes, contextx.EstimateMessages(msgs))
	}
	ec.history = repairOrphans(ec.history)
	return ec, nil
}

// stripNative 去掉助手消息中的推理摘要与协议原生内容（返回新切片，不修改原消息）。
func stripNative(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role != llm.RoleAssistant {
			out = append(out, m)
			continue
		}
		nm := llm.Message{Role: m.Role}
		for _, p := range m.Parts {
			if p.Kind != llm.PartReasoning {
				nm.Parts = append(nm.Parts, p)
			}
		}
		if len(nm.Parts) > 0 {
			out = append(out, nm)
		}
	}
	return out
}

// repairOrphans 修复孤儿工具结果（8.4）：工具结果对应的调用不在上下文中时（整理后保留原文的起点），
// 改写为文字"（玩家对上一轮选项的回应：…）"。返回新切片，不修改原消息。
func repairOrphans(msgs []llm.Message) []llm.Message {
	calls := map[string]bool{}
	out := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		out[i] = m
		if m.Role == llm.RoleAssistant {
			for _, c := range m.ToolCalls() {
				calls[c.ID] = true
			}
			continue
		}
		orphan := false
		for _, p := range m.Parts {
			if p.Kind == llm.PartToolResult && p.ToolResult != nil && !calls[p.ToolResult.CallID] {
				orphan = true
			}
		}
		if !orphan {
			continue
		}
		parts := make([]llm.Part, 0, len(m.Parts))
		for _, p := range m.Parts {
			if p.Kind == llm.PartToolResult && p.ToolResult != nil && !calls[p.ToolResult.CallID] {
				var text string
				for _, rp := range p.ToolResult.Parts {
					if rp.Kind == llm.PartText {
						text += rp.Text
					}
				}
				parts = append(parts, llm.Text(fmt.Sprintf(prompts.OrphanChoiceResult, text)))
				continue
			}
			parts = append(parts, p)
		}
		out[i] = llm.Message{Role: m.Role, Parts: parts}
	}
	return out
}
