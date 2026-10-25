import { ChevronLeft, ChevronRight, File, Folder, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useDebugFile } from '@/api/queries'
import type { DebugCall, DebugSnapshot } from '@/api/types'
import { LoadError } from '@/components/common/LoadError'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import { CodeBlock, EmptyDebug } from './DebugContent'

export function DebugVFS({ saveId, call }: { saveId: string; call: DebugCall }) {
  const { t } = useTranslation()
  const [phase, setPhase] = useState<'before' | 'after'>('after')
  const [search, setSearch] = useState('')
  const [path, setPath] = useState('')
  const snap = call[phase]
  const files = useMemo(
    () => snap?.files.filter((f) => `${f.path} ${f.summary}`.toLowerCase().includes(search.toLowerCase())) ?? [],
    [snap, search],
  )
  const selected = files.find((f) => f.path === path) ?? files[0]
  const changes = useMemo(() => {
    if (!call.before || !call.after) return new Map<string, 'added' | 'removed' | 'modified'>()
    const a = new Map(call.before?.files.map((f) => [f.path, f.blob]))
    const b = new Map(call.after?.files.map((f) => [f.path, f.blob]))
    return new Map(
      [...new Set([...a.keys(), ...b.keys()])].flatMap((p) =>
        a.get(p) === b.get(p) ? [] : [[p, !a.has(p) ? 'added' : !b.has(p) ? 'removed' : 'modified'] as const],
      ),
    )
  }, [call.before, call.after])
  return (
    <div className="space-y-4">
      <p className="text-xs leading-6 text-ink-muted">{t('debug.vfsHint')}</p>
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex rounded-lg bg-paper-sunken p-0.5">
          {(['before', 'after'] as const).map((p) => (
            <Button
              key={p}
              size="xs"
              variant="ghost"
              aria-pressed={phase === p}
              className={cn(phase === p && 'bg-segment text-gold-text shadow-sm')}
              onClick={() => setPhase(p)}
            >
              {t(`debug.${p}`)}
            </Button>
          ))}
        </div>
        <span className="ml-auto text-xs text-ink-faint">{t('debug.fileCount', { count: snap?.files.length ?? 0 })}</span>
      </div>
      {!snap && <EmptyDebug text={t('debug.noVFS')} />}
      {snap && (
        <>
          {snap?.error && (
            <p role="alert" className="text-xs text-danger">
              {snap.error}
            </p>
          )}
          {!!changes.size && (
            <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-muted">
              {[...changes].map(([p, kind]) => (
                <span key={p} title={p}>
                  {t(`debug.${kind}`)} · {p}
                </span>
              ))}
            </div>
          )}
          {[...changes.keys()]
            .filter((p) => p.startsWith('/memory/'))
            .map((p) => (
              <MemoryDiff key={p} saveId={saveId} call={call} path={p} />
            ))}
          <div className="relative">
            <Search className="absolute top-2.5 left-3 size-4 text-ink-faint" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t('debug.searchFiles')}
              aria-label={t('debug.searchFiles')}
              className="pl-9"
            />
          </div>
          <div className="grid min-w-0 gap-4 xl:grid-cols-[210px_minmax(0,1fr)]">
            <nav className="max-h-52 overflow-auto rounded-lg border border-rule p-2 xl:max-h-[60vh]" aria-label={t('debug.files')}>
              {snap?.roots.map((root) => (
                <div key={root} className="mb-2">
                  <p className="flex items-center gap-2 px-2 py-2 font-mono text-xs text-ink-muted">
                    <Folder className="size-3.5" />
                    {root}
                  </p>
                  {files
                    .filter((f) => f.path.startsWith(`${root}/`))
                    .map((f) => (
                      <button
                        type="button"
                        key={f.path}
                        title={`${f.path}\n${f.summary}`}
                        aria-current={selected?.path === f.path ? 'true' : undefined}
                        onClick={() => setPath(f.path)}
                        className={cn(
                          'flex w-full items-center gap-2 rounded px-2 py-2 text-left font-mono text-xs transition-colors hover:bg-hover',
                          selected?.path === f.path && 'bg-gold-soft text-gold-text',
                        )}
                      >
                        <File className="size-3 shrink-0" />
                        <span className="min-w-0 break-all">{f.path.slice(root.length + 1)}</span>
                        {changes.has(f.path) && <span className="ml-auto size-1.5 shrink-0 rounded-full bg-gold" />}
                      </button>
                    ))}
                </div>
              ))}
            </nav>
            {selected ? (
              <FilePreview
                key={`${call.id}:${phase}:${selected.path}:${selected.blob}`}
                saveId={saveId}
                callId={call.id}
                phase={phase}
                file={selected}
              />
            ) : (
              <EmptyDebug text={t('debug.noFiles')} />
            )}
          </div>
        </>
      )}
    </div>
  )
}

function FilePreview({
  saveId,
  callId,
  phase,
  file,
}: {
  saveId: string
  callId: string
  phase: string
  file: DebugSnapshot['files'][number]
}) {
  const { t } = useTranslation()
  const [offsets, setOffsets] = useState([1])
  const offset = offsets[offsets.length - 1]
  const query = useDebugFile(saveId, callId, phase, file.path, offset, file.blob)
  const data = query.data
  return (
    <div className="min-w-0 space-y-3">
      <p className="break-all font-mono text-xs text-ink-muted">{file.path}</p>
      {file.summary && <p className="text-xs leading-6 text-ink-faint">{file.summary}</p>}
      {query.isError && <LoadError error={query.error} onRetry={() => void query.refetch()} />}
      {query.isPending && (
        <p role="status" className="text-xs text-ink-faint">
          {t('common.loading')}
        </p>
      )}
      {data?.image ? (
        <img
          className="max-h-[55vh] max-w-full rounded-lg border border-rule object-contain"
          alt={file.summary || file.path}
          src={`/api/v1/${file.path.startsWith('/uploads/') ? 'uploads' : 'assets'}/${data.image.blob.replace(/^sha256:/, '')}`}
        />
      ) : (
        data && (
          <>
            <CodeBlock label={t('debug.fileContent')} value={data.text} />
            <div className="flex items-center justify-between gap-2 text-xs text-ink-faint">
              <Button
                size="icon-xs"
                variant="outline"
                disabled={offsets.length === 1}
                aria-label={t('debug.previousPage')}
                onClick={() => setOffsets(offsets.slice(0, -1))}
              >
                <ChevronLeft />
              </Button>
              <span>{t('debug.characters', { from: data.from_char, to: data.to_char, total: data.total_chars })}</span>
              <Button
                size="icon-xs"
                variant="outline"
                disabled={data.to_char >= data.total_chars}
                aria-label={t('debug.nextPage')}
                onClick={() => setOffsets([...offsets, data.to_char + 1])}
              >
                <ChevronRight />
              </Button>
            </div>
          </>
        )
      )}
    </div>
  )
}

function MemoryDiff({ saveId, call, path }: { saveId: string; call: DebugCall; path: string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <details onToggle={(e) => setOpen(e.currentTarget.open)} className="rounded-lg border border-rule p-3 text-xs">
      <summary className="cursor-pointer">
        {t('debug.memoryDiff')} · {path}
      </summary>
      {open && <MemoryDiffContent saveId={saveId} call={call} path={path} />}
    </details>
  )
}

function MemoryDiffContent({ saveId, call, path }: { saveId: string; call: DebugCall; path: string }) {
  const { t } = useTranslation()
  const a = call.before?.files.find((f) => f.path === path)
  const b = call.after?.files.find((f) => f.path === path)
  const before = useDebugFile(saveId, call.id, 'before', a ? path : '', 1, a?.blob ?? '')
  const after = useDebugFile(saveId, call.id, 'after', b ? path : '', 1, b?.blob ?? '')
  if ((a && before.isPending) || (b && after.isPending)) return <p>{t('common.loading')}</p>
  if (before.isError || after.isError)
    return (
      <LoadError
        error={before.error ?? after.error}
        onRetry={() => {
          void before.refetch()
          void after.refetch()
        }}
      />
    )
  const old = before.data?.text.split('\n') ?? []
  const next = after.data?.text.split('\n') ?? []
  return (
    <div className="mt-3">
      <p className="mb-2 text-ink-faint">{t('debug.diffHint')}</p>
      {Array.from({ length: Math.max(old.length, next.length) }, (_, i) => (
        <div key={i} className="font-mono whitespace-pre-wrap break-all">
          {old[i] === next[i] ? (
            <div>
              {i + 1} {old[i]}
            </div>
          ) : (
            <>
              {old[i] !== undefined && (
                <div className="bg-danger/10 text-danger">
                  − {i + 1} {old[i]}
                </div>
              )}
              {next[i] !== undefined && (
                <div className="bg-gold/10 text-gold-text">
                  + {i + 1} {next[i]}
                </div>
              )}
            </>
          )}
        </div>
      ))}
    </div>
  )
}
