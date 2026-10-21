import { ChevronRight } from 'lucide-react'
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

/**
 * 作品详情（《04-交互与界面设计》6）：简介、存档、版本、校验。
 *
 *   书架 › 雾港来信                              ← 与其他页面的标题同一行
 *   ┌──────┐  雾港来信
 *   │ 封面 │  副标题
 *   │      │  简述
 *   │      │  作者 / 版本 / 许可 / 分级
 *   └──────┘  标签 · [开始新故事] [继续]
 *   简介  存档 3  版本  校验
 */
export function WorkDetailPage() {
  const { workId = '' } = useParams()
  const { t } = useTranslation()
  const work = useWork(workId)
  const saves = useWorkSaves(workId)
  const [starting, setStarting] = useState(false)

  if (work.isError) {
    return (
      <>
        <Breadcrumb />
        <LoadError error={work.error} onRetry={() => void work.refetch()} />
      </>
    )
  }
  if (!work.data) return <DetailSkeleton />
  const w = work.data
  const lastSave = saves.data?.[0]

  return (
    <div>
      <Breadcrumb title={w.title} />

      <div className="grid gap-8 sm:grid-cols-[176px_minmax(0,1fr)] md:grid-cols-[208px_minmax(0,1fr)] md:gap-12">
        <WorkCover id={w.id} title={w.title} dev={w.latest.is_dev} className="w-40 sm:w-full" />
        <div className="min-w-0">
          <h1 className="title-book text-[32px] leading-[1.25] tracking-[0.1em] md:text-[36px]">{w.title}</h1>
          {w.subtitle && <p className="mt-2 font-serif text-base leading-7 text-ink-muted">{w.subtitle}</p>}
          {w.description && <p className="mt-5 max-w-[60ch] text-[15px] leading-7 text-ink">{w.description}</p>}
          <Meta work={w} />
          {w.tags.length > 0 && (
            <ul className="mt-5 flex flex-wrap gap-1.5">
              {w.tags.map((tag) => (
                <li key={tag} className="rounded-md border border-rule px-2 py-0.5 text-xs leading-5 text-ink-muted">
                  {tag}
                </li>
              ))}
            </ul>
          )}
          {w.requires.vision !== 'none' && <p className="mt-4 text-xs text-warn">{t(`work.vision.${w.requires.vision}`)}</p>}
          <div className="mt-8 flex flex-wrap gap-3">
            <Button size="lg" onClick={() => setStarting(true)}>
              {t('work.startNew')}
            </Button>
            {lastSave && (
              <Button asChild size="lg" variant="outline">
                <Link to={`/play/${lastSave.id}`}>{t('work.resumeLast')}</Link>
              </Button>
            )}
          </div>
        </div>
      </div>

      <Tabs defaultValue={saves.data?.length ? 'saves' : 'about'} className="mt-14 gap-0" key={saves.data ? 'ready' : 'loading'}>
        <TabsList variant="line" className="w-full justify-start">
          <TabsTrigger value="about">{t('work.tabs.about')}</TabsTrigger>
          <TabsTrigger value="saves">
            {t('work.tabs.saves')}
            {saves.data && <span className="text-xs font-normal text-ink-faint tabular">{saves.data.length}</span>}
          </TabsTrigger>
          <TabsTrigger value="versions">{t('work.tabs.versions')}</TabsTrigger>
          <TabsTrigger value="report">
            {t('work.tabs.report')}
            {w.report.issues.length > 0 && <span className="text-xs font-normal text-warn tabular">{w.report.issues.length}</span>}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="about" className="pt-8">
          {w.readme_html ? (
            <SafeHTML html={w.readme_html} className="max-w-[68ch]" />
          ) : (
            <p className="text-ink-muted">{t('work.noReadme')}</p>
          )}
        </TabsContent>
        <TabsContent value="saves" className="pt-4">
          <SaveList workId={w.id} saves={saves.data} onStart={() => setStarting(true)} />
        </TabsContent>
        <TabsContent value="versions" className="pt-4">
          <ul className="divide-y divide-rule">
            {w.versions.map((v) => (
              <li key={v.id} className="flex h-14 items-center gap-6 px-1">
                <span className="w-20 font-medium text-ink tabular">{v.version}</span>
                <span className="text-ink-muted">{t(`work.sources.${v.source.type}`)}</span>
                {v.id === w.latest.id && (
                  <span className="rounded-md bg-gold-soft px-1.5 py-0.5 text-xs leading-4 text-gold-text">{t('work.latest')}</span>
                )}
                <span className="ml-auto text-xs text-ink-faint tabular">{formatDateTime(v.imported_at)}</span>
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

/** 面包屑：占据页面标题行（40px），与书架、设置的标题同一位置。 */
function Breadcrumb({ title }: { title?: string }) {
  const { t } = useTranslation()
  return (
    <nav aria-label="位置" className="mb-10 flex h-10 items-center gap-1.5 text-[13px]">
      <Link to="/" className="rounded-md text-ink-muted transition-colors hover:text-ink">
        {t('nav.library')}
      </Link>
      {title && (
        <>
          <ChevronRight aria-hidden className="size-3.5 text-ink-faint" />
          <span className="truncate text-ink" aria-current="page">
            {title}
          </span>
        </>
      )}
    </nav>
  )
}

/** 作者、版本、许可、分级：小号标签在上、值在下，等宽排成一行。 */
function Meta({ work }: { work: WorkDetail }) {
  const { t } = useTranslation()
  const items: [string, string][] = []
  if (work.authors.length) items.push([t('work.by'), work.authors.map((a) => a.name).join('、')])
  items.push([t('work.version'), work.latest.version])
  if (work.license) items.push([t('work.license'), work.license])
  if (work.content_rating) items.push([t('work.ratingLabel'), t(`work.rating.${work.content_rating}`)])
  return (
    <dl className="mt-6 flex flex-wrap gap-x-10 gap-y-4">
      {items.map(([k, v]) => (
        <div key={k} className="min-w-0">
          <dt className="text-xs leading-5 text-ink-faint">{k}</dt>
          <dd className="mt-0.5 text-[13px] leading-5 text-ink">{v}</dd>
        </div>
      ))}
    </dl>
  )
}

function DetailSkeleton() {
  return (
    <div>
      <div className="mb-10 h-10" />
      <div className="grid gap-8 sm:grid-cols-[176px_minmax(0,1fr)] md:grid-cols-[208px_minmax(0,1fr)] md:gap-12">
        <Skeleton className="aspect-[5/7] w-40 rounded-sm sm:w-full" />
        <div className="space-y-4">
          <Skeleton className="h-10 w-1/2" />
          <Skeleton className="h-5 w-2/3" />
          <Skeleton className="h-20 w-full" />
        </div>
      </div>
    </div>
  )
}
