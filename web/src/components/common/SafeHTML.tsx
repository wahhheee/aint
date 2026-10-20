/**
 * 展示来自作品的 HTML（README 等）：先经 DOMPurify 净化。作品内容不执行（不变量 6）。
 * 面板 HTML 有更严格的白名单与 ui-* 组件映射，见 features/reader/panels/PanelHTML.tsx。
 */
import DOMPurify from 'dompurify'
import { useMemo } from 'react'

import { cn } from '@/lib/utils'

export function SafeHTML({ html, className }: { html: string; className?: string }) {
  const clean = useMemo(() => DOMPurify.sanitize(html, { FORBID_TAGS: ['style', 'form', 'iframe'], FORBID_ATTR: ['style'] }), [html])
  return <div className={cn('readme', className)} dangerouslySetInnerHTML={{ __html: clean }} />
}
