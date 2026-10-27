import { useState, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'

import type { TimelineNode } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'

interface Props {
  nodes: TimelineNode[]
  open: boolean
  desktop: boolean
  onOpenChange: (open: boolean) => void
  triggerRef: RefObject<HTMLButtonElement | null>
  onJump: (id: string) => void
  hasMore?: boolean
  loadingEarlier: boolean
  loadEarlier: () => void
}

/** 桌面目录参与阅读布局；窄屏目录使用带焦点管理的侧滑抽屉。 */
export function TurnIndex({ nodes, open, desktop, onOpenChange, triggerRef, onJump, hasMore, loadingEarlier, loadEarlier }: Props) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const entries = nodes
    .filter((n) => n.kind === 'turn')
    .map((n) => ({
      id: n.id,
      no: n.turn_no,
      text: n.display
        .filter((d) => d.t === 'text')
        .map((d) => d.markup)
        .join(' ')
        .replace(/<[^>]*>/g, '')
        .slice(0, 120),
    }))
    .filter((n) => `${n.no} ${n.text}`.includes(search))
  const content = (
    <nav id="reader-turn-index" aria-label={t('reader.turnIndex')} className="scroll-stable min-h-0 flex-1 px-3 pb-5">
      <Input
        aria-label={t('reader.findTurn')}
        placeholder={t('reader.findTurn')}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="mb-3 h-8 text-xs"
      />
      {hasMore && (
        <Button variant="ghost" size="sm" disabled={loadingEarlier} onClick={loadEarlier}>
          {t('reader.loadEarlier')}
        </Button>
      )}
      <ol className="space-y-1">
        {entries.map((n) => (
          <li key={n.id}>
            <button
              className="w-full rounded-lg p-2 text-left text-xs leading-5 text-ink-muted transition-colors hover:bg-hover focus-visible:bg-hover"
              onClick={() => {
                onJump(n.id)
                if (!desktop) onOpenChange(false)
              }}
            >
              <span className="block text-gold-text">{t('reader.turnNumber', { no: n.no })}</span>
              <span className="line-clamp-2">{n.text}</span>
            </button>
          </li>
        ))}
      </ol>
      {entries.length === 0 && <p className="px-2 py-6 text-center text-xs text-ink-faint">{t('reader.noMatchingTurns')}</p>}
    </nav>
  )

  if (desktop) {
    return (
      <div className="reader-side reader-index" data-open={open} inert={!open} aria-hidden={!open}>
        <div className="flex h-full w-52 flex-col border-r border-rule bg-paper/15">
          <h2 className="px-5 pt-5 pb-3 font-serif text-sm text-ink-muted">{t('reader.turnIndex')}</h2>
          {content}
        </div>
      </div>
    )
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="left"
        className="work-root gap-0 bg-paper/95 backdrop-blur-xl duration-300 data-[side=left]:w-[min(288px,85vw)]"
        aria-describedby={undefined}
        onCloseAutoFocus={(e) => {
          e.preventDefault()
          triggerRef.current?.focus()
        }}
      >
        <SheetHeader className="px-5 pt-5 pb-4">
          <SheetTitle className="font-serif text-sm text-ink-muted">{t('reader.turnIndex')}</SheetTitle>
        </SheetHeader>
        {content}
      </SheetContent>
    </Sheet>
  )
}
