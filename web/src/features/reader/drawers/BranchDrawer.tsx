/**
 * 时间线与分支抽屉（《04-交互与界面设计》8.11）：只展示分叉点与各分支的概况（起止回合、回合数、整理次数），
 * 不展开整棵树。点一条分支切换到它的最新进度；"⋯"菜单中可以删除分支（不含当前进度）、复制为新存档。
 *
 *   第 1–2 回合  开场
 *     ├─ 第 3–58 回合  去找林舟        ← 当前（你在这里）
 *     └─ 第 3–4 回合   去灯塔
 */
import { useQueryClient } from '@tanstack/react-query'
import { Ellipsis } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'
import { qk, useBranchTree } from '@/api/queries'
import type { BranchSegment } from '@/api/types'
import { confirmAction } from '@/components/common/confirm'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

import { useReaderContext } from '../ReaderContext'

export function BranchDrawer({ saveId, open, onOpenChange }: { saveId: string; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation()
  const tree = useBranchTree(saveId, open)
  const root = tree.data?.root

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full gap-0 bg-paper px-0 data-[side=right]:sm:max-w-md">
        <SheetHeader className="border-b border-rule px-6 pt-5 pb-4">
          <SheetTitle className="title-book text-base">{t('reader.branchDrawer.title')}</SheetTitle>
          <SheetDescription className="text-[13px]">{t('reader.branchDrawer.intro')}</SheetDescription>
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
          {root ? (
            <SegmentRow seg={root} headId={tree.data!.head_id} isRoot onDone={() => onOpenChange(false)} saveId={saveId} />
          ) : (
            tree.data && <p className="px-2 py-6 text-[13px] text-ink-faint">{t('reader.branchDrawer.empty')}</p>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function SegmentRow({
  seg,
  headId,
  isRoot,
  saveId,
  onDone,
}: {
  seg: BranchSegment
  headId: string
  isRoot?: boolean
  saveId: string
  onDone: () => void
}) {
  const { t } = useTranslation()
  const { actions } = useReaderContext()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const atLeaf = seg.leaf_id === headId

  const range =
    seg.turns === 0
      ? t('reader.branchDrawer.root')
      : seg.from_turn === seg.to_turn
        ? t('reader.branchDrawer.single', { from: seg.from_turn })
        : t('reader.branchDrawer.range', { from: seg.from_turn, to: seg.to_turn })

  const switchTo = async () => {
    if (atLeaf) return
    if (await actions.switchTo(seg.leaf_id)) {
      toast(t('reader.branchDrawer.switched'))
      onDone()
    }
  }
  const remove = async () => {
    const count = countTurns(seg)
    if (!(await confirmAction(t('reader.branchDrawer.deleteConfirm', { count })))) return
    try {
      await api.saves.deleteBranch(saveId, seg.start_id)
      toast(t('reader.branchDrawer.deleted'))
      void qc.invalidateQueries({ queryKey: qk.tree(saveId) })
      void qc.invalidateQueries({ queryKey: qk.save(saveId) })
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }
  const copy = async () => {
    try {
      const sv = await api.saves.copy(saveId, seg.leaf_id)
      void qc.invalidateQueries({ queryKey: qk.workSaves(sv.work_id) })
      void qc.invalidateQueries({ queryKey: qk.recentSaves })
      toast(t('reader.branchDrawer.copied'), {
        action: { label: t('reader.branchDrawer.open'), onClick: () => void navigate(`/play/${sv.id}`) },
      })
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }

  return (
    <div className={cn(!isRoot && 'mt-1 border-l border-rule pl-3')}>
      <div
        className={cn(
          'group flex items-start gap-2 rounded-lg px-2 py-2 transition-colors',
          !atLeaf && 'cursor-pointer hover:bg-hover',
          seg.current && 'bg-selected/60',
        )}
        onClick={() => void switchTo()}
        role={atLeaf ? undefined : 'button'}
        tabIndex={atLeaf ? undefined : 0}
        onKeyDown={(e) => e.key === 'Enter' && void switchTo()}
      >
        <span aria-hidden className={cn('mt-[7px] size-1.5 shrink-0 rounded-full', seg.current ? 'bg-gold' : 'bg-rule-strong')} />
        <div className="min-w-0 flex-1">
          <p className="flex flex-wrap items-baseline gap-x-2 text-[13px] text-ink">
            <span className="tabular">{range}</span>
            {seg.summary && <span className="min-w-0 truncate text-ink-muted">{seg.summary}</span>}
          </p>
          <p className="mt-0.5 flex flex-wrap gap-x-3 text-xs text-ink-faint">
            {seg.turns > 0 && <span>{t('reader.branchDrawer.turns', { count: seg.turns })}</span>}
            {!!seg.compactions && <span>◇ {t('reader.branchDrawer.compactions', { count: seg.compactions })}</span>}
            {!!seg.edits && <span>{t('reader.branchDrawer.edits', { count: seg.edits })}</span>}
            {seg.head_inside && <span className="text-gold-text">{t('reader.branchDrawer.here')}</span>}
          </p>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={t('reader.turnActions')}
              onClick={(e) => e.stopPropagation()}
              className="opacity-60 group-hover:opacity-100"
            >
              <Ellipsis />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
            <DropdownMenuItem disabled={atLeaf} onSelect={() => void switchTo()}>
              {t('reader.branchDrawer.switch')}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => void copy()}>{t('reader.branchDrawer.copy')}</DropdownMenuItem>
            {!isRoot && !seg.current && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem variant="destructive" onSelect={() => void remove()}>
                  {t('reader.branchDrawer.delete')}
                </DropdownMenuItem>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {seg.children && seg.children.length > 0 && (
        <div className="ml-2">
          {seg.children.map((c) => (
            <SegmentRow key={c.start_id} seg={c} headId={headId} saveId={saveId} onDone={onDone} />
          ))}
        </div>
      )}
    </div>
  )
}

function countTurns(seg: BranchSegment): number {
  return seg.turns + (seg.children ?? []).reduce((n, c) => n + countTurns(c), 0)
}
