/**
 * 回合操作（《04-交互与界面设计》8.8）：悬停回合时出现在右上角（触屏上常显、较淡）。
 *
 *   ‹ 2/3 ›     有多个版本的回合：切换到该版本所在分支的最新进度
 *   ⋯ 菜单      重新生成（最后一个回合）、修改正文、修改我的输入、回到此处（较早的回合）、复制正文
 *
 * 修改在对话框中进行，说明会产生什么：新版本成为当前进度，原来的版本与后续保留为另一条分支。
 */
import { ChevronLeft, ChevronRight, Copy, CornerUpLeft, Ellipsis, PencilLine, RotateCcw, SquarePen, ScanSearch } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import type { DisplayItem, TimelineNode } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Textarea } from '@/components/ui/textarea'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { parseMarkup, plainText } from '@/markup/parse'

import { useReaderContext } from '../ReaderContext'
import { displayedInput } from './displayedInput'

type Editing = { kind: 'text' | 'input' | 'revise'; value: string } | null

export function TurnToolbar({
  node,
  latest,
  disabled,
  open,
  onOpenChange,
}: {
  node: TimelineNode
  latest: boolean
  disabled: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const { actions, ui, inspect } = useReaderContext()
  const [editing, setEditing] = useState<Editing>(null)
  const [saving, setSaving] = useState(false)

  const input = node.display.find((it): it is Extract<DisplayItem, { t: 'input' }> => it.t === 'input')
  const markup = node.display
    .filter((it): it is Extract<DisplayItem, { t: 'text' }> => it.t === 'text')
    .map((it) => it.markup)
    .join('\n\n')
  const staticOpening = node.stop === 'static_opening'
  const { index, count, ids } = node.versions
  const editableInput = input && input.kind !== 'continue' && input.kind !== 'opening'

  const copy = () => {
    const tags = new Set([...ui.builtin_tags, ...ui.tags.map((tag) => tag.name)])
    void navigator.clipboard.writeText(plainText(parseMarkup(markup, { tags }))).then(() => toast(t('common.copied')))
  }
  const save = async () => {
    if (!editing) return
    const value = editing.value.trim()
    if (!value) return
    setSaving(true)
    const ok =
      editing.kind === 'revise'
        ? await actions.operate('rewrite', value, node.id)
        : editing.kind === 'text'
          ? await actions.rewrite(node.id, value)
          : await actions.regenerate(node.id, node.parent_id, { kind: input?.kind === 'ooc' ? 'ooc' : 'free', text: value })
    setSaving(false)
    if (ok) setEditing(null)
  }

  // 零高度的粘性容器：不占版面，读到长回合的后半段时操作按钮仍停在视野上方
  return (
    <div className="pointer-events-none sticky top-8 z-10 -mb-px flex h-px justify-end">
      <div
        className="pointer-events-auto flex -translate-y-1/2 items-center gap-0.5 self-start rounded-lg bg-paper/80 font-sans opacity-0 backdrop-blur transition-opacity group-hover/turn:opacity-100 focus-within:opacity-100 [@media(hover:none)]:opacity-60"
        role="toolbar"
        aria-label={t('reader.turnActions')}
      >
        {count > 1 && ids && (
          <span className="flex items-center text-xs text-ink-faint tabular">
            <Button
              variant="ghost"
              size="icon-xs"
              disabled={disabled || index <= 1}
              aria-label={t('reader.versionPrev')}
              onClick={() => void actions.switchTo(ids[index - 2])}
            >
              <ChevronLeft />
            </Button>
            <span aria-label={t('reader.versionLabel', { index, count })}>
              {index}/{count}
            </span>
            <Button
              variant="ghost"
              size="icon-xs"
              disabled={disabled || index >= count}
              aria-label={t('reader.versionNext')}
              onClick={() => void actions.switchTo(ids[index])}
            >
              <ChevronRight />
            </Button>
          </span>
        )}
        {node.edited_from && <span className="px-1 text-xs text-ink-faint">{t('reader.edited')}</span>}
        <DropdownMenu open={open} onOpenChange={onOpenChange}>
          <Tooltip>
            <TooltipTrigger asChild>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon-xs" aria-label={t('reader.turnActions')}>
                  <Ellipsis />
                </Button>
              </DropdownMenuTrigger>
            </TooltipTrigger>
            <TooltipContent>{t('reader.turnActions')}</TooltipContent>
          </Tooltip>
          <DropdownMenuContent align="end">
            {inspect && (
              <DropdownMenuItem onSelect={() => inspect(node.id)}>
                <ScanSearch />
                {t('reader.inspector')}
              </DropdownMenuItem>
            )}
            {latest && !staticOpening && (
              <DropdownMenuItem
                disabled={disabled}
                onSelect={() => void actions.regenerate(node.id, node.parent_id, undefined, displayedInput(node.display))}
              >
                <RotateCcw />
                {t('reader.regenerate')}
              </DropdownMenuItem>
            )}
            {latest && !staticOpening && markup.trim() && (
              <DropdownMenuItem disabled={disabled} onSelect={() => setEditing({ kind: 'revise', value: '' })}>
                <PencilLine />
                {t('reader.revise')}
              </DropdownMenuItem>
            )}
            <DropdownMenuItem disabled={disabled} onSelect={() => setEditing({ kind: 'text', value: markup })}>
              <SquarePen />
              {t('reader.editText')}
            </DropdownMenuItem>
            {editableInput && (
              <DropdownMenuItem disabled={disabled} onSelect={() => setEditing({ kind: 'input', value: input.text ?? '' })}>
                <PencilLine />
                {t('reader.editInput')}
              </DropdownMenuItem>
            )}
            {!latest && (
              <DropdownMenuItem disabled={disabled} onSelect={() => void actions.backHere(node.id)}>
                <CornerUpLeft />
                {t('reader.backHere')}
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={copy}>
              <Copy />
              {t('reader.copyText')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <Dialog open={editing !== null} onOpenChange={(open) => !open && setEditing(null)}>
          <DialogContent className="sm:max-w-2xl">
            <DialogHeader>
              <DialogTitle>
                {editing?.kind === 'revise'
                  ? t('reader.revise')
                  : editing?.kind === 'input'
                    ? t('reader.editInputTitle')
                    : t('reader.editTextTitle')}
              </DialogTitle>
              <DialogDescription>
                {editing?.kind === 'revise'
                  ? t('reader.reviseHint')
                  : editing?.kind === 'input'
                    ? t('reader.editInputHint')
                    : t('reader.editTextHint')}
              </DialogDescription>
            </DialogHeader>
            <Textarea
              aria-label={t(
                editing?.kind === 'revise' ? 'reader.revise' : editing?.kind === 'input' ? 'reader.editInputTitle' : 'reader.editTextTitle',
              )}
              value={editing?.value ?? ''}
              onChange={(e) => setEditing((cur) => cur && { ...cur, value: e.target.value })}
              className="max-h-[55dvh] min-h-40 font-serif text-[15px] leading-7"
              autoFocus
            />
            <DialogFooter>
              <Button variant="ghost" onClick={() => setEditing(null)}>
                {t('common.cancel')}
              </Button>
              <Button onClick={() => void save()} disabled={saving || !editing?.value.trim()}>
                {editing?.kind === 'revise' ? t('reader.revise') : editing?.kind === 'input' ? t('reader.regenerate') : t('common.save')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </div>
  )
}
