import { Copy, FileText } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import type { DebugPart } from '@/api/types'
import { Button } from '@/components/ui/button'
import { pretty } from './format'

/** 大报文按需展开；复制始终包含当前保留的全部内容。 */
export function CodeBlock({ value, label }: { value: unknown; label: string }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const [formatted, setFormatted] = useState(false)
  const original = typeof value === 'string' ? value : pretty(value)
  const prettyText = pretty(value)
  const text = formatted ? prettyText : original
  const long = text.length > 12000
  return (
    <section className="min-w-0 overflow-hidden rounded-lg border border-rule bg-paper-raised">
      <div className="flex items-center justify-between gap-3 border-b border-rule px-3 py-1.5">
        <span className="text-xs text-ink-muted">{label}</span>
        {original !== prettyText && (
          <Button variant="ghost" size="xs" className="ml-auto text-xs" onClick={() => setFormatted(!formatted)}>
            {t(formatted ? 'debug.original' : 'debug.formatJSON')}
          </Button>
        )}
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label={t('debug.copy', { label })}
          onClick={() => {
            void navigator.clipboard
              .writeText(text)
              .then(() => toast(t('common.copied')))
              .catch(() => toast.error(t('debug.copyFailed')))
          }}
        >
          <Copy />
        </Button>
      </div>
      <pre className="max-h-[52vh] overflow-auto p-3 font-mono text-xs leading-6 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">
        {long && !expanded ? text.slice(0, 12000) : text || t('debug.emptyContent')}
      </pre>
      {long && (
        <Button variant="ghost" className="w-full rounded-none border-t border-rule text-xs" onClick={() => setExpanded(!expanded)}>
          {t(expanded ? 'debug.collapse' : 'debug.expand', { count: text.length.toLocaleString() })}
        </Button>
      )}
    </section>
  )
}

export function EmptyDebug({ text }: { text: string }) {
  return (
    <div className="flex min-h-40 flex-col items-center justify-center gap-3 px-6 py-10 text-center text-[13px] leading-6 text-ink-faint">
      <FileText className="size-6 opacity-50" aria-hidden />
      <p className="max-w-sm">{text}</p>
    </div>
  )
}

export function DebugParts({ parts, imageURL, textLabel }: { parts: DebugPart[]; imageURL: (blob: string) => string; textLabel?: string }) {
  const { t } = useTranslation()
  return (
    <div className="space-y-3">
      {parts.map((p, i) => {
        if (p.kind === 'image' && p.image)
          return (
            <img
              key={i}
              className="max-h-80 max-w-full rounded border border-rule object-contain"
              src={imageURL(p.image.blob)}
              alt={t('debug.image')}
            />
          )
        if (p.tool_result) return <DebugParts key={i} parts={p.tool_result.parts} imageURL={imageURL} textLabel={t('debug.result')} />
        const label =
          p.kind === 'text'
            ? (textLabel ?? t('debug.textContent'))
            : p.kind === 'reasoning'
              ? t('debug.reasoning')
              : p.kind === 'tool_call'
                ? t('debug.tool')
                : p.kind === 'native'
                  ? t('debug.native')
                  : p.kind
        return <CodeBlock key={i} label={label} value={p.kind === 'text' ? p.text : p} />
      })}
    </div>
  )
}
