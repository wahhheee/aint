import { ArrowLeft } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router'

import { useWork, useWorkSaves } from '@/api/queries'
import type { WorkDetail } from '@/api/types'
import { LoadError } from '@/components/common/LoadError'
import { SafeHTML } from '@/components/common/SafeHTML'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { WorkCover } from '@/features/library/WorkCover'
import { NewGameDialog } from '@/features/save/NewGameDialog'
import { formatDateTime } from '@/lib/format'

import { ReportList } from './ReportList'
import { SaveList } from './SaveList'

/** 作品详情（《04-交互与界面设计》6）：简介、存档、版本、校验。 */
export function WorkDetailPage() {
  const { workId = '' } = useParams()
  const { t } = useTranslation()
  const work = useWork(workId)
  const saves = useWorkSaves(workId)
  const [starting, setStarting] = useState(false)

  if (work.isError) return <LoadError error={work.error} onRetry={() => void work.refetch()} />
  if (!work.data) return <DetailSkeleton />
  const w = work.data

  return (
    <div>
      <Link to="/" className="mb-8 inline-flex items-center gap-1.5 text-[13px] text-ink-muted hover:text-ink">
        <ArrowLeft className="size-4" />
        {t('nav.library')}
      </Link>

      <div className="grid gap-10 md:grid-cols-[220px_1fr] md:gap-14">
        <WorkCover id={w.id} title={w.title} dev={w.latest.is_dev} className="mx-auto w-48 md:w-full" />
        <div className="min-w-0">
          <h1 className="title-book text-[32px] leading-tight tracking-[0.12em]">{w.title}</h1>
          {w.subtitle && <p className="mt-3 font-serif text-[17px] text-ink-muted">{w.subtitle}</p>}
          {w.description && <p className="mt-6 max-w-prose text-[15px] leading-relaxed text-ink">{w.description}</p>}
          <Meta work={w} />
          {w.tags.length > 0 && (
            <ul className="mt-5 flex flex-wrap gap-2">
              {w.tags.map((tag) => (
                <li key={tag} className="rounded-md bg-paper-sunken px-2 py-0.5 text-xs text-ink-muted">
                  {tag}
                </li>
              ))}
            </ul>
          )}
          {w.requires.vision !== 'none' && <p className="mt-5 text-xs text-warn">{t(`work.vision.${w.requires.vision}`)}</p>}
          <div className="mt-8 flex gap-3">
            <Button size="lg" className="px-6" onClick={() => setStarting(true)}>
              {t('work.startNew')}
            </Button>
          </div>
        </div>
      </div>

      <Tabs defaultValue={saves.data?.length ? 'saves' : 'about'} className="mt-14" key={saves.data ? 'ready' : 'loading'}>
        <TabsList variant="line" className="border-b border-rule">
          <TabsTrigger value="about">{t('work.tabs.about')}</TabsTrigger>
          <TabsTrigger value="saves">
            {t('work.tabs.saves')}
            {saves.data && <span className="tabular text-ink-faint">{saves.data.length}</span>}
          </TabsTrigger>
          <TabsTrigger value="versions">{t('work.tabs.versions')}</TabsTrigger>
          <TabsTrigger value="report">{t('work.tabs.report')}</TabsTrigger>
        </TabsList>
        <TabsContent value="about" className="pt-8">
          {w.readme_html ? (
            <SafeHTML html={w.readme_html} className="max-w-prose" />
          ) : (
            <p className="text-ink-muted">{t('work.noReadme')}</p>
          )}
        </TabsContent>
        <TabsContent value="saves" className="pt-6">
          <SaveList workId={w.id} saves={saves.data} onStart={() => setStarting(true)} />
        </TabsContent>
        <TabsContent value="versions" className="pt-6">
          <ul className="divide-y divide-rule">
            {w.versions.map((v) => (
              <li key={v.id} className="flex items-baseline gap-4 py-3">
                <span className="tabular w-20 font-medium">{v.version}</span>
                <span className="text-ink-muted">{t(`work.sources.${v.source.type}`)}</span>
                {v.id === w.latest.id && <span className="text-xs text-gold-text">{t('work.latest')}</span>}
                <span className="ml-auto text-xs text-ink-faint">{formatDateTime(v.imported_at)}</span>
              </li>
            ))}
          </ul>
          {/* TODO(M6-1/M6-5)：检查更新、导出 zip、删除版本 */}
        </TabsContent>
        <TabsContent value="report" className="pt-6">
          <ReportList issues={w.report.issues} />
        </TabsContent>
      </Tabs>

      <NewGameDialog work={w} open={starting} onOpenChange={setStarting} />
    </div>
  )
}

/** 作者、版本、许可、分级：标签淡、值清楚，用间距分隔。 */
function Meta({ work }: { work: WorkDetail }) {
  const { t } = useTranslation()
  const items: [string, string][] = []
  if (work.authors.length) items.push([t('work.by'), work.authors.map((a) => a.name).join('、')])
  items.push(['版本', work.latest.version])
  if (work.license) items.push([t('work.license'), work.license])
  if (work.content_rating) items.push(['分级', t(`work.rating.${work.content_rating}`)])
  return (
    <dl className="mt-6 flex flex-wrap gap-x-8 gap-y-2 text-[13px]">
      {items.map(([k, v]) => (
        <div key={k} className="flex gap-2">
          <dt className="text-ink-faint">{k}</dt>
          <dd className="text-ink">{v}</dd>
        </div>
      ))}
    </dl>
  )
}

function DetailSkeleton() {
  return (
    <div className="grid gap-14 pt-14 md:grid-cols-[220px_1fr]">
      <Skeleton className="aspect-[5/7] w-full rounded-sm" />
      <div className="space-y-4">
        <Skeleton className="h-9 w-1/2" />
        <Skeleton className="h-5 w-2/3" />
        <Skeleton className="h-20 w-full" />
      </div>
    </div>
  )
}
