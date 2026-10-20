// Package panel 渲染作品面板（《02-技术方案》第 17 节）。
//
// 服务端用 Liquid 渲染全部面板，输出为空的面板不显示；结果带内容哈希，前端按哈希做差量更新。
// 渲染时机：回合提交、回合内状态变化（去抖 100ms，TODO(M2-8)）、edit 节点、切换 head。
// 净化在前端（DOMPurify 白名单 + ui-* 组件），这里只负责模板与 Markdown。
package panel

import (
	"log/slog"
	"strings"

	"github.com/wahhheee/yanyi/internal/event"
	"github.com/wahhheee/yanyi/internal/store"
	"github.com/wahhheee/yanyi/internal/tmpl"
	"github.com/wahhheee/yanyi/internal/work"
)

// Vars 是面板模板可用的变量（《03-作品格式规范》11.2）。
type Vars struct {
	State     map[string]any
	Turn      int // 当前所在回合的序号（模板中为 turn.no）
	WorkTitle string
	SaveTitle string
}

// Render 渲染作品的全部面板。单个面板出错时以错误提示代替其内容（开发者能看到原因），不影响其他面板。
func Render(eng *tmpl.Engine, m *work.Model, v Vars) []event.PanelRender {
	vars := map[string]any{
		"state": v.State,
		"turn":  TurnVar(v.Turn),
		"work":  map[string]any{"title": v.WorkTitle},
		"save":  map[string]any{"title": v.SaveTitle},
	}
	out := []event.PanelRender{}
	for _, p := range m.Layout.Panels {
		html, err := eng.Render(p.Template, p.Source, vars)
		if err != nil {
			slog.Warn("面板渲染失败", "panel", p.ID, "err", err)
			html = `<p class="panel-error">面板渲染失败：` + escape(err.Error()) + `</p>`
		} else {
			if strings.TrimSpace(html) == "" {
				continue // 渲染结果为空的面板不显示
			}
			if p.Format != "html" {
				html = tmpl.Markdown(html)
			}
		}
		out = append(out, event.PanelRender{ID: p.ID, HTML: html, Hash: store.HashBytes([]byte(html))})
	}
	return out
}

// TurnVar 构造作者模板中的 turn 变量（D-36）：对象形式，turn.no 为回合序号，以后在这里扩展字段。
// 面板与命令模板共用，保证两处字段一致。
func TurnVar(no int) map[string]any {
	return map[string]any{"no": no}
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
