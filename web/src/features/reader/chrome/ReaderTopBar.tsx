import { ArrowLeft, BookMarked, GitBranch, PanelRight, ScanSearch } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import type { Save } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { saveTitle } from '@/lib/format'
import { useReader } from '@/stores/reader'

import { ContextRing } from './ContextRing'
import { ReadingSettingsPopover } from './ReadingSettingsPopover'

/** 阅读页顶栏：返回、作品与存档名、上下文用量、工具按钮。安静、细，不抢正文。 */
export function ReaderTopBar({
  save,
  onToggleSidebar,
  sidebarAvailable,
}: {
  save?: Save
  onToggleSidebar: () => void
  sidebarAvailable: boolean
}) {
  const { t } = useTranslation()
  const context = useReader((s) => s.context)
  const todo = () => toast(t('common.notImplemented'))

  return (
    <header className="relative flex h-14 shrink-0 items-center gap-2 px-3 sm:px-4">
      <IconButton label={t('reader.backToLibrary')} asLink="/">
        <ArrowLeft />
      </IconButton>
      <div className="flex min-w-0 items-baseline gap-3">
        <span className="truncate font-serif text-[15px] font-semibold tracking-[0.08em]">{save?.work_title}</span>
        {save && <span className="hidden truncate text-xs text-ink-faint sm:inline">{saveTitle(save)}</span>}
      </div>

      <div className="ml-auto flex items-center gap-0.5">
        <ContextRing used={context?.used} threshold={context?.threshold} />
        {/* TODO(M3-9)：记忆抽屉（浏览 / 编辑本存档记忆文件） */}
        <IconButton label={t('reader.memory')} onClick={todo}>
          <BookMarked />
        </IconButton>
        {/* TODO(M3-9)：时间线与分支抽屉 */}
        <IconButton label={t('reader.branches')} onClick={todo}>
          <GitBranch />
        </IconButton>
        <ReadingSettingsPopover />
        {/* TODO(M5-9)：检查器（概览 / 上下文 / 工具 / 状态 / 记忆 / 原始） */}
        <IconButton label={t('reader.inspector')} onClick={todo}>
          <ScanSearch />
        </IconButton>
        {sidebarAvailable && (
          <IconButton label={t('reader.panels')} onClick={onToggleSidebar}>
            <PanelRight />
          </IconButton>
        )}
      </div>
    </header>
  )
}

function IconButton({ label, children, onClick, asLink }: { label: string; children: ReactNode; onClick?: () => void; asLink?: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {asLink ? (
          <Button asChild variant="ghost" size="icon-sm" aria-label={label}>
            <Link to={asLink}>{children}</Link>
          </Button>
        ) : (
          <Button variant="ghost" size="icon-sm" aria-label={label} onClick={onClick}>
            {children}
          </Button>
        )}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}
