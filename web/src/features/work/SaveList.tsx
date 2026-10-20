import { Ellipsis } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import { errorMessage } from '@/api/errors'
import { useDeleteSave, useRenameSave } from '@/api/queries'
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
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { formatRelative, saveTitle } from '@/lib/format'

/** 作品的存档列表。存档每回合自动保存；游玩与分支都在存档内部进行，只有"开始新故事"会新增存档。 */
export function SaveList({ workId, saves, onStart }: { workId: string; saves?: Save[]; onStart: () => void }) {
  const { t } = useTranslation()
  const [renaming, setRenaming] = useState<Save | null>(null)
  const [deleting, setDeleting] = useState<Save | null>(null)
  const rename = useRenameSave()
  const remove = useDeleteSave()

  if (!saves) return null
  if (saves.length === 0) {
    return (
      <div className="py-10 text-center">
        <p className="text-ink-muted">{t('work.noSaves')}</p>
        <Button className="mt-4" onClick={onStart}>
          {t('work.startNew')}
        </Button>
      </div>
    )
  }

  return (
    <>
      <ul className="divide-y divide-rule">
        {saves.map((s) => (
          <li key={s.id} className="flex items-center gap-4 py-3.5">
            <Link to={`/play/${s.id}`} className="min-w-0 flex-1 rounded-md outline-offset-4">
              <p className="truncate font-serif text-[15px] font-semibold">{saveTitle(s)}</p>
              <p className="mt-0.5 text-xs text-ink-faint">
                <span className="tabular">{t('work.turns', { n: s.turn_count })}</span>
                <span className="ml-4">{formatRelative(s.last_played_at)}</span>
              </p>
            </Link>
            <Button asChild variant="outline" size="sm">
              <Link to={`/play/${s.id}`}>{t('work.resume')}</Link>
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon-sm" aria-label="更多操作">
                  <Ellipsis />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setRenaming(s)}>{t('common.rename')}</DropdownMenuItem>
                {/* TODO(M5-13)：生成标题；TODO(M3-5)：复制为新存档；TODO(M6-6)：升级到新版本 */}
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
          <DialogFooter className="mt-4">
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
