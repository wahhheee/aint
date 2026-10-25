import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'
import { qk, useVersionActions } from '@/api/queries'
import type { WorkDetail } from '@/api/types'
import { confirmAction } from '@/components/common/confirm'
import { Button } from '@/components/ui/button'
import { ImportDialog } from '@/features/library/ImportDialog'
import { useWorkImport } from '@/features/library/useWorkImport'
import { formatDateTime } from '@/lib/format'

export function VersionList({ work }: { work: WorkDetail }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const actions = useVersionActions(work.id)
  const imp = useWorkImport()
  const [importing, setImporting] = useState(false)
  const busy = actions.check.isPending || actions.remove.isPending
  const remove = async (id?: string) => {
    if (!(await confirmAction(t(id ? 'distribution.deleteVersionConfirm' : 'distribution.deleteWorkConfirm')))) return
    try {
      await actions.remove.mutateAsync(id)
      const removedWork = !id || work.versions.length === 1
      if (removedWork) await navigate('/')
      await qc.invalidateQueries({ queryKey: qk.works, refetchType: removedWork ? 'none' : 'active' })
      if (removedWork) await qc.invalidateQueries({ queryKey: qk.works, exact: true })
    } catch (e) {
      toast.error(errorMessage(e))
    }
  }
  return (
    <>
      <ul className="divide-y divide-rule">
        {work.versions.map((v) => (
          <li key={v.id} className="space-y-3 py-5">
            <div className="flex flex-wrap items-center gap-3">
              <span className="font-medium tabular">{v.version}</span>
              <span className="text-sm text-ink-muted">{t(`work.sources.${v.source.type}`)}</span>
              {v.id === work.latest.id && (
                <span className="rounded-md bg-gold-soft px-1.5 py-0.5 text-xs text-gold-text">{t('work.latest')}</span>
              )}
              <span className="ml-auto text-xs text-ink-faint tabular">{formatDateTime(v.imported_at)}</span>
            </div>
            <p className="text-xs break-all text-ink-muted">{v.source.repo || v.source.url || v.source.path || v.source.name}</p>
            {v.changelog && <p className="text-sm whitespace-pre-wrap text-ink-muted">{v.changelog}</p>}
            {v.source.ref && (
              <p className="text-xs break-all text-ink-faint">
                {v.source.ref} · {v.source.subdir || '/'} · {v.source.commit}
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              {(v.source.type === 'github' || v.source.type === 'url') && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={() =>
                    actions.check.mutate(v.id, {
                      onSuccess: (result) => {
                        if (result.preview) {
                          imp.showPreview(result.preview)
                          setImporting(true)
                        }
                        if (!result.changed) toast(t('distribution.noUpdate'))
                      },
                      onError: (e) => toast.error(errorMessage(e)),
                    })
                  }
                >
                  {t('distribution.checkUpdate')}
                </Button>
              )}
              <Button variant="outline" size="sm" asChild>
                <a href={api.works.exportURL(work.id, v.id)} download>
                  {t('distribution.export')}
                </a>
              </Button>
              <Button variant="ghost" size="sm" disabled={busy} onClick={() => void remove(v.id)}>
                {t('distribution.deleteVersion')}
              </Button>
            </div>
          </li>
        ))}
      </ul>
      <Button className="mt-6" variant="destructive" disabled={busy || work.save_count > 0} onClick={() => void remove()}>
        {t('distribution.deleteWork')}
      </Button>
      {work.save_count > 0 && <p className="mt-2 text-xs text-ink-muted">{t('distribution.deleteReferenced')}</p>}
      <ImportDialog open={importing} onOpenChange={setImporting} imp={imp} />
    </>
  )
}
