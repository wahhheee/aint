/**
 * 状态抽屉（开发者工具，D-31；《04-交互与界面设计》8.11）：当前进度处的完整状态（JSON），可以直接修改。
 * 保存时把有变化的顶层字段写成 set 操作（删掉的字段写成 delete，恢复默认值），产生编辑节点并以注记告知执笔者。
 * 普通玩家通过作品面板查看状态。
 */
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'
import { qk, useSaveState } from '@/api/queries'
import type { StateOp } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'

export function StateDrawer({ saveId, open, onOpenChange }: { saveId: string; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const view = useSaveState(saveId, open)
  // 编辑中的文字；为 null 时显示服务端的当前值
  const [draft, setDraft] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const original = view.data ? JSON.stringify(view.data.state, null, 2) : ''
  const text = draft ?? original

  const save = async () => {
    if (!view.data) return
    let next: Record<string, unknown>
    try {
      next = JSON.parse(text) as Record<string, unknown>
    } catch (err) {
      toast.error(t('reader.stateDrawer.invalid', { message: (err as Error).message }))
      return
    }
    const ops: StateOp[] = []
    for (const key of view.data.schema.order) {
      const before = JSON.stringify(view.data.state[key])
      if (!(key in next)) ops.push({ op: 'delete', path: key })
      else if (JSON.stringify(next[key]) !== before) ops.push({ op: 'set', path: key, value: next[key] })
    }
    if (ops.length === 0) {
      toast(t('reader.stateDrawer.unchanged'))
      return
    }
    setSaving(true)
    try {
      await api.saves.patchState(saveId, ops)
      toast(t('reader.stateDrawer.saved'))
      setDraft(null)
      void qc.invalidateQueries({ queryKey: qk.state(saveId) })
      void qc.invalidateQueries({ queryKey: qk.timeline(saveId) })
      void qc.invalidateQueries({ queryKey: qk.tree(saveId) })
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(o) => {
        if (!o) setDraft(null)
        onOpenChange(o)
      }}
    >
      <SheetContent side="right" className="w-full gap-0 bg-paper px-0 data-[side=right]:sm:max-w-lg">
        <SheetHeader className="border-b border-rule px-6 pt-5 pb-4">
          <SheetTitle className="title-book text-base">{t('reader.stateDrawer.title')}</SheetTitle>
          <SheetDescription className="text-[13px]">{t('reader.stateDrawer.intro')}</SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-3 px-5 py-4">
          <Textarea
            value={text}
            onChange={(e) => setDraft(e.target.value)}
            spellCheck={false}
            className="min-h-0 flex-1 font-mono text-[12.5px] leading-5 [field-sizing:fixed]"
          />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDraft(null)} disabled={draft === null}>
              {t('common.cancel')}
            </Button>
            <Button onClick={() => void save()} disabled={saving || draft === null}>
              {t('common.save')}
            </Button>
          </div>
          {!!view.data?.migration?.length && (
            <details className="max-h-60 overflow-auto text-sm">
              <summary className="cursor-pointer">{t('distribution.migration')}</summary>
              <p className="my-2 text-xs text-ink-muted">{t('distribution.migrationHint')}</p>
              <pre className="whitespace-pre-wrap break-all text-xs">{JSON.stringify(view.data.migration, null, 2)}</pre>
            </details>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
