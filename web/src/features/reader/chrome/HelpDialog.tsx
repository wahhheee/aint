import { useTranslation } from 'react-i18next'

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'

import { paletteEntries } from '../input/paletteEntries'
import { useReaderContext } from '../ReaderContext'

/** /help：本地显示命令表，不消耗 token（PLY-13）。 */
export function HelpDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation()
  const { ui } = useReaderContext()
  const entries = paletteEntries(ui, '')
  const modes = ui.commands.filter((c) => c.present.includes('mode'))
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="title-book">{t('reader.help')}</DialogTitle>
          <DialogDescription>{t('reader.helpBody')}</DialogDescription>
        </DialogHeader>
        <dl className="space-y-3">
          {entries.map((e) => (
            <div key={e.name}>
              <dt className="font-mono text-[13px]">{e.signature}</dt>
              <dd className="text-[13px] text-ink-muted">{e.description}</dd>
            </div>
          ))}
          {modes.map((m) => (
            <div key={m.name}>
              <dt className="text-[13px]">输入模式「{m.mode_label || m.title}」</dt>
              <dd className="text-[13px] text-ink-muted">{m.description}</dd>
            </div>
          ))}
        </dl>
      </DialogContent>
    </Dialog>
  )
}
