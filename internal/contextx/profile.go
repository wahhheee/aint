// Package contextx 是上下文工程：能力画像、提示词按需组装、分层构建请求、玩家消息封装。
//
// 分层（《02-技术方案》8.1）：
//
//	system : [L0 平台核心][L1 作品指令]          tools: [L2 工具定义]
//	user   : [L3 常驻资料] ① [L4 阶段块] ②  …
//	……L5 回合记录（只追加）……                                  ③（滚动）
//
// 不变量 1（前缀稳定）：同一阶段内，请求 i 是请求 i+1 的前缀。为此：
//   - 组装只依赖稳定输入（作品版本、存档设置、模型能力、存档初始状态），结果可缓存；
//   - 历史消息以发送形态存储，构建时只读取不改写；序列化统一 jsonx（确定性）；
//   - 任何层都不放时间戳、随机数、请求 ID。
//
// 调试：Built.Fragments 列出每个片段的启用状态与原因（检查器"上下文"页），
// Built.PrefixHash 变化即意味着缓存重建（记录在节点 meta）。
package contextx

import (
	"github.com/wahhheee/yanyi/internal/llm"
	"github.com/wahhheee/yanyi/internal/prompts"
	"github.com/wahhheee/yanyi/internal/save"
	"github.com/wahhheee/yanyi/internal/work"
)

// Profile 计算能力画像（《02-技术方案》8.6）：作品用到了哪些功能、模型能做什么、存档开了什么。
// 结果在一个存档的游玩过程中不变（只依赖稳定输入）。
func Profile(m *work.Model, st save.Settings, caps llm.Caps, helpersAvailable bool) prompts.FeatureSet {
	fs := prompts.FeatureSet{}
	set := func(f prompts.Feature, on bool) {
		if on {
			fs[f] = true
		}
	}
	mf := m.Manifest

	for _, d := range m.Docs {
		fs[prompts.FLore] = true
		set(prompts.FLoreIndex, d.Load == "index")
		set(prompts.FLoreAlways, d.Load == "always")
	}
	set(prompts.FState, !m.State.Empty())
	for _, c := range m.Commands {
		set(prompts.FCommands, c.AIVisible)
		set(prompts.FInputModes, c.HasPresent("mode"))
	}

	for _, a := range m.Assets {
		if a.Kind == "font" || m.UIOnly(a) {
			continue // 字体、封面与主题背景只给界面用，不出现在 AI 的资料里
		}
		fs[prompts.FAssets] = true
		set(prompts.FAssetsBG, a.Kind == "image" && a.Role == "background")
		set(prompts.FAssetsImage, a.Kind == "image" && a.Role != "background")
		set(prompts.FAssetsAudio, a.Kind == "audio")
		set(prompts.FPins, a.Pin)
	}
	set(prompts.FTagScene, mf.Presentation.Stage && fs.Has(prompts.FAssetsBG))
	set(prompts.FTagCG, fs.Has(prompts.FAssetsImage))
	set(prompts.FTagSay, mf.Presentation.Dialogue == "tagged")
	set(prompts.FTagAudio, fs.Has(prompts.FAssetsAudio))
	set(prompts.FTagEffects, mf.Presentation.TextEffects)
	set(prompts.FWorkTags, len(m.Tags) > 0)
	for _, f := range []prompts.Feature{prompts.FTagScene, prompts.FTagCG, prompts.FTagSay, prompts.FTagAudio, prompts.FTagEffects, prompts.FWorkTags} {
		set(prompts.FTags, fs.Has(f))
	}

	set(prompts.FDice, mf.Features.Dice)
	set(prompts.FFreeInput, mf.Features.FreeInput)
	set(prompts.FTimed, mf.Features.TimedChoices)
	set(prompts.FOOC, mf.Features.OOC)
	set(prompts.FMemoryGuide, m.MemoryGuide != "")
	set(prompts.FCompactGuide, m.CompactionGuide != "")
	set(prompts.FVision, caps.Vision)
	set(prompts.FHelpers, st.Helpers && helpersAvailable)
	return fs
}

// EnabledBuiltinTags 返回作品当前可用的内置标签名，前端据此决定渲染白名单
// （未启用的标签按通用规则显示，决策 D-21）。
func EnabledBuiltinTags(fs prompts.FeatureSet) []string {
	var out []string
	add := func(f prompts.Feature, names ...string) {
		if fs.Has(f) {
			out = append(out, names...)
		}
	}
	add(prompts.FTagScene, "scene")
	add(prompts.FTagCG, "cg")
	add(prompts.FTagSay, "say")
	add(prompts.FTagAudio, "bgm", "sfx")
	add(prompts.FTagEffects, "pause", "fx", "aside")
	add(prompts.FOOC, "ooc") // AI 的场外回应（D-30）；在"消息格式"中说明，不计入 tags 特征
	// TODO(P1)：act（有命令时启用）
	return out
}
