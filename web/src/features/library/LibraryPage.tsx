import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'

import { useRecentSaves, useWorks } from '@/api/queries'
import type { Save, WorkSummary } from '@/api/types'
import { LoadError } from '@/components/common/LoadError'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatRelative, saveTitle } from '@/lib/format'

import { coverColor } from './coverColor'
import { ImportDialog } from './ImportDialog'
import { WorkCover } from './WorkCover'

/** 书架（首页）：继续阅读 + 全部作品。 */
export function LibraryPage() {
  const { t } = useTranslation()
  const works = useWorks()
  const recent = useRecentSaves()
  const [importing, setImporting] = useState(false)

  return (
    <div className="space-y-14">
      {recent.data && recent.data.length > 0 && (
        <section aria-labelledby="continue-heading">
          <h2 id="continue-heading" className="title-book mb-5 text-lg">
            {t('library.continueReading')}
          </h2>
          <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {recent.data.map((s) => (
              <li key={s.id}>
                <SaveSlip save={s} />
              </li>
            ))}
          </ul>
        </section>
      )}

      <section aria-labelledby="shelf-heading">
        <div className="mb-6 flex items-end justify-between gap-4">
          <h2 id="shelf-heading" className="title-book text-lg">
            {t('library.shelf')}
          </h2>
          <Button variant="outline" size="sm" onClick={() => setImporting(true)}>
            <Plus />
            {t('library.import')}
          </Button>
        </div>

        {works.isPending && <ShelfSkeleton />}
        {works.isError && <LoadError error={works.error} onRetry={() => void works.refetch()} />}
        {works.data?.length === 0 && <EmptyShelf onImport={() => setImporting(true)} />}
        {works.data && works.data.length > 0 && (
          <ul className="grid grid-cols-2 gap-x-6 gap-y-10 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
            {works.data.map((w) => (
              <li key={w.id}>
                <ShelfBook work={w} />
              </li>
            ))}
          </ul>
        )}
      </section>

      <ImportDialog open={importing} onOpenChange={setImporting} />
    </div>
  )
}

/** 书架上的一本书：封面 + 书名 + 一行副标题。 */
function ShelfBook({ work }: { work: WorkSummary }) {
  const { t } = useTranslation()
  return (
    <Link to={`/works/${encodeURIComponent(work.id)}`} className="group block rounded-sm outline-offset-4">
      <WorkCover
        id={work.id}
        title={work.title}
        dev={work.latest.is_dev}
        className="transition-transform duration-200 ease-out group-hover:-translate-y-1"
      />
      <div className="mt-3.5 space-y-1">
        <p className="font-serif text-[15px] leading-snug font-semibold tracking-[0.04em] text-ink">{work.title}</p>
        {work.subtitle && <p className="line-clamp-1 text-xs text-ink-muted">{work.subtitle}</p>}
        <p className="text-xs text-ink-faint">{t('library.saveCount', { count: work.save_count })}</p>
      </div>
    </Link>
  )
}

/** 继续阅读：像夹在书里的书签条，左侧色带是作品封面的颜色。 */
function SaveSlip({ save }: { save: Save }) {
  const { t } = useTranslation()
  return (
    <Link
      to={`/play/${save.id}`}
      className="group flex h-full overflow-hidden rounded-xl border border-rule bg-paper-raised transition-colors hover:border-gold/50"
    >
      <span aria-hidden className="w-1.5 shrink-0" style={{ backgroundColor: coverColor(save.work_id) }} />
      <span className="flex min-w-0 flex-1 flex-col gap-1 px-4 py-3.5">
        <span className="truncate font-serif text-[15px] font-semibold text-ink">{saveTitle(save)}</span>
        <span className="truncate text-xs text-ink-muted">{save.work_title}</span>
        <span className="mt-1.5 flex items-center justify-between text-xs text-ink-faint">
          <span className="tabular">{t('library.turnNo', { n: save.turn_count })}</span>
          <span>{formatRelative(save.last_played_at)}</span>
        </span>
      </span>
    </Link>
  )
}

function ShelfSkeleton() {
  return (
    <div className="grid grid-cols-2 gap-x-6 gap-y-10 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
      {Array.from({ length: 4 }, (_, i) => (
        <div key={i}>
          <Skeleton className="aspect-[5/7] w-full rounded-sm" />
          <Skeleton className="mt-3.5 h-4 w-2/3" />
        </div>
      ))}
    </div>
  )
}

function EmptyShelf({ onImport }: { onImport: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col items-center rounded-2xl border border-dashed border-rule px-6 py-20 text-center">
      <div aria-hidden className="mb-6 h-24 w-[68px] rounded-[2px_4px_4px_2px] border border-dashed border-ink-faint/60" />
      <p className="title-book text-lg">{t('library.emptyTitle')}</p>
      <p className="mt-2 max-w-sm text-ink-muted">{t('library.emptyBody')}</p>
      <Button className="mt-6" onClick={onImport}>
        <Plus />
        {t('library.import')}
      </Button>
    </div>
  )
}
