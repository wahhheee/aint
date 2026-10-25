import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'

import { errorMessage } from '@/api/errors'
import { useVersionActions, useWork } from '@/api/queries'
import type { Save } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'

export function UpgradeDialog({ save, onClose }: { save: Save; onClose: () => void }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const work = useWork(save.work_id)
  const { upgrade } = useVersionActions(save.work_id)
  const versions = work.data?.versions.filter((v) => !v.is_dev && v.id !== save.work_version_id) ?? []
  const [selected, setSelected] = useState('')
  const target = selected || versions[0]?.id || ''
  const changelog = versions.find((v) => v.id === target)?.changelog
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !upgrade.isPending) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('distribution.upgrade')}</DialogTitle>
          <DialogDescription>{t('distribution.upgradeHint')}</DialogDescription>
        </DialogHeader>
        {work.isError && (
          <p role="alert" className="text-sm text-danger">
            {errorMessage(work.error)}
          </p>
        )}
        {versions.length ? (
          <Label className="grid gap-2">
            {t('distribution.targetVersion')}
            <select
              className="h-10 w-full rounded-md border border-rule bg-paper px-3"
              value={target}
              onChange={(e) => setSelected(e.target.value)}
            >
              {versions.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.version} · {t(`work.sources.${v.source.type}`)}
                </option>
              ))}
            </select>
          </Label>
        ) : (
          <p className="text-sm text-ink-muted">{t('distribution.noTarget')}</p>
        )}
        {changelog && <p className="max-h-48 overflow-auto text-sm whitespace-pre-wrap text-ink-muted">{changelog}</p>}
        <DialogFooter>
          <Button variant="ghost" disabled={upgrade.isPending} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={!target || upgrade.isPending}
            onClick={() =>
              upgrade.mutate(
                { id: save.id, versionId: target },
                {
                  onSuccess: () => {
                    onClose()
                    void navigate(`/play/${save.id}`)
                  },
                  onError: (e) => toast.error(errorMessage(e)),
                },
              )
            }
          >
            {t('distribution.startUpgrade')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
