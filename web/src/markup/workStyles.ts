/**
 * 作品样式标签 → CSS（《02-技术方案》17）。生成 .mk-<tag> 类，作用域限定在阅读区 .work-root 内。
 *
 * 深色模式颜色：作品写了 dark_color 用作品的；没写时服务端编译作品已按对比度 ≥ 4.5 算好（work/color.go），
 * 这里的 color-mix 提亮只是兜底。
 */
import type { TagDef } from '@/api/types'

import type { Inline } from './parse'

const SAFE_COLOR = /^#[0-9a-fA-F]{3,8}$/
const SAFE_CLASS = /^[a-zA-Z][\w-]*$/

export function workTagCSS(tags: TagDef[]): string {
  const rules: string[] = []
  for (const t of tags) {
    if (!/^[a-z][a-z0-9-]*$/.test(t.name)) continue
    const s = t.style ?? {}
    const decl: string[] = []
    if (s.color && SAFE_COLOR.test(s.color)) decl.push(`color:${s.color}`)
    if (s.background && SAFE_COLOR.test(s.background)) decl.push(`background:${s.background}`, 'padding:0 .15em', 'border-radius:2px')
    if (s.weight === 'bold') decl.push('font-weight:700')
    if (s.italic) decl.push('font-style:italic')
    if (s.underline && s.underline !== 'none') decl.push(`text-decoration:underline ${s.underline}`, 'text-underline-offset:.25em')
    if (s.size) decl.push(`font-size:${clamp(s.size, 0.8, 1.6)}em`)
    if (s.letter_spacing) decl.push(`letter-spacing:${clamp(s.letter_spacing, 0, 0.5)}em`)
    if (s.opacity !== undefined) decl.push(`opacity:${clamp(s.opacity, 0.3, 1)}`)
    if (s.font === 'mono') decl.push('font-family:ui-monospace,monospace')
    else if (s.font === 'heading' || s.font === 'body') decl.push('font-family:var(--reading-font)')
    rules.push(`.work-root .mk-${t.name}{${decl.join(';')}}`)

    if (s.color && SAFE_COLOR.test(s.color)) {
      const dark = s.dark_color && SAFE_COLOR.test(s.dark_color) ? s.dark_color : `color-mix(in oklch, ${s.color} 55%, white)`
      rules.push(`.dark .work-root .mk-${t.name}{color:${dark}}`)
    }
  }
  return rules.join('\n')
}

/** 标签元素需要附加的类名（内置特效与作品 custom.css 中的类）。 */
export function workTagClasses(t: TagDef): string {
  const cls = [`mk-${t.name}`]
  if (t.style?.effect) cls.push(`mk-effect-${t.style.effect}`)
  if (t.class && SAFE_CLASS.test(t.class)) cls.push(t.class)
  return cls.join(' ')
}

function clamp(n: number, lo: number, hi: number) {
  return Math.min(hi, Math.max(lo, n))
}

/** 样式标签的悬停提示：模板中的 {{ text }} 换成标签内的文字（《03-作品格式规范》10.1）。 */
export function tooltipText(template: string, children: Inline[]): string {
  return template.replace(/\{\{\s*text\s*\}\}/g, () => plainText(children))
}

function plainText(nodes: Inline[]): string {
  return nodes.map((n) => (n.type === 'text' || n.type === 'raw' ? n.text : plainText(n.children))).join('')
}
