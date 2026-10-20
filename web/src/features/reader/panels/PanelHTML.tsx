/**
 * 面板内容（《02-技术方案》17）：服务端用 Liquid 渲染出 HTML，这里负责净化与展示。
 *
 *   1. DOMPurify 白名单：常用排版标签、表格、ui-* 自定义元素、class；禁止脚本、事件属性、iframe、表单、外链；
 *   2. html-react-parser 把 ui-* 元素替换为 React 组件（uiComponents.tsx）。
 *
 * 白名单集中在这一个文件里，安全审查只需看这里。
 * TODO(M2-8)：style 属性只允许颜色、字重、边距、对齐等安全属性；<img> 只允许 asset: 方案（映射为素材地址）。
 */
import DOMPurify from 'dompurify'
import parse, { domToReact, Element, type DOMNode, type HTMLReactParserOptions } from 'html-react-parser'
import { useMemo } from 'react'

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
]
const UI_ATTRS = /^(label|value|max|icon|command|args|confirm|title|open|text|src|alt|size|variant)$/

const PURIFY_CONFIG = {
  ALLOWED_TAGS,
  ALLOWED_ATTR: ['class', 'title', 'colspan', 'rowspan'],
  ALLOW_DATA_ATTR: true,
  CUSTOM_ELEMENT_HANDLING: {
    tagNameCheck: /^ui-[a-z]+$/,
    attributeNameCheck: UI_ATTRS,
    allowCustomizedBuiltInElements: false,
  },
}

const parserOptions: HTMLReactParserOptions = {
  replace(node) {
    if (!(node instanceof Element) || !node.name.startsWith('ui-')) return
    const Comp = UI_COMPONENTS[node.name]
    if (!Comp) return <></>
    return <Comp attrs={node.attribs}>{domToReact(node.children as DOMNode[], parserOptions)}</Comp>
  },
}

export function PanelHTML({ html }: { html: string }) {
  const content = useMemo(() => parse(DOMPurify.sanitize(html, PURIFY_CONFIG), parserOptions), [html])
  return <div className="panel-content">{content}</div>
}
