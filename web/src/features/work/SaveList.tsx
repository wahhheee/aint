import { Ellipsis } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { confirmAction } from '@/components/common/confirm'
import { errorMessage } from '@/api/errors'
import { useCopySave, useDeleteSave, useRenameSave, useSaveState, useWork } from '@/api/queries'
import type { Save } from '@/api/types'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { formatRelative, saveTitle } from '@/lib/format'
import { UpgradeDialog } from './UpgradeDialog'

/** 作品的存档列表。存档每回合自动保存；游玩与分支都在存档内部进行，只有"开始新故事"会新增存档。 */
export function SaveList({ workId, saves, onStart }: { workId: string; saves?: Save[]; onStart: () => void }) {
  const { t } = useTranslation()
  const [renaming, setRenaming] = useState<Save | null>(null)
  const [deleting, setDeleting] = useState<Save | null>(null)
  const [upgrading, setUpgrading] = useState<Save | null>(null)
  const [migration, setMigration] = useState<Save | null>(null)
  const work = useWork(workId)
  const rename = useRenameSave()
  const remove = useDeleteSave()
  const copyMutation = useCopySave()
  const copy = (s: Save) =>
    copyMutation.mutate(
      { id: s.id, nodeId: s.head_node_id },
      { onSuccess: () => toast(t('work.copied')), onError: (e) => toast.error(errorMessage(e)) },
    )

  if (!saves) return null
  if (saves.length === 0) {
    return (
      <div className="flex flex-col items-center py-16 text-center">
        <p className="text-ink-muted">{t('work.noSaves')}</p>
        <Button className="mt-5" onClick={onStart}>
          {t('work.startNew')}
        </Button>
      </div>
    )
  }

  return (
    <>
      <ul className="-mx-3">
        {saves.map((s) => (
          <li key={s.id} className="relative flex min-h-16 items-center gap-3 rounded-lg px-3 py-3 transition-colors hover:bg-hover">
            <Link to={`/play/${s.id}`} className="min-w-0 flex-1 rounded-md outline-offset-4 after:absolute after:inset-0">
              <p className="truncate font-serif text-[15px] leading-6 font-semibold text-ink">{saveTitle(s)}</p>
              <p className="flex flex-wrap gap-x-4 text-xs leading-5 text-ink-faint">
                <span>{work.data?.versions.find((v) => v.id === s.work_version_id)?.version}</span>
                <span className="tabular">{t('work.turns', { n: s.turn_count })}</span>
                {s.branch_count > 1 && <span className="tabular">{t('work.branches', { count: s.branch_count })}</span>}
                {s.forked_from && (
                  <span className="truncate">
                    {t('work.forkedFrom', { title: s.forked_from.title || t('work.untitled'), turn: s.forked_from.turn_no })}
                  </span>
                )}
                <span>{formatRelative(s.last_played_at)}</span>
              </p>
            </Link>
            <Button asChild variant="outline" size="sm" className="relative">
              <Link to={`/play/${s.id}`} tabIndex={-1}>
                {t('work.resume')}
              </Link>
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon-sm" aria-label={t('common.more')} className="relative">
                  <Ellipsis />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setRenaming(s)}>{t('common.rename')}</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => copy(s)}>{t('work.copySave')}</DropdownMenuItem>
                <DropdownMenuItem
                  onSelect={() =>
                    void (async () => {
                      if (await confirmAction(t('background.titleCost'))) {
                        try {
                          await api.saves.title(s.id)
                          toast(t('background.titleStarted'))
                        } catch (err) {
                          toast.error(errorMessage(err))
                        }
                      }
                    })()
                  }
                >
                  {t('background.generateTitle')}
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setUpgrading(s)}>{t('distribution.upgrade')}</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setMigration(s)}>{t('distribution.migration')}</DropdownMenuItem>
                <DropdownMenuItem variant="destructive" onSelect={() => setDeleting(s)}>
                  {t('common.delete')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </li>
        ))}
      </ul>

      <RenameDialog
        save={renaming}
        onClose={() => setRenaming(null)}
        onSubmit={(title) =>
          rename.mutate({ id: renaming!.id, title }, { onSuccess: () => setRenaming(null), onError: (e) => toast.error(errorMessage(e)) })
        }
      />
      {upgrading && <UpgradeDialog save={upgrading} onClose={() => setUpgrading(null)} />}
      {migration && <MigrationDialog save={migration} onClose={() => setMigration(null)} />}

      <AlertDialog open={deleting !== null} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('work.deleteSaveTitle')}</AlertDialogTitle>
            <AlertDialogDescription>{t('work.deleteSaveBody')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => remove.mutate({ id: deleting!.id, workId }, { onError: (e) => toast.error(errorMessage(e)) })}
            >
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function MigrationDialog({ save, onClose }: { save: Save; onClose: () => void }) {
  const { t } = useTranslation()
  const view = useSaveState(save.id, true)
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('distribution.migration')}</DialogTitle>
          <DialogDescription>{t('distribution.migrationHint')}</DialogDescription>
        </DialogHeader>
        {view.isPending ? (
          <p>{t('common.loading')}</p>
        ) : view.error ? (
          <p role="alert">{errorMessage(view.error)}</p>
        ) : (
          <pre className="max-h-[60dvh] overflow-auto whitespace-pre-wrap break-all text-xs">
            {JSON.stringify(view.data?.migration ?? [], null, 2)}
          </pre>
        )}
      </DialogContent>
    </Dialog>
  )
}

function RenameDialog({ save, onClose, onSubmit }: { save: Save | null; onClose: () => void; onSubmit: (title: string) => void }) {
  const { t } = useTranslation()
  const [title, setTitle] = useState('')
  return (
    <Dialog
      open={save !== null}
      onOpenChange={(o) => {
        if (o && save) setTitle(save.title)
        if (!o) onClose()
      }}
    >
      <DialogContent className="sm:max-w-md" onOpenAutoFocus={() => save && setTitle(save.title)}>
        <DialogHeader>
          <DialogTitle>{t('common.rename')}</DialogTitle>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            onSubmit(title)
          }}
        >
          <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t('newGame.saveNamePlaceholder')} autoFocus />
          <DialogFooter className="mt-5">
            <Button type="button" variant="ghost" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button type="submit">{t('common.confirm')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
