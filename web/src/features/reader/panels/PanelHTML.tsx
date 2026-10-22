/**
 * 面板内容（《02-技术方案》17）：服务端用 Liquid 渲染出 HTML，这里负责净化与展示。
 *
 *   1. DOMPurify 白名单：常用排版标签、表格、图片、ui-* 自定义元素、class、data-*；禁止脚本、事件属性、iframe、表单、外链；
 *      style 只保留颜色、字号字重、边距、对齐等排版属性（sanitizeStyle），值中不能有 url() 等外部资源；
 *   2. html-react-parser 把 ui-* 元素替换为 React 组件（uiComponents.tsx）；<img> 只接受 asset: 方案（模板中的
 *      {{ "lin" | asset }}），映射为作品素材地址，其余图片丢弃。
 *
 * 白名单集中在这一个文件里，安全审查只需看这里。
 */
import DOMPurify from 'dompurify'
import parse, { domToReact, Element, type DOMNode, type HTMLReactParserOptions } from 'html-react-parser'
import { useContext, useMemo } from 'react'

import { imageURL, MarkupContext, type MarkupContextValue } from '@/markup/context'

import { sanitizeStyle } from './sanitize'

import { UI_COMPONENTS } from './uiComponents'

const ALLOWED_TAGS = [
  'p',
  'br',
  'hr',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'ul',
  'ol',
  'li',
  'strong',
  'em',
  'b',
  'i',
  'u',
  's',
  'span',
  'div',
  'small',
  'blockquote',
  'code',
  'table',
  'thead',
  'tbody',
  'tr',
  'th',
  'td',
  'img',
]
const UI_ATTRS = /^(label|value|max|icon|command|args|confirm|title|open|text|src|alt|size|variant)$/

/**
 * URI 类属性只接受 asset: 方案或不带方案的值（如 width="80"）；http(s)、data:、javascript: 等一律去掉。
 * 不带方案的 <img src> 也会在 replace 中被丢弃：图片只能引用作品素材。
 */
const ALLOWED_URI = /^(?:asset:|[^a-z]|[a-z+.-]+(?:[^a-z+.\-:]|$))/i

const PURIFY_CONFIG = {
  ALLOWED_TAGS,
  ALLOWED_ATTR: ['class', 'title', 'colspan', 'rowspan', 'style', 'src', 'alt', 'width', 'height'],
  ALLOW_DATA_ATTR: true,
  ALLOWED_URI_REGEXP: ALLOWED_URI,
  CUSTOM_ELEMENT_HANDLING: {
    tagNameCheck: /^ui-[a-z]+$/,
    attributeNameCheck: UI_ATTRS,
    allowCustomizedBuiltInElements: false,
  },
}

// 面板专用的 DOMPurify 实例：钩子只作用于面板，不影响其他用途
const purify = DOMPurify(window)
purify.addHook('uponSanitizeAttribute', (_node, data) => {
  if (data.attrName !== 'style') return
  data.attrValue = sanitizeStyle(data.attrValue)
  if (!data.attrValue) data.keepAttr = false
})

/** 渲染为块级元素的 ui-* 组件（不能出现在 <p> 中） */
const BLOCK_UI = new Set(['ui-stat', 'ui-bar', 'ui-collapse', 'ui-image'])

/** 把 style 文本转成 React 的 style 对象（已经过 sanitizeStyle）。 */
function styleObject(style: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const decl of style.split(';')) {
    const i = decl.indexOf(':')
    if (i <= 0) continue
    const prop = decl
      .slice(0, i)
      .trim()
      .replace(/-([a-z])/g, (_, c: string) => c.toUpperCase())
    out[prop] = decl.slice(i + 1).trim()
  }
  return out
}

function parserOptions(ctx: MarkupContextValue): HTMLReactParserOptions {
  const opts: HTMLReactParserOptions = {
    replace(node) {
      if (!(node instanceof Element)) return
      // Markdown 会把写在一行里的 <ui-stat …></ui-stat> 包进 <p>（CommonMark 只把单独成行的开标签视为 HTML 块）；
      // 块级组件放在 <p> 中是非法嵌套，这里把这样的段落换成 <div>。
      if (node.name === 'p' && node.children.some((c) => c instanceof Element && BLOCK_UI.has(c.name))) {
        return <div>{domToReact(node.children as DOMNode[], opts)}</div>
      }
      if (node.name === 'img') {
        const url = node.attribs.src?.startsWith('asset:') ? imageURL(node.attribs.src, ctx) : undefined
        if (!url) return <></>
        const { style, width, height, alt, title } = node.attribs
        return (
          <img
            src={url}
            alt={alt ?? ''}
            title={title}
            width={width}
            height={height}
            style={style ? styleObject(style) : undefined}
            className="panel-img"
          />
        )
      }
      if (!node.name.startsWith('ui-')) return
      const Comp = UI_COMPONENTS[node.name]
      if (!Comp) return <></>
      return <Comp attrs={node.attribs}>{domToReact(node.children as DOMNode[], opts)}</Comp>
    },
  }
  return opts
}

export function PanelHTML({ html }: { html: string }) {
  const ctx = useContext(MarkupContext)
  const content = useMemo(() => parse(purify.sanitize(html, PURIFY_CONFIG), parserOptions(ctx)), [html, ctx])
  return <div className="panel-content">{content}</div>
}
