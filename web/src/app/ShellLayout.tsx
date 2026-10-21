/**
 * 书架、作品详情、设置共用的外壳。
 *
 *   宽屏（≥ 1024px）                         窄屏
 *   ┌──────────┬──────────────────────┐     ┌──────────────────────┐
 *   │ 品牌      │ 页面标题        操作   │     │ 品牌          书架 设置 │ ← 顶栏
 *   │ 书架      │                      │     ├──────────────────────┤
 *   │ 设置      │ 内容（唯一的滚动区）   │     │ 页面标题        操作   │
 *   │ 作品      │                      │     │ 内容                  │
 *   │  ▌雾港来信 │                      │     │                      │
 *   │ 演示模型 ◐ │                      │     │                      │
 *   └──────────┴──────────────────────┘     └──────────────────────┘
 *
 * 外壳本身不滚动（h-dvh），只有内容区滚动，并且始终预留滚动条的位置（.scroll-stable）：
 * 切换页面或标签时内容长短变化，侧栏、顶栏与内容的左边缘都不会移动。
 */
import { LibraryBig, Settings2, type LucideIcon } from 'lucide-react'
import { useLayoutEffect, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { NavLink, Outlet, useLocation, useMatch } from 'react-router'

import { useSystemInfo, useWorks } from '@/api/queries'
import { Brand } from '@/components/brand/BrandMark'
import { ThemeSwitch } from '@/components/common/ThemeSwitch'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { coverColor } from '@/features/library/coverColor'
import { cn } from '@/lib/utils'

export function ShellLayout() {
  const mainRef = useRef<HTMLElement>(null)
  const { pathname } = useLocation()
  // 内容区是独立的滚动容器，换页时回到顶部
  useLayoutEffect(() => {
    mainRef.current?.scrollTo({ top: 0 })
  }, [pathname])

  return (
    <div className="flex h-dvh overflow-hidden">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar />
        <main ref={mainRef} className="scroll-stable min-h-0 flex-1">
          <div className="mx-auto w-full max-w-[1200px] px-5 pt-8 pb-24 sm:px-8 lg:px-12 lg:pt-10">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}

/** 宽屏的左侧栏：品牌、主导航、作品目录（书脊色 + 书名）、演示模型提示与外观切换。 */
function Sidebar() {
  const { t } = useTranslation()
  const works = useWorks()
  const current = useMatch('/works/:workId')?.params.workId

  return (
    <aside className="hidden w-(--sidebar-width) shrink-0 flex-col border-r border-rule lg:flex" aria-label="侧栏">
      <div className="mt-10 flex h-10 shrink-0 items-center px-6">
        <Brand />
      </div>

      <nav className="mt-8 space-y-0.5 px-3" aria-label="主导航">
        <SideLink to="/" end icon={LibraryBig}>
          {t('nav.library')}
        </SideLink>
        <SideLink to="/settings" icon={Settings2}>
          {t('nav.settings')}
        </SideLink>
      </nav>

      {works.data && works.data.length > 0 && (
        <div className="mt-8 flex min-h-0 flex-1 flex-col">
          <p className="px-6 pb-2 text-xs text-ink-faint">{t('nav.works')}</p>
          <ul className="no-scrollbar min-h-0 flex-1 space-y-0.5 overflow-y-auto px-3 pb-4">
            {works.data.map((w) => (
              <li key={w.id}>
                <NavLink
                  to={`/works/${encodeURIComponent(w.id)}`}
                  className={cn(
                    'flex h-8 items-center gap-3 rounded-lg px-3 text-[13px] text-ink-muted transition-colors hover:bg-hover hover:text-ink',
                    current === w.id && 'bg-selected text-ink',
                  )}
                >
                  <span aria-hidden className="h-3.5 w-1 shrink-0 rounded-[1px]" style={{ backgroundColor: coverColor(w.id) }} />
                  <span className="truncate">{w.title}</span>
                </NavLink>
              </li>
            ))}
          </ul>
        </div>
      )}

      <footer className="mt-auto flex h-14 shrink-0 items-center justify-between gap-3 border-t border-rule px-5">
        <FakeModelBadge />
        <ThemeSwitch />
      </footer>
    </aside>
  )
}

function SideLink({ to, end, icon: Icon, children }: { to: string; end?: boolean; icon: LucideIcon; children: ReactNode }) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        cn(
          'group relative flex h-9 items-center gap-3 rounded-lg px-3 text-sm text-ink-muted transition-colors hover:bg-hover hover:text-ink',
          isActive && 'bg-selected text-ink',
        )
      }
    >
      {({ isActive }) => (
        <>
          <Icon
            className={cn('size-4 shrink-0', isActive ? 'text-gold' : 'text-ink-faint group-hover:text-ink-muted')}
            strokeWidth={1.75}
          />
          {children}
        </>
      )}
    </NavLink>
  )
}

/** 窄屏的顶栏：品牌 + 两个导航入口。 */
function TopBar() {
  const { t } = useTranslation()
  return (
    <header className="flex h-(--topbar-height) shrink-0 items-center gap-4 border-b border-rule px-5 sm:px-8 lg:hidden">
      <Brand />
      <FakeModelBadge compact />
      <nav className="ml-auto flex items-center gap-1" aria-label="主导航">
        <TopLink to="/" end>
          {t('nav.library')}
        </TopLink>
        <TopLink to="/settings">{t('nav.settings')}</TopLink>
      </nav>
    </header>
  )
}

function TopLink({ to, end, children }: { to: string; end?: boolean; children: ReactNode }) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        cn(
          'flex h-8 items-center rounded-lg px-3 text-sm text-ink-muted transition-colors hover:text-ink',
          isActive && 'bg-selected text-ink',
        )
      }
    >
      {children}
    </NavLink>
  )
}

/** 使用内置演示模型时的提示：一粒泥金圆点 + 文字，悬停看说明。 */
function FakeModelBadge({ compact }: { compact?: boolean }) {
  const { t } = useTranslation()
  const { data: info } = useSystemInfo()
  if (!info?.fake_llm) return null
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={0} className="inline-flex min-w-0 items-center gap-2 rounded-md text-xs text-ink-muted outline-offset-2">
          <span aria-hidden className="size-1.5 shrink-0 rounded-full bg-gold" />
          <span className={cn('truncate', compact && 'hidden sm:inline')}>{t('app.fakeBadge')}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-64">{t('app.fakeHint')}</TooltipContent>
    </Tooltip>
  )
}
