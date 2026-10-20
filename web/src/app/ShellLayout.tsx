import { useTranslation } from 'react-i18next'
import { NavLink, Outlet } from 'react-router'

import { useSystemInfo } from '@/api/queries'
import { Brand } from '@/components/brand/BrandMark'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

/** 书架、作品详情、设置共用的外壳：顶部一条安静的导航。 */
export function ShellLayout() {
  const { t } = useTranslation()
  const { data: info } = useSystemInfo()

  return (
    <div className="min-h-dvh">
      <header className="sticky top-0 z-30 border-b border-transparent bg-background/85 backdrop-blur-sm supports-[backdrop-filter]:bg-background/70">
        <div className="mx-auto flex h-16 max-w-6xl items-center gap-6 px-6">
          <Brand />
          {info?.fake_llm && (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="rounded-full border border-gold/40 px-2 py-0.5 text-xs text-gold-text">{t('app.fakeBadge')}</span>
              </TooltipTrigger>
              <TooltipContent className="max-w-64">{t('app.fakeHint')}</TooltipContent>
            </Tooltip>
          )}
          <nav className="ml-auto flex items-center gap-1" aria-label="主导航">
            <ShellLink to="/" end>
              {t('nav.library')}
            </ShellLink>
            <ShellLink to="/settings">{t('nav.settings')}</ShellLink>
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-6 pt-6 pb-24">
        <Outlet />
      </main>
    </div>
  )
}

function ShellLink({ to, end, children }: { to: string; end?: boolean; children: React.ReactNode }) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        cn(
          'relative rounded-md px-3 py-1.5 text-[13px] text-ink-muted transition-colors hover:text-ink',
          isActive && 'text-ink after:absolute after:inset-x-3 after:-bottom-[13px] after:h-px after:bg-gold',
        )
      }
    >
      {children}
    </NavLink>
  )
}
