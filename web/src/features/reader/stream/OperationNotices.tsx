import { CircleAlert, CirclePause, StepForward, RotateCcw, ScanSearch, TriangleAlert, X } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { selectBusy, useReader, type TurnFailure } from '@/stores/reader'
import { useReaderContext } from '../ReaderContext'

/** 通知独立于正文滚动区。关闭只影响本标签页，下一次中断仍会显示。 */
function ResultNotice({
  id,
  storageKey,
  children,
  actions,
  tone,
  title,
}: {
  id: string
  storageKey: string
  children: ReactNode
  actions: ReactNode
  tone: 'quiet' | 'warning' | 'error'
  title: string
}) {
  const { t } = useTranslation()
  const [dismissed, setDismissed] = useState(() => sessionStorage.getItem(storageKey))
  if (dismissed === id) return null
  const Icon = tone === 'quiet' ? CirclePause : tone === 'warning' ? TriangleAlert : CircleAlert
  return (
    <div
      role={tone === 'quiet' ? 'status' : 'alert'}
      aria-label={title}
      className="mb-2 grid grid-cols-[1.125rem_minmax(0,1fr)_auto] items-start gap-x-2.5 rounded-xl border border-rule bg-paper-raised px-3 py-2 font-sans @min-[36rem]:grid-cols-[1.125rem_minmax(0,1fr)_auto_auto] @min-[36rem]:items-center"
    >
      <Icon
        aria-hidden
        className={cn(
          'mt-0.5 size-[18px] @min-[36rem]:mt-0',
          tone === 'quiet' ? 'text-ink-muted' : tone === 'warning' ? 'text-warn' : 'text-danger',
        )}
      />
      <div className="text-[13px] leading-5 [overflow-wrap:anywhere]">
        <p className="font-semibold text-ink">{title}</p>
        <p className="text-ink-muted">{children}</p>
      </div>
      <div className="col-span-2 col-start-2 row-start-2 mt-1 flex flex-wrap items-center gap-1 empty:hidden @min-[36rem]:col-span-1 @min-[36rem]:col-start-3 @min-[36rem]:row-start-1 @min-[36rem]:mt-0">
        {actions}
      </div>
      <Button
        className="col-start-3 row-start-1 -mr-1 justify-self-end @min-[36rem]:col-start-4"
        size="icon-sm"
        variant="ghost"
        aria-label={t('common.close')}
        title={t('common.close')}
        onClick={() => {
          sessionStorage.setItem(storageKey, id)
          setDismissed(id)
        }}
      >
        <X aria-hidden className="size-4" />
      </Button>
    </div>
  )
}

function turnTitle({ code, operation, nodeId }: TurnFailure) {
  if (code === 'canceled') return 'reader.turnStopped'
  if (operation) return operation.kind === 'choices' ? 'reader.choicesInterrupted' : 'reader.rewriteInterrupted'
  return nodeId ? 'reader.turnPartial' : 'reader.turnNotSaved'
}

function InspectButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation()
  return (
    <Button size="sm" variant="ghost" className="px-2" onClick={onClick}>
      <ScanSearch aria-hidden /> {t('reader.inspector')}
    </Button>
  )
}

export function OperationNotices({ headId, onRetry, onContinue }: { headId?: string; onRetry: () => void; onContinue: () => void }) {
  const { t } = useTranslation()
  const { saveId, actions, inspect } = useReaderContext()
  const failure = useReader((s) => s.failure)
  const compact = useReader((s) => s.compactFailure)
  const busy = useReader(selectBusy)
  const compacting = useReader((s) => s.compacting)
  const connected = useReader((s) => s.connection === 'open')
  const disabled = busy || compacting || !connected
  const stopped = failure?.code === 'canceled'
  return (
    <div className="reader-column @container max-h-[35dvh] shrink-0 overflow-y-auto">
      {failure && (
        <ResultNotice
          key={`turn:${saveId}`}
          id={failure.turnId}
          storageKey={`aint.notice.turn.${saveId}`}
          tone={stopped ? 'quiet' : failure.nodeId ? 'warning' : 'error'}
          title={t(turnTitle(failure))}
          actions={
            <>
              {failure.nodeId && (failure.code === 'length' || failure.code === 'budget') && (
                <Button size="sm" variant="secondary" disabled={disabled} onClick={onContinue}>
                  <StepForward aria-hidden />
                  {t('reader.continue')}
                </Button>
              )}
              {failure.recoverable && (
                <Button size="sm" variant="ghost" disabled={disabled} onClick={onRetry}>
                  <RotateCcw aria-hidden />
                  {t(failure.nodeId ? 'reader.regenerate' : 'common.retry')}
                </Button>
              )}
              {inspect && <InspectButton onClick={() => inspect(failure.turnId)} />}
            </>
          }
        >
          {stopped && !failure.operation ? t(failure.nodeId ? 'reader.partialKept' : 'reader.turnNotSaved') : failure.message}
        </ResultNotice>
      )}
      {compact && (
        <ResultNotice
          key={`compact:${saveId}`}
          id={compact.attempt_id ?? compact.message}
          storageKey={`aint.notice.compact.${saveId}`}
          tone="warning"
          title={t(compact.reason === 'threshold' ? 'reader.autoCompactFailed' : 'reader.compactFailed')}
          actions={
            <>
              {compact.retry_head && compact.retry_head === headId && (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={disabled}
                  onClick={() => void actions.compact(compact.retry_head, compact.redo)}
                >
                  <RotateCcw aria-hidden /> {t('reader.compactRetry')}
                </Button>
              )}
              {inspect && <InspectButton onClick={() => inspect(compact.attempt_id)} />}
            </>
          }
        >
          {compact.message} {t('reader.compactUnchanged')}
        </ResultNotice>
      )}
    </div>
  )
}
