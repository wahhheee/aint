import { ArrowLeft, BookMarked, Ellipsis, GitBranch, PanelRight, ScanSearch } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import type { Save } from '@/api/types'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
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
  devTools,
}: {
  save?: Save
  onToggleSidebar: () => void
  sidebarAvailable: boolean
  /** 开发者工具（D-31）：普通玩家看不到调试入口 */
  devTools: boolean
}) {
  const { t } = useTranslation()
  const context = useReader((s) => s.context)
  const todo = () => toast(t('common.notImplemented'))

  return (
    <header className="relative z-10 flex h-14 shrink-0 items-center gap-2 px-2 sm:gap-3 sm:px-4">
      <IconButton label={t('reader.backToLibrary')} asLink="/">
        <ArrowLeft />
      </IconButton>
      <div className="min-w-0 leading-tight">
        <p className="truncate font-serif text-[15px] font-semibold tracking-[0.06em] text-ink">{save?.work_title}</p>
        {save && <p className="truncate text-xs text-ink-faint">{saveTitle(save)}</p>}
      </div>

      <div className="ml-auto flex items-center gap-1">
        <ContextRing used={context?.used} threshold={context?.threshold} />
        <span aria-hidden className="mx-1 h-4 w-px bg-rule-strong sm:mx-1.5" />
        {/* 窄屏时记忆、分支、检查器收进"更多"菜单，给标题留出位置 */}
        <div className="hidden items-center gap-1 sm:flex">
          {/* TODO(M3-9)：记忆抽屉（浏览 / 编辑本存档记忆文件） */}
          <IconButton label={t('reader.memory')} onClick={todo}>
            <BookMarked />
          </IconButton>
          {/* TODO(M3-9)：时间线与分支抽屉 */}
          <IconButton label={t('reader.branches')} onClick={todo}>
            <GitBranch />
          </IconButton>
        </div>
        <ReadingSettingsPopover />
        {/* TODO(M5-9)：检查器（概览 / 上下文 / 工具 / 状态 / 记忆 / 原始） */}
        {devTools && (
          <span className="hidden sm:contents">
            <IconButton label={t('reader.inspector')} onClick={todo}>
              <ScanSearch />
            </IconButton>
          </span>
        )}
        {sidebarAvailable && (
          <IconButton label={t('reader.panels')} onClick={onToggleSidebar}>
            <PanelRight />
          </IconButton>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" className="sm:hidden" aria-label={t('reader.more')}>
              <Ellipsis />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={todo}>
              <BookMarked />
              {t('reader.memory')}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={todo}>
              <GitBranch />
              {t('reader.branches')}
            </DropdownMenuItem>
            {devTools && (
              <DropdownMenuItem onSelect={todo}>
                <ScanSearch />
                {t('reader.inspector')}
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
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
