import { Brain, Check, ChevronDown, Circle, Clock3, Wrench } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { DebugCall } from '@/api/types'
import { cn } from '@/lib/utils'

import { CodeBlock, DebugParts, EmptyDebug } from './DebugContent'
import { pretty } from './format'

export function Overview({ call }: { call: DebugCall }) {
  const { t } = useTranslation()
  const input = call.usage.input_uncached + call.usage.cache_read + call.usage.cache_write
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-3 divide-x divide-rule rounded-lg border border-rule bg-paper-raised py-4 text-center">
        {[
          [t('debug.elapsed'), `${(call.duration_ms / 1000).toFixed(2)} s`],
          [t('debug.inputTokens'), input.toLocaleString()],
          [t('debug.outputTokens'), call.usage.output.toLocaleString()],
        ].map(([label, value]) => (
          <div key={label}>
            <p className="text-xs text-ink-faint">{label}</p>
            <p className="mt-2 font-mono text-lg tabular-nums">{value}</p>
          </div>
        ))}
      </div>
      <p className="text-xs leading-6 text-ink-muted">{t('debug.reasoningHint')}</p>
      {!call.steps.length && <EmptyDebug text={t(call.status === 'running' ? 'debug.waiting' : 'debug.noSteps')} />}
      <ol className="space-y-5">
        {call.steps.map((step, i) => {
          const result = step.call && call.tools.find((tool) => tool.id === step.call?.id)
          const Icon = step.kind === 'reasoning' ? Brain : step.kind === 'tool' ? Wrench : Check
          return (
            <li key={i} className="relative pl-9">
              <span className="absolute top-0 left-0 flex size-6 items-center justify-center rounded-full border border-rule bg-paper-raised text-ink-muted">
                <Icon className="size-3.5" />
              </span>
              <div className="flex items-center gap-3 text-xs text-ink-faint">
                <span className="text-ink">{step.call?.name ?? t(`debug.${step.kind}`)}</span>
                <span className="font-mono">+{(step.at_ms / 1000).toFixed(2)} s</span>
              </div>
              {step.kind === 'tool' ? (
                <details className="mt-2 rounded-lg border border-rule bg-paper-raised p-3 text-xs">
                  <summary className="cursor-pointer text-ink-muted">
                    {result
                      ? t(result.is_error ? 'debug.toolError' : 'debug.toolDone')
                      : t(call.status === 'error' ? 'debug.notExecuted' : 'debug.toolPending')}
                  </summary>
                  <div className="mt-3 space-y-3">
                    <CodeBlock label={t('debug.arguments')} value={step.call?.args} />
                    {result && <CodeBlock label={t('debug.result')} value={result.result} />}
                  </div>
                </details>
              ) : (
                <div className={cn('mt-2', step.kind === 'reasoning' && 'border-l-2 border-gold/40 pl-3')}>
                  <CodeBlock label={t(`debug.${step.kind}`)} value={step.text} />
                </div>
              )}
            </li>
          )
        })}
      </ol>
      {call.status === 'running' && (
        <p role="status" className="flex items-center gap-2 text-xs text-ink-muted">
          <Clock3 className="size-3.5 animate-pulse" />
          {t('debug.streaming')}
        </p>
      )}
      {call.stop && (
        <p className="border-t border-rule pt-3 text-xs text-ink-faint">
          {t('debug.stop')} <span className="font-mono text-ink-muted">{call.stop}</span>
        </p>
      )}
    </div>
  )
}

export function ContextView({ call, imageURL }: { call: DebugCall; imageURL: (blob: string) => string }) {
  const { t } = useTranslation()
  const req = call.request
  if (!req) return <EmptyDebug text={t('debug.noContext')} />
  return (
    <div className="space-y-4">
      <p className="text-xs leading-6 text-ink-muted">{t('debug.contextHint')}</p>
      {(req.system ?? []).map((s, i) => (
        <CodeBlock key={i} label={t('debug.system', { n: i + 1 })} value={s.text} />
      ))}
      <details className="rounded-lg border border-rule p-3">
        <summary className="cursor-pointer text-[13px]">{t('debug.toolDefinitions', { count: req.tools?.length ?? 0 })}</summary>
        <div className="mt-3">
          <CodeBlock label={t('debug.definitions')} value={req.tools} />
        </div>
      </details>
      {(req.messages ?? []).map((msg, i) => (
        <details key={i} className="rounded-lg border border-rule p-3" open={i === req.messages.length - 1}>
          <summary className="cursor-pointer text-[13px]">
            <span className="mr-3 font-mono text-ink-faint">{String(i + 1).padStart(2, '0')}</span>
            {t(msg.role === 'assistant' ? 'debug.assistant' : 'debug.user')}
            <span className="ml-3 text-xs text-ink-faint">{t('debug.parts', { count: msg.parts?.length ?? 0 })}</span>
          </summary>
          <div className="mt-3">
            <DebugParts parts={msg.parts ?? []} imageURL={imageURL} />
          </div>
        </details>
      ))}
      <CodeBlock
        label={t('debug.parameters')}
        value={{ max_output: req.max_output, reasoning: req.reasoning, cache_key: req.cache_key, cache_ttl: req.cache_ttl }}
      />
    </div>
  )
}

export function ToolsView({ call, imageURL }: { call: DebugCall; imageURL: (blob: string) => string }) {
  const { t } = useTranslation()
  const calls = call.steps.flatMap((s) => (s.call ? [s.call] : []))
  if (!calls.length) return <EmptyDebug text={t('debug.noTools')} />
  return (
    <div className="space-y-4">
      {calls.map((tool) => {
        const result = call.tools.find((r) => r.id === tool.id)
        const parts = call.results.find((r) => r.call_id === tool.id)?.parts
        return (
          <section key={tool.id} className="rounded-xl border border-rule bg-paper-raised p-4">
            <div className="mb-4 flex items-center gap-2">
              <Wrench className="size-4 text-ink-faint" />
              <h3 className="font-mono text-sm">{tool.name}</h3>
              <span className={cn('ml-auto text-xs', result?.is_error ? 'text-danger' : 'text-ink-muted')}>
                {result
                  ? `${t(result.is_error ? 'debug.toolError' : 'debug.toolDone')} · ${result.duration_ms} ms`
                  : t(call.status === 'error' ? 'debug.notExecuted' : 'debug.toolPending')}
              </span>
            </div>
            <div className="space-y-3">
              <CodeBlock label={t('debug.arguments')} value={tool.args} />
              {parts ? (
                <DebugParts parts={parts} imageURL={imageURL} textLabel={t('debug.result')} />
              ) : (
                result && <CodeBlock label={t('debug.result')} value={result.result || t('debug.terminal')} />
              )}
            </div>
            <p className="mt-3 break-all font-mono text-xs text-ink-faint">{tool.id}</p>
          </section>
        )
      })}
    </div>
  )
}

export function StateView({ call }: { call: DebugCall }) {
  const { t } = useTranslation()
  if (!call.before || !call.after) return <EmptyDebug text={t('debug.noState')} />
  const before = call.before?.state ?? {}
  const after = call.after?.state ?? {}
  const keys = [...new Set([...Object.keys(before), ...Object.keys(after)])].filter((key) => pretty(before[key]) !== pretty(after[key]))
  return (
    <div className="space-y-4">
      <p className="flex items-center gap-2 text-[13px] text-ink-muted">
        <Circle className="size-3" />
        {t(keys.length ? 'debug.stateChanged' : 'debug.stateUnchanged', { count: keys.length })}
      </p>
      {keys.map((key) => (
        <div key={key} className="space-y-2">
          <h3 className="font-mono text-sm">{key}</h3>
          <div className="grid gap-3 lg:grid-cols-2">
            <CodeBlock label={t('debug.before')} value={before[key]} />
            <CodeBlock label={t('debug.after')} value={after[key]} />
          </div>
        </div>
      ))}
      <CodeBlock label={t('debug.fullState')} value={after} />
    </div>
  )
}

export function RawView({ call }: { call: DebugCall }) {
  const { t } = useTranslation()
  if (!call.http.length) return <EmptyDebug text={t('debug.noHTTP')} />
  return (
    <div className="space-y-5">
      <p className="text-xs leading-6 text-ink-muted">{t('debug.rawHint')}</p>
      {call.http.map((http, i) => (
        <details key={i} open className="space-y-3 rounded-xl border border-rule p-4">
          <summary className="flex cursor-pointer items-center gap-2 text-[13px]">
            <ChevronDown className="size-3.5" />
            {t('debug.attempt', { n: i + 1 })}
            <span className={cn('ml-auto font-mono', http.status >= 400 ? 'text-danger' : 'text-ink-muted')}>
              {http.status || '…'} · {http.duration_ms || http.header_ms} ms
            </span>
          </summary>
          <p className="break-all font-mono text-xs leading-6 text-ink-muted">
            {http.method} {http.url}
          </p>
          {http.error && <p className="text-xs text-danger">{http.error}</p>}
          {http.truncated && <p className="text-xs text-gold-text">{t('debug.responseTruncated')}</p>}
          <CodeBlock label={t('debug.requestHeaders')} value={http.request_headers} />
          <CodeBlock label={t('debug.requestBody')} value={http.request_body} />
          <CodeBlock label={t('debug.responseHeaders')} value={http.response_headers} />
          <CodeBlock label={t('debug.responseBody')} value={http.response_body} />
        </details>
      ))}
    </div>
  )
}
