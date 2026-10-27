import { Activity, ArrowDownToLine, Bug, Pause, Play, Search, Trash2, X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useDebugCall } from '@/api/queries'
import { api } from '@/api/endpoints'
import type { DebugStatus, DebugSummary } from '@/api/types'
import { LoadError } from '@/components/common/LoadError'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

import { EmptyDebug } from './DebugContent'
import { DebugVFS } from './DebugVFS'
import { ContextView, Overview, RawView, StateView, ToolsView } from './DebugViews'

export function DebugInspector({
  saveId,
  open,
  onOpenChange,
  status,
  pending,
  onToggle,
  onClear,
  turnId,
  onAllTurns,
}: {
  saveId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  status?: DebugStatus
  pending: boolean
  onToggle: (on: boolean) => void
  onClear: () => void
  turnId?: string
  onAllTurns: () => void
}) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState('')
  const [follow, setFollow] = useState(true)
  const [search, setSearch] = useState('')
  const calls = (status?.calls ?? []).filter(
    (c) =>
      (!turnId || c.turn_id === turnId) &&
      `${c.model} ${c.turn_no} ${c.purpose} ${t(`debug.purpose.${purpose(c)}`)}`.toLowerCase().includes(search.toLowerCase()),
  )
  const current = (follow ? calls[calls.length - 1] : calls.find((c) => c.id === selected)) ?? calls[calls.length - 1]
  const detail = useDebugCall(saveId, current?.id ?? '', current?.revision ?? 0, open)
  const call = detail.data
  return (
    <Sheet modal={false} open={open} onOpenChange={onOpenChange}>
      <SheetContent
        showCloseButton={false}
        side="right"
        className="w-full gap-0 bg-paper p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-[min(960px,85vw)] motion-reduce:animate-none motion-reduce:transition-none"
        onInteractOutside={(e) => e.preventDefault()}
      >
        <SheetHeader className="gap-3 border-b border-rule px-5 pt-5 pb-4">
          <div className="flex items-center gap-3">
            <Bug className="size-5 text-ink-muted" />
            <SheetTitle className="title-book text-lg">{t('debug.title')}</SheetTitle>
            <span className="ml-auto flex items-center gap-2 text-xs text-ink-muted">
              <span className={cn('size-1.5 rounded-full', status?.enabled ? 'bg-ok' : 'bg-ink-faint')} />
              {t(status?.enabled ? 'debug.recording' : 'debug.paused')}
            </span>
            <Button size="icon-sm" variant="ghost" onClick={() => onOpenChange(false)} aria-label={t('debug.hide')}>
              <X />
            </Button>
          </div>
          <SheetDescription className="text-xs leading-5">{t('debug.description')}</SheetDescription>
          <div className="flex flex-wrap items-center gap-3">
            <Button
              size="xs"
              variant="ghost"
              disabled={pending}
              className={cn(status?.enabled && 'text-gold-text hover:text-gold-text')}
              onClick={() => onToggle(!status?.enabled)}
            >
              {status?.enabled ? <Pause /> : <Play />}
              {t(status?.enabled ? 'debug.pauseCapture' : 'debug.startCapture')}
            </Button>
            <Button
              size="xs"
              variant="ghost"
              aria-pressed={follow}
              className={cn(follow && 'text-gold-text')}
              onClick={() => {
                setFollow(!follow)
                if (current) setSelected(current.id)
              }}
            >
              <ArrowDownToLine />
              {t('debug.follow')}
            </Button>
            <Button
              size="xs"
              variant="ghost"
              className="ml-auto text-ink-muted"
              disabled={pending || !status?.calls.length}
              onClick={onClear}
            >
              <Trash2 />
              {t('debug.clear')}
            </Button>
          </div>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col sm:flex-row">
          <aside className="flex max-h-48 shrink-0 flex-col border-b border-rule sm:max-h-none sm:w-52 sm:border-r sm:border-b-0">
            <div className="relative m-3">
              <Search className="absolute top-2.5 left-2.5 size-3.5 text-ink-faint" />
              <Input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="h-8 pl-8 text-xs"
                aria-label={t('debug.searchCalls')}
                placeholder={t('debug.searchCalls')}
              />
            </div>
            {turnId && (
              <Button variant="ghost" size="xs" className="mx-3 mb-2" onClick={onAllTurns}>
                {t('debug.allTurns')}
              </Button>
            )}
            <div className="scroll-stable min-h-0 flex-1 space-y-1 px-2 pb-3" role="navigation" aria-label={t('debug.calls')}>
              {[...calls].reverse().map((c, i) => (
                <button
                  type="button"
                  key={c.id}
                  onClick={() => {
                    setSelected(c.id)
                    setFollow(false)
                  }}
                  aria-current={current?.id === c.id ? 'true' : undefined}
                  className={cn(
                    'w-full rounded-lg border border-transparent px-3 py-3 text-left transition-colors hover:bg-hover',
                    current?.id === c.id && 'border-gold/20 bg-gold-soft',
                  )}
                >
                  <div className="flex items-center gap-2 text-xs">
                    <span
                      className={cn(
                        'size-1.5 rounded-full',
                        c.status === 'error' ? 'bg-danger' : c.status === 'running' ? 'animate-pulse bg-ok' : 'bg-ink-faint/50',
                      )}
                    />
                    <span>{t('debug.turn', { n: c.turn_no })}</span>
                    <span className="ml-auto font-mono text-ink-faint">{String(calls.length - i).padStart(2, '0')}</span>
                  </div>
                  <p className="mt-2 truncate text-[13px]">{c.model}</p>
                  <p className="mt-1 flex items-center justify-between gap-2 text-xs text-ink-faint">
                    <span>{t(`debug.purpose.${purpose(c)}`)}</span>
                    <span className="font-mono">{new Date(c.started).toLocaleTimeString('zh-CN', { hour12: false })}</span>
                  </p>
                </button>
              ))}
              {!calls.length && <p className="px-3 py-4 text-xs leading-6 text-ink-faint">{t('debug.noMatches')}</p>}
            </div>
            {!!status?.dropped && (
              <p className="border-t border-rule px-4 py-3 text-xs leading-5 text-ink-faint">
                {t('debug.dropped', { count: status.dropped })}
              </p>
            )}
          </aside>
          <main className="flex min-h-0 min-w-0 flex-1 flex-col">
            {!current ? (
              <div className="flex flex-1 items-center justify-center">
                <EmptyDebug text={t(turnId ? 'debug.noTurn' : status?.enabled ? 'debug.emptyEnabled' : 'debug.emptyDisabled')} />
              </div>
            ) : (
              <>
                <div className="flex items-center gap-2 border-b border-rule px-5 py-3 text-xs text-ink-muted">
                  <Activity className="size-3.5" />
                  <span className="min-w-0 truncate">{current.model}</span>
                  <span className="ml-auto font-mono text-ink-faint">{current.protocol}</span>
                  <span>{t(`debug.status.${current.status}`)}</span>
                </div>
                {detail.isError && (
                  <div className="p-5">
                    <LoadError error={detail.error} onRetry={() => void detail.refetch()} />
                  </div>
                )}
                {!call && detail.isPending && (
                  <p className="p-5 text-xs text-ink-faint" role="status">
                    {t('common.loading')}
                  </p>
                )}
                {call && (
                  <Tabs defaultValue="overview" className="min-h-0 flex-1 gap-0">
                    <div className="shrink-0 overflow-x-auto px-5">
                      <TabsList variant="line" className="w-full min-w-max gap-5">
                        {(['overview', 'context', 'tools', 'state', 'vfs', 'raw'] as const).map((tab) => (
                          <TabsTrigger key={tab} value={tab}>
                            {t(`debug.tabs.${tab}`)}
                          </TabsTrigger>
                        ))}
                      </TabsList>
                    </div>
                    {call.truncated && (
                      <p role="status" className="border-b border-rule bg-warn/5 px-5 py-2 text-xs leading-5 text-warn">
                        {t('debug.truncated')}
                      </p>
                    )}
                    {call.error && (
                      <p
                        role="alert"
                        className="max-h-24 shrink-0 overflow-auto border-b border-rule bg-danger/5 px-5 py-2 text-xs leading-5 break-words text-danger"
                      >
                        {call.error}
                      </p>
                    )}
                    <TabsContent value="overview" className="scroll-stable min-h-0 flex-1 p-5">
                      <Overview call={call} />
                    </TabsContent>
                    <TabsContent value="context" className="scroll-stable min-h-0 flex-1 p-5">
                      <ContextView call={call} imageURL={(blob) => api.debug.imageURL(saveId, call.id, blob)} />
                    </TabsContent>
                    <TabsContent value="tools" className="scroll-stable min-h-0 flex-1 p-5">
                      <ToolsView call={call} imageURL={(blob) => api.debug.imageURL(saveId, call.id, blob)} />
                    </TabsContent>
                    <TabsContent value="state" className="scroll-stable min-h-0 flex-1 p-5">
                      <StateView call={call} />
                    </TabsContent>
                    <TabsContent value="vfs" className="scroll-stable min-h-0 flex-1 p-5">
                      <DebugVFS key={call.id} saveId={saveId} call={call} />
                    </TabsContent>
                    <TabsContent value="raw" className="scroll-stable min-h-0 flex-1 p-5">
                      <RawView call={call} />
                    </TabsContent>
                  </Tabs>
                )}
              </>
            )}
          </main>
        </div>
      </SheetContent>
    </Sheet>
  )
}

function purpose(call: DebugSummary): 'narrate' | 'continue' | 'compact' | 'rewrite' | 'rewrite_memory' | 'choices' {
  return call.purpose === 'compact' ||
    call.purpose === 'continue' ||
    call.purpose === 'rewrite' ||
    call.purpose === 'rewrite_memory' ||
    call.purpose === 'choices'
    ? call.purpose
    : 'narrate'
}
