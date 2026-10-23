/**
 * 记忆抽屉（《04-交互与界面设计》8.11，PLY-15）：左侧是 /memory 的文件列表，右侧是内容，可以编辑、新建、删除。
 * 记忆随分支变化：这里看到的是当前进度（head）处的记忆。玩家的修改产生编辑节点，下一回合以注记告知执笔者。
 */
import { useQueryClient } from '@tanstack/react-query'
import { FilePlus2, Pencil, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'
import { qk, useMemory, useMemoryFile } from '@/api/queries'
import type { MemoryFile } from '@/api/types'
import { confirmAction } from '@/components/common/confirm'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { formatBytes } from '@/lib/format'
import { cn } from '@/lib/utils'

type Mode = { kind: 'view' } | { kind: 'edit'; content: string } | { kind: 'new'; path: string; content: string }

export function MemoryDrawer({ saveId, open, onOpenChange }: { saveId: string; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const memory = useMemory(saveId, open)
  const [selected, setSelected] = useState<string | null>(null)
  const [mode, setMode] = useState<Mode>({ kind: 'view' })
  const [saving, setSaving] = useState(false)
  const files = memory.data?.files ?? []
  const current = selected && files.some((f) => f.path === selected) ? selected : null
  const file = useMemoryFile(saveId, open ? current : null)

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: qk.memory(saveId) })
    void qc.invalidateQueries({ queryKey: qk.timeline(saveId) })
    void qc.invalidateQueries({ queryKey: qk.tree(saveId) })
  }
  const select = (path: string) => {
    setSelected(path)
    setMode({ kind: 'view' })
  }
  const save = async (path: string, content: string) => {
    setSaving(true)
    try {
      const res = await api.saves.putMemory(saveId, path, content)
      toast(res.node_id ? t('reader.memoryDrawer.saved') : t('reader.memoryDrawer.unchanged'))
      refresh()
      setSelected(normalize(path))
      setMode({ kind: 'view' })
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }
  const remove = async (path: string) => {
    if (!(await confirmAction(t('reader.memoryDrawer.deleteConfirm', { path })))) return
    try {
      await api.saves.deleteMemory(saveId, path)
      toast(t('reader.memoryDrawer.deleted'))
      setSelected(null)
      refresh()
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }
  // 点"第 N 回合修改"跳到正文中的那一回合（已加载时）
  const jumpToTurn = (turn: number) => {
    const el = document.querySelector(`[data-turn="${turn}"]`)
    if (el) {
      onOpenChange(false)
      el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full gap-0 bg-paper px-0 data-[side=right]:sm:max-w-3xl">
        <SheetHeader className="border-b border-rule px-6 pt-5 pb-4">
          <SheetTitle className="title-book text-base">{t('reader.memoryDrawer.title')}</SheetTitle>
          <SheetDescription className="text-[13px]">{t('reader.memoryDrawer.intro')}</SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col sm:flex-row">
          <aside className="flex max-h-[35dvh] shrink-0 flex-col border-b border-rule sm:max-h-none sm:w-60 sm:border-r sm:border-b-0">
            <div className="flex items-center justify-between px-4 pt-3 pb-2">
              <span className="text-xs text-ink-faint tabular">
                {memory.data &&
                  t('reader.memoryDrawer.total', {
                    count: files.length,
                    size: formatBytes(memory.data.total),
                    limit: formatBytes(memory.data.limit),
                  })}
              </span>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={t('reader.memoryDrawer.new')}
                onClick={() => setMode({ kind: 'new', path: '/memory/', content: '' })}
              >
                <FilePlus2 />
              </Button>
            </div>
            <ul className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
              {files.length === 0 && memory.data && (
                <li className="px-2 py-6 text-[13px] text-ink-faint">{t('reader.memoryDrawer.empty')}</li>
              )}
              {files.map((f) => (
                <li key={f.path}>
                  <button
                    type="button"
                    onClick={() => select(f.path)}
                    className={cn(
                      'w-full rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-hover',
                      f.path === current && mode.kind !== 'new' && 'bg-selected',
                    )}
                  >
                    <span className="block truncate font-mono text-[12.5px] text-ink">{f.path.replace(/^\/memory\//, '')}</span>
                    {f.summary && <span className="block truncate text-xs text-ink-faint">{f.summary}</span>}
                  </button>
                </li>
              ))}
            </ul>
          </aside>
          <section className="flex min-h-0 min-w-0 flex-1 flex-col">
            {mode.kind === 'new' ? (
              <Editor
                title={t('reader.memoryDrawer.newTitle')}
                path={mode.path}
                onPath={(path) => setMode({ ...mode, path })}
                content={mode.content}
                onContent={(content) => setMode({ ...mode, content })}
                saving={saving}
                onCancel={() => setMode({ kind: 'view' })}
                onSave={() => void save(mode.path, mode.content)}
              />
            ) : current && file.data ? (
              mode.kind === 'edit' ? (
                <Editor
                  title={current}
                  content={mode.content}
                  onContent={(content) => setMode({ kind: 'edit', content })}
                  saving={saving}
                  onCancel={() => setMode({ kind: 'view' })}
                  onSave={() => void save(current, mode.content)}
                />
              ) : (
                <Viewer
                  file={file.data}
                  onEdit={() => setMode({ kind: 'edit', content: file.data.content ?? '' })}
                  onDelete={() => void remove(current)}
                  onJump={jumpToTurn}
                />
              )
            ) : (
              <p className="m-auto px-6 py-10 text-[13px] text-ink-faint">{t('reader.memoryDrawer.pick')}</p>
            )}
          </section>
        </div>
      </SheetContent>
    </Sheet>
  )
}

function Viewer({
  file,
  onEdit,
  onDelete,
  onJump,
}: {
  file: MemoryFile
  onEdit: () => void
  onDelete: () => void
  onJump: (turn: number) => void
}) {
  const { t } = useTranslation()
  return (
    <>
      <div className="flex items-center gap-2 border-b border-rule px-5 py-2.5">
        <div className="min-w-0 flex-1">
          <p className="truncate font-mono text-[13px] text-ink">{file.path}</p>
          <p className="text-xs text-ink-faint">
            {file.turn > 0 ? (
              <button type="button" className="underline-offset-2 hover:underline" onClick={() => onJump(file.turn)}>
                {t('reader.memoryDrawer.turn', { turn: file.turn })}
              </button>
            ) : (
              t('reader.memoryDrawer.template')
            )}
            <span className="ml-3 tabular">{formatBytes(file.size)}</span>
          </p>
        </div>
        <Button variant="ghost" size="icon-sm" aria-label={t('common.edit')} onClick={onEdit}>
          <Pencil />
        </Button>
        <Button variant="ghost" size="icon-sm" aria-label={t('common.delete')} onClick={onDelete}>
          <Trash2 />
        </Button>
      </div>
      <pre className="min-h-0 flex-1 overflow-y-auto px-5 py-4 font-serif text-[14.5px] leading-7 break-words whitespace-pre-wrap text-ink">
        {file.content}
      </pre>
    </>
  )
}

function Editor({
  title,
  path,
  onPath,
  content,
  onContent,
  saving,
  onCancel,
  onSave,
}: {
  title: string
  path?: string
  onPath?: (path: string) => void
  content: string
  onContent: (content: string) => void
  saving: boolean
  onCancel: () => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 px-5 py-4">
      <p className="truncate font-mono text-[13px] text-ink">{title}</p>
      {onPath && (
        <label className="block text-xs text-ink-muted">
          {t('reader.memoryDrawer.path')}
          <Input value={path} onChange={(e) => onPath(e.target.value)} className="mt-1 font-mono" autoFocus />
          <span className="mt-1 block text-ink-faint">{t('reader.memoryDrawer.pathHint')}</span>
        </label>
      )}
      <Textarea
        value={content}
        onChange={(e) => onContent(e.target.value)}
        aria-label={t('reader.memoryDrawer.content')}
        className="min-h-48 flex-1 font-serif text-[14.5px] leading-7 [field-sizing:fixed]"
        autoFocus={!onPath}
      />
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel}>
          {t('common.cancel')}
        </Button>
        <Button onClick={onSave} disabled={saving || (path !== undefined && !/^\/memory\/.+\.(md|txt|yaml)$/.test(path.trim()))}>
          {t('common.save')}
        </Button>
      </div>
    </div>
  )
}

/** 统一为以 /memory/ 开头的路径。 */
function normalize(path: string): string {
  const p = path.trim().replace(/^\/+/, '')
  return '/' + (p.startsWith('memory/') ? p : 'memory/' + p)
}
