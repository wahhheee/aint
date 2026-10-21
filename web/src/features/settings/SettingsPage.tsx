/**
 * 设置（《04-交互与界面设计》9）：左侧分区导航，右侧固定宽度的内容列。设置的权威来源是后端（GET/PATCH /settings）。
 *
 * 版式固定：导航与内容的顶部对齐，每个分区的标题都在同一位置；内容列宽度固定，
 * 不随分区内容的多少伸缩。窄屏时导航变为横向一排。
 */
import { BookOpen, Cpu, HardDrive, Info, Wrench, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { NavLink, useParams } from 'react-router'

import { PageHeader } from '@/components/common/PageHeader'
import { cn } from '@/lib/utils'

import { AboutSection } from './sections/AboutSection'
import { DeveloperSection } from './sections/DeveloperSection'
import { ProvidersSection } from './sections/ProvidersSection'
import { ReadingSection } from './sections/ReadingSection'
import { StorageSection } from './sections/StorageSection'

const SECTIONS = ['providers', 'reading', 'storage', 'developer', 'about'] as const
type Section = (typeof SECTIONS)[number]

const ICONS: Record<Section, LucideIcon> = { providers: Cpu, reading: BookOpen, storage: HardDrive, developer: Wrench, about: Info }

export function SettingsPage() {
  const { t } = useTranslation()
  const { section } = useParams()
  const current: Section = SECTIONS.includes(section as Section) ? (section as Section) : 'providers'

  return (
    <>
      <PageHeader title={t('settings.title')} />
      <div className="grid gap-8 lg:grid-cols-[184px_minmax(0,680px)] lg:gap-14">
        <nav aria-label={t('settings.title')} className="lg:sticky lg:top-10 lg:self-start">
          <ul className="no-scrollbar -mx-5 flex gap-1 overflow-x-auto px-5 sm:-mx-8 sm:px-8 lg:mx-0 lg:flex-col lg:gap-0.5 lg:overflow-visible lg:px-0">
            {SECTIONS.map((s) => {
              const Icon = ICONS[s]
              const active = s === current
              return (
                <li key={s} className="shrink-0">
                  <NavLink
                    to={`/settings/${s}`}
                    aria-current={active ? 'page' : undefined}
                    className={cn(
                      'group flex h-9 items-center gap-3 rounded-lg px-3 text-sm whitespace-nowrap text-ink-muted transition-colors hover:bg-hover hover:text-ink',
                      active && 'bg-selected text-ink',
                    )}
                  >
                    <Icon
                      className={cn('size-4 shrink-0', active ? 'text-gold' : 'text-ink-faint group-hover:text-ink-muted')}
                      strokeWidth={1.75}
                    />
                    {t(`settings.sections.${s}`)}
                  </NavLink>
                </li>
              )
            })}
          </ul>
        </nav>
        <div className="min-w-0">
          {current === 'providers' && <ProvidersSection />}
          {current === 'reading' && <ReadingSection />}
          {current === 'storage' && <StorageSection />}
          {current === 'developer' && <DeveloperSection />}
          {current === 'about' && <AboutSection />}
        </div>
      </div>
    </>
  )
}
