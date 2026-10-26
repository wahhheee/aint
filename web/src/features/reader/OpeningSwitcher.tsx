import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'
import { qk } from '@/api/queries'
import type { Presentation } from '@/api/types'
import { confirmAction } from '@/components/common/confirm'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { OpeningSelect } from '@/features/save/OpeningSelect'
import { selectBusy, useReader } from '@/stores/reader'

/** 只在开场尚可切换时提供入口，生成或断线期间保留入口并禁用。 */
export function OpeningSwitcher({ presentation }: { presentation: Presentation }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const busy = useReader(selectBusy)
  const connection = useReader((s) => s.connection)
  const compacting = useReader((s) => s.compacting)
  const { save, opening } = presentation
  const change = useMutation({
    mutationFn: (id: string) => api.saves.switchOpening(save.id, id, save.head_node_id),
    onSuccess: (next) => {
      qc.setQueryData(qk.save(save.id), next)
      void qc.resetQueries({ queryKey: qk.timeline(save.id) })
      setOpen(false)
    },
    onError: (err) => {
      toast.error(errorMessage(err))
      void qc.invalidateQueries({ queryKey: qk.save(save.id) })
    },
  })
  if (!opening.can_switch) return null
  const disabled =
    busy || compacting || presentation.compacting || !!presentation.running_turn_id || connection !== 'open' || change.isPending
  const selected = opening.options.find((op) => op.id === save.opening_id)
  const choose = async (id: string) => {
    if (disabled || id === save.opening_id) return
    if (opening.has_unfinished_turn && !(await confirmAction(t('openings.discardUnfinished')))) return
    change.mutate(id)
  }
  return (
    <div className="mb-8 rounded-lg border border-rule bg-paper/70 px-4 py-3 text-sm">
      <div className="flex items-center justify-between gap-3">
        <span className="font-serif text-ink-muted">{t('openings.current', { title: selected?.title ?? t('openings.select') })}</span>
        <Button variant="ghost" size="sm" disabled={disabled} onClick={() => setOpen(true)}>
          {t('openings.switch')}
        </Button>
      </div>
      <p className="mt-1 text-xs leading-5 text-ink-faint">{t('openings.hint')}</p>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('openings.switch')}</DialogTitle>
            <DialogDescription>{t('openings.switchHint')}</DialogDescription>
          </DialogHeader>
          <OpeningSelect options={opening.options} value={save.opening_id} onChange={(id) => void choose(id)} disabled={disabled} />
        </DialogContent>
      </Dialog>
    </div>
  )
}
