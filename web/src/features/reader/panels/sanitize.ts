/** 面板 HTML 的 style 白名单（PanelHTML 使用）。 */

/** style 中允许的属性（排版与装饰；不含定位、尺寸以外的布局与任何可以引入外部资源的属性）。 */
const STYLE_PROPS =
  /^(color|background-color|opacity|font-size|font-weight|font-style|font-variant|letter-spacing|line-height|text-align|text-decoration|text-decoration-(line|style|color)|text-indent|text-transform|white-space|vertical-align|(margin|padding)(-(top|right|bottom|left|inline|block|inline-start|inline-end|block-start|block-end))?|border(-(top|right|bottom|left))?(-(width|style|color))?|border-radius|width|max-width|min-width|height|display|gap)$/
/** 值里不能有的东西：外部资源、旧式表达式、转义（可以用来拼出被拦截的写法）、尖括号。 */
const STYLE_BAD_VALUE = /url\s*\(|image-set|expression|javascript:|@import|\\|[<>]/i
const STYLE_DISPLAY = /^(block|inline|inline-block|flex|inline-flex|grid|none)$/

/** 过滤 style 属性，只保留白名单中的声明。 */
export function sanitizeStyle(style: string): string {
  const out: string[] = []
  for (const decl of style.split(';')) {
    const i = decl.indexOf(':')
    if (i <= 0) continue
    const prop = decl.slice(0, i).trim().toLowerCase()
    const value = decl.slice(i + 1).trim()
    if (!value || !STYLE_PROPS.test(prop) || STYLE_BAD_VALUE.test(value)) continue
    if (prop === 'display' && !STYLE_DISPLAY.test(value)) continue
    out.push(`${prop}: ${value}`)
  }
  return out.join('; ')
}
