/**
 * 设置（《04-交互与界面设计》9）：左侧分组导航，右侧表单。设置的权威来源是后端（GET/PATCH /settings）。
 */
import { useTranslation } from 'react-i18next'
import { NavLink, useParams } from 'react-router'

import { cn } from '@/lib/utils'

import { AboutSection } from './sections/AboutSection'
import { DeveloperSection } from './sections/DeveloperSection'
import { ProvidersSection } from './sections/ProvidersSection'
import { ReadingSection } from './sections/ReadingSection'
import { StorageSection } from './sections/StorageSection'

const SECTIONS = ['providers', 'reading', 'storage', 'developer', 'about'] as const
type Section = (typeof SECTIONS)[number]

export function SettingsPage() {
  const { t } = useTranslation()
  const { section } = useParams()
  const current: Section = SECTIONS.includes(section as Section) ? (section as Section) : 'providers'

  return (
    <div className="grid gap-10 md:grid-cols-[180px_1fr]">
      <nav aria-label={t('settings.title')} className="md:sticky md:top-24 md:self-start">
        <h1 className="title-book mb-5 text-lg">{t('settings.title')}</h1>
        <ul className="flex gap-1 overflow-x-auto md:flex-col">
          {SECTIONS.map((s) => (
            <li key={s}>
              <NavLink
                to={`/settings/${s}`}
                className={cn(
                  'block rounded-md px-3 py-1.5 text-[13px] whitespace-nowrap text-ink-muted transition-colors hover:bg-accent hover:text-ink',
                  s === current && 'bg-accent text-ink',
                )}
              >
                {t(`settings.sections.${s}`)}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
      <div className="min-w-0 max-w-2xl md:pt-12">
        {current === 'providers' && <ProvidersSection />}
        {current === 'reading' && <ReadingSection />}
        {current === 'storage' && <StorageSection />}
        {current === 'developer' && <DeveloperSection />}
        {current === 'about' && <AboutSection />}
      </div>
    </div>
  )
}
