// Package prompts 是平台提示词：片段、启用条件、固定文案，带版本号（决策 D-20、D-21）。
//
// 写作原则（《02-技术方案》第 20 节）：恰到好处、正向帮助、约束用轻语气、结果即事实、风格留给作品。
//
// 按需组装的规则：
//   - 每个片段声明启用条件（Cond），条件只取决于稳定输入（作品版本、存档设置、模型能力），
//     在一个存档的游玩过程中不变，因此裁切不会破坏前缀缓存；
//   - 节内没有任何启用的片段时，整节连同标题省略；
//   - 文本只提到已注册的工具（一致性测试见 contextx）。
//
// 本包是纯数据，不依赖业务包；组装逻辑在 contextx。
// 修改任何提示词文本都必须递增 Version（会让所有存档重建一次缓存，属预期行为）。
package prompts

import (
	"slices"
	"strings"
)

// Version 是平台提示词版本，记录在节点 meta 中，便于把模型行为与提示词版本对应起来。
const Version = "2026.10.20-1"

// Feature 是能力画像中的一个特征（《02-技术方案》8.6）。
type Feature string

const (
	FLore         Feature = "lore"        // 存在任何设定文档
	FLoreIndex    Feature = "lore.index"  // 存在 load: index 的文档
	FLoreAlways   Feature = "lore.always" // 存在 load: always 的文档
	FState        Feature = "state"       // 作品定义了状态
	FCommands     Feature = "commands"    // 存在 AI 可见的命令
	FInputModes   Feature = "input_modes" // 存在 present 含 mode 的命令
	FTags         Feature = "tags"        // 至少一个可用标签（内置或作品）
	FTagScene     Feature = "tags.scene"
	FTagCG        Feature = "tags.cg"
	FTagSay       Feature = "tags.say"
	FTagAudio     Feature = "tags.audio"
	FTagEffects   Feature = "tags.text_effects"
	FWorkTags     Feature = "tags.work" // 作品自定义了样式标签
	FAssets       Feature = "assets"
	FAssetsImage  Feature = "assets.image"
	FAssetsBG     Feature = "assets.background"
	FAssetsAudio  Feature = "assets.audio"
	FPins         Feature = "pins" // 存在常驻图片
	FDice         Feature = "dice"
	FFreeInput    Feature = "free_input"
	FTimed        Feature = "timed_choices"
	FOOC          Feature = "ooc"
	FMemoryGuide  Feature = "memory.guide"
	FCompactGuide Feature = "compaction.guide"
	FVision       Feature = "vision"  // 主笔模型支持看图
	FHelpers      Feature = "helpers" // 存档开启后台助手（且已配置轻量模型）
)

// FeatureSet 是一组已满足的特征。
type FeatureSet map[Feature]bool

// Has 判断是否满足某特征。
func (s FeatureSet) Has(f Feature) bool { return s[f] }

// Signature 返回稳定的特征签名（排序后拼接），用于组装结果的缓存键与检查器展示。
func (s FeatureSet) Signature() string {
	var on []string
	for f, ok := range s {
		if ok {
			on = append(on, string(f))
		}
	}
	slices.Sort(on)
	return strings.Join(on, ",")
}

// Cond 是启用条件：All 全部满足、Any 至少一个满足（为空则不要求）、None 一个都不满足。
type Cond struct {
	All  []Feature
	Any  []Feature
	None []Feature
}

// Eval 判断条件是否满足；不满足时给出原因（检查器"上下文"页展示，如"未满足：state"）。
func (c Cond) Eval(s FeatureSet) (bool, string) {
	for _, f := range c.All {
		if !s.Has(f) {
			return false, "未满足：" + string(f)
		}
	}
	if len(c.Any) > 0 && !slices.ContainsFunc(c.Any, s.Has) {
		return false, "以下均未满足：" + joinFeatures(c.Any)
	}
	for _, f := range c.None {
		if s.Has(f) {
			return false, "已排除：" + string(f)
		}
	}
	return true, ""
}

func joinFeatures(fs []Feature) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = string(f)
	}
	return strings.Join(parts, "、")
}

// Requires 是只要求全部满足的便捷写法。
func Requires(fs ...Feature) Cond { return Cond{All: fs} }
