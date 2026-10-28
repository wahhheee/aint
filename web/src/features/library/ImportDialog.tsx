/**
 * 导入作品（《04-交互与界面设计》5）：文件（zip）/ 文件夹 / GitHub / 链接 / 开发目录。
 *
 *   选择（或把 zip 包、文件夹拖到书架上）→ 上传（进度）→ 检查结果：作品摘要、与书架上已有作品的关系、校验报告
 *   → 导入（有 error 时不能导入）→ 完成：查看作品
 *
 * 流程状态在 useWorkImport（书架页持有，拖入书架页的内容也从那里开始）；这里只负责呈现。
 * 链接与开发目录同样先预览校验结果，再确认导入。
 */
import { Check, Copy, FolderOpen, GitBranch, Link2, LoaderCircle, PackageOpen, SquareTerminal, type LucideIcon } from 'lucide-react'
import { useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'

import { errorMessage, isApiError } from '@/api/errors'
import type { ImportPreview, ImportResult } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ReportList } from '@/features/work/ReportList'
import { formatBytes } from '@/lib/format'
import { cn } from '@/lib/utils'

import { countIssues, progressPercent, reportText, shelfRelation, sortIssues } from './importReport'
import { IMPORT_LIMITS, pickedFromFileList, pickedFromZip } from './importSource'
import type { ImportStage, WorkImport } from './useWorkImport'
import { WorkCover } from './WorkCover'

type Tab = 'file' | 'folder' | 'github' | 'url' | 'dev'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  imp: WorkImport
}

export function ImportDialog({ open, onOpenChange, imp }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [tab, setTab] = useState<Tab>('file')

  // 关闭即放弃：取消上传、删除服务端暂存；已导入的不受影响
  const setOpen = (next: boolean) => {
    if (!next) imp.reset()
    onOpenChange(next)
  }
  const viewWork = (workId: string) => {
    setOpen(false)
    void navigate(`/works/${encodeURIComponent(workId)}`)
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('import.title')}</DialogTitle>
          <DialogDescription>{t('import.description')}</DialogDescription>
        </DialogHeader>
        {imp.stage.step === 'select' ? (
          <SourceTabs tab={tab} onTab={setTab} imp={imp} />
        ) : (
          <StageView stage={imp.stage} imp={imp} onClose={() => setOpen(false)} onViewWork={viewWork} />
        )}
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------- 选择

function SourceTabs({ tab, onTab, imp }: { tab: Tab; onTab: (t: Tab) => void; imp: WorkImport }) {
  const { t } = useTranslation()
  const limit = t('import.limitHint', { size: formatBytes(IMPORT_LIMITS.maxTotal), count: IMPORT_LIMITS.maxFiles })
  return (
    <Tabs value={tab} onValueChange={(v) => onTab(v as Tab)}>
      <TabsList className="w-full">
        <TabsTrigger value="file">
          <PackageOpen />
          {t('import.tabs.file')}
        </TabsTrigger>
        <TabsTrigger value="folder">
          <FolderOpen />
          {t('import.tabs.folder')}
        </TabsTrigger>
        <TabsTrigger value="github">
          <GitBranch />
          {t('import.tabs.github')}
        </TabsTrigger>
        <TabsTrigger value="url">
          <Link2 />
          {t('import.tabs.url')}
        </TabsTrigger>
        <TabsTrigger value="dev">
          <SquareTerminal />
          {t('import.tabs.dev')}
        </TabsTrigger>
      </TabsList>

      <TabsContent value="file">
        <PickZone icon={PackageOpen} drop={t('import.fileDrop')} hint={limit}>
          <FilePicker label={t('import.pickZip')} accept=".zip,application/zip" onFiles={(f) => imp.start(pickedFromZip(f))} />
        </PickZone>
      </TabsContent>
      <TabsContent value="folder">
        <PickZone icon={FolderOpen} drop={t('import.folderDrop')} hint={t('import.folderHint')}>
          <FilePicker label={t('import.pickFolder')} directory onFiles={(f) => imp.start(pickedFromFileList(f))} />
        </PickZone>
      </TabsContent>
      {(['github', 'url', 'dev'] as const).map((s) => (
        <TabsContent key={s} value={s}>
          <RemoteSource source={s} imp={imp} />
        </TabsContent>
      ))}
    </Tabs>
  )
}

/** 选择区：虚线框里一个图标、"拖到这里"与选择按钮。拖放由书架页整页处理，这里只是提示。 */
function PickZone({ icon: Icon, drop, hint, children }: { icon: LucideIcon; drop: string; hint: string; children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <div className="mt-4 flex h-48 flex-col items-center justify-center rounded-xl border border-dashed border-rule-strong bg-paper-sunken/60 px-6 text-center">
      <Icon aria-hidden className="size-6 text-ink-faint" strokeWidth={1.5} />
      <p className="mt-3 text-[13px] text-ink-muted">
        {drop}
        <span className="mx-1.5 text-ink-faint">{t('import.or')}</span>
      </p>
      <div className="mt-3">{children}</div>
      <p className="mt-3 text-xs leading-5 text-ink-faint">{hint}</p>
    </div>
  )
}

/** 选择文件的按钮。directory 时选择整个文件夹（webkitdirectory：多文件，各带相对路径）。 */
function FilePicker({
  label,
  accept,
  directory,
  onFiles,
}: {
  label: string
  accept?: string
  directory?: boolean
  onFiles: (files: FileList | null) => void
}) {
  const input = useRef<HTMLInputElement>(null)
  return (
    <>
      <Button variant="outline" size="sm" onClick={() => input.current?.click()}>
        {label}
      </Button>
      <input
        ref={(el) => {
          input.current = el
          // React 的类型里没有 webkitdirectory，直接设置 DOM 属性
          if (el && directory) el.setAttribute('webkitdirectory', '')
        }}
        type="file"
        hidden
        tabIndex={-1}
        accept={accept}
        onChange={(e) => {
          onFiles(e.target.files)
          e.target.value = '' // 允许再次选择同一个文件
        }}
      />
    </>
  )
}

/** 链接从服务端下载；目录路径相对于运行服务的电脑。 */
function RemoteSource({ source, imp }: { source: 'github' | 'url' | 'dev'; imp: WorkImport }) {
  const { t } = useTranslation()
  const [value, setValue] = useState('')
  const [ref, setRef] = useState('')
  const [subdir, setSubdir] = useState('')
  const [timeout, setTimeoutValue] = useState('120')
  const [limit, setLimit] = useState('528')
  const label = source === 'github' ? t('import.githubLabel') : source === 'url' ? t('import.urlLabel') : t('import.devLabel')
  const placeholder =
    source === 'github'
      ? 'https://github.com/someone/inverted-stars/tree/v0.3.0'
      : source === 'url'
        ? 'https://example.com/work.zip'
        : 'D:\\works\\inverted-stars'
  return (
    <form
      className="space-y-2.5 pt-4"
      onSubmit={(e) => {
        e.preventDefault()
        imp.startRemote(
          source === 'dev'
            ? { source, path: value.trim() }
            : { source, url: value.trim(), ref, subdir, timeout_seconds: Number(timeout), max_download_mib: Number(limit) },
        )
      }}
    >
      <Label htmlFor={`import-${source}`}>{label}</Label>
      <div className="flex gap-2">
        <Input id={`import-${source}`} placeholder={placeholder} value={value} onChange={(e) => setValue(e.target.value)} required />
        <Button type="submit" disabled={!value.trim() || imp.busy}>
          {t('import.parse')}
        </Button>
      </div>
      <p className="text-xs leading-5 text-ink-faint">
        {source === 'github' ? t('import.githubHint') : source === 'dev' ? t('import.devHint') : ''}
      </p>
      {source === 'dev' ? (
        <Notice>{t('distribution.devHint')}</Notice>
      ) : (
        <details className="text-sm">
          <summary className="cursor-pointer text-ink-muted">{t('distribution.advanced')}</summary>
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            {source === 'github' && (
              <>
                <Label className="grid gap-2">
                  {t('distribution.ref')}
                  <Input value={ref} onChange={(e) => setRef(e.target.value)} />
                </Label>
                <Label className="grid gap-2">
                  {t('distribution.subdir')}
                  <Input value={subdir} onChange={(e) => setSubdir(e.target.value)} />
                </Label>
              </>
            )}
            <Label className="grid gap-2">
              {t('distribution.timeout')}
              <Input type="number" min="5" max="300" value={timeout} onChange={(e) => setTimeoutValue(e.target.value)} />
            </Label>
            <Label className="grid gap-2">
              {t('distribution.downloadLimit')}
              <Input type="number" min="1" max="528" value={limit} onChange={(e) => setLimit(e.target.value)} />
            </Label>
          </div>
        </details>
      )}
    </form>
  )
}

// ---------------------------------------------------------------- 上传、检查结果、完成

interface StageProps {
  stage: Exclude<ImportStage, { step: 'select' }>
  imp: WorkImport
  onClose: () => void
  onViewWork: (workId: string) => void
}

function StageView({ stage, imp, onClose, onViewWork }: StageProps) {
  const { t } = useTranslation()
  switch (stage.step) {
    case 'reading':
      return <Working text={t('import.reading')} />
    case 'uploading': {
      const sent = stage.total > 0 && stage.loaded >= stage.total
      return (
        <div className="space-y-3 py-2" aria-live="polite">
          <Working text={sent ? t('import.checking', { name: stage.name }) : t('import.uploading', { name: stage.name })} />
          <Progress value={progressPercent(stage.loaded, stage.total)} aria-label={t('import.uploading', { name: stage.name })} />
          <p className="text-right text-xs text-ink-faint tabular">
            {t('import.progress', { loaded: formatBytes(stage.loaded), total: formatBytes(stage.total) })}
          </p>
          <DialogFooter>
            <Button variant="ghost" onClick={onClose}>
              {t('common.cancel')}
            </Button>
          </DialogFooter>
        </div>
      )
    }
    case 'failed':
      return (
        <div className="space-y-5">
          <Alert tone="danger">
            {stage.message}
            {stage.requestId && (
              <span className="mt-1 block text-xs text-ink-faint">
                {t('common.requestId')}：<span className="tabular select-all">{stage.requestId}</span>
              </span>
            )}
          </Alert>
          <DialogFooter>
            <Button variant="ghost" onClick={onClose}>
              {t('common.close')}
            </Button>
            <Button onClick={imp.reset}>{t('import.reselect')}</Button>
          </DialogFooter>
        </div>
      )
    case 'preview':
      return <PreviewView preview={stage.preview} imp={imp} onClose={onClose} />
    case 'done':
      return <DoneView preview={stage.preview} result={stage.result} imp={imp} onViewWork={onViewWork} />
  }
}

function PreviewView({ preview: p, imp, onClose }: { preview: ImportPreview; imp: WorkImport; onClose: () => void }) {
  const { t } = useTranslation()
  const n = countIssues(p.report.issues)
  const relation = shelfRelation(p)
  const w = p.work
  const meta = [
    w.id,
    w.authors.length > 0 && `${t('work.by')} ${w.authors.map((a) => a.name).join('、')}`,
    w.content_rating && t(`work.rating.${w.content_rating}`),
    w.requires.vision !== 'none' && t(`work.vision.${w.requires.vision}`),
  ].filter(Boolean)
  const source = [p.source.name, t('import.files', { n: p.files }), formatBytes(p.size)].filter(Boolean)

  const copyReport = () => {
    navigator.clipboard.writeText(reportText(p)).then(
      () => toast(t('common.copied')),
      (err: unknown) => toast.error(errorMessage(err)),
    )
  }

  return (
    <div className="space-y-6">
      <section className="flex gap-4">
        <WorkCover id={w.id || p.tree_hash} title={w.title || '?'} className="w-16 shrink-0" />
        <div className="min-w-0">
          <p className="title-book text-xl leading-8">
            {w.title || p.source.name}
            {w.version && <span className="ml-2 font-sans text-sm font-normal tracking-normal text-ink-muted tabular">v{w.version}</span>}
          </p>
          {w.subtitle && <p className="font-serif text-[13px] leading-6 text-ink-muted">{w.subtitle}</p>}
          <p className="mt-1 text-xs leading-5 text-ink-faint">{meta.join(' · ')}</p>
          <p className="text-xs leading-5 text-ink-faint">{source.join(' · ')}</p>
          {w.tags.length > 0 && <p className="mt-1 text-xs leading-5 text-ink-muted">{w.tags.join(' · ')}</p>}
        </div>
      </section>

      {relation !== 'new' && p.existing && (
        <Notice>
          {relation === 'sameVersion'
            ? t('import.shelf.sameVersion', { version: p.existing.same_version })
            : t('import.shelf.newVersion', { title: p.existing.title, version: p.existing.latest_version })}
        </Notice>
      )}

      <section>
        <div className="mb-3 flex items-center gap-3">
          <h3 className="text-[13px] font-medium text-ink">{t('import.report')}</h3>
          <p className="flex gap-3 text-xs text-ink-muted">
            {p.report.issues.length === 0 ? (
              <span className="inline-flex items-center gap-1 text-ok">
                <Check className="size-3.5" />
                {t('import.reportClean')}
              </span>
            ) : (
              (['error', 'warning', 'info'] as const)
                .filter((l) => n[l] > 0)
                .map((l) => (
                  <span key={l} className={cn(l === 'error' && 'text-danger', l === 'warning' && 'text-warn')}>
                    {t(`import.counts.${l}`, { n: n[l] })}
                  </span>
                ))
            )}
          </p>
          {p.report.issues.length > 0 && (
            <Button variant="ghost" size="xs" className="ml-auto" onClick={copyReport}>
              <Copy />
              {t('import.copyReport')}
            </Button>
          )}
        </div>
        {p.report.issues.length > 0 && <ReportList issues={sortIssues(p.report.issues)} className="max-h-72 overflow-y-auto" />}
      </section>

      {!p.task && <Alert tone="danger">{t('import.blocked', { n: n.error })}</Alert>}
      {imp.confirmError != null && (
        <Alert tone="danger">
          {errorMessage(imp.confirmError)}
          {isApiError(imp.confirmError, 'not_found') && (
            <Button variant="link" size="xs" className="ml-1 h-auto px-0" onClick={imp.reset}>
              {t('import.reselect')}
            </Button>
          )}
        </Alert>
      )}

      <DialogFooter className="sm:justify-between">
        <Button variant="ghost" onClick={imp.reset} disabled={imp.confirming}>
          {t('import.reselect')}
        </Button>
        <div className="flex flex-col-reverse gap-2 sm:flex-row">
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={imp.confirm} disabled={!p.task || imp.confirming} className="min-w-24">
            {imp.confirming && <LoaderCircle className="animate-spin" />}
            {t('import.confirm')}
          </Button>
        </div>
      </DialogFooter>
    </div>
  )
}

function DoneView({
  preview: p,
  result,
  imp,
  onViewWork,
}: {
  preview: ImportPreview
  result: ImportResult
  imp: WorkImport
  onViewWork: (workId: string) => void
}) {
  const { t } = useTranslation()
  const vars = { title: p.work.title, version: p.work.version }
  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3 py-2" role="status">
        <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-ok/12 text-ok">
          <Check className="size-4" />
        </span>
        <div>
          <p className="text-[15px] text-ink">{result.created ? t('import.doneCreated', vars) : t('import.doneReused', vars)}</p>
          {p.existing && <p className="mt-1 text-[13px] text-ink-muted">{t('import.doneHint')}</p>}
        </div>
      </div>
      <DialogFooter>
        <Button variant="ghost" onClick={imp.reset}>
          {t('import.another')}
        </Button>
        <Button onClick={() => onViewWork(result.work_id)} autoFocus>
          {t('import.viewWork')}
        </Button>
      </DialogFooter>
    </div>
  )
}

// ---------------------------------------------------------------- 小部件

function Working({ text }: { text: string }) {
  return (
    <p className="flex items-center gap-2 text-[13px] text-ink-muted" role="status">
      <LoaderCircle aria-hidden className="size-4 animate-spin text-ink-faint" />
      {text}
    </p>
  )
}

function Notice({ children }: { children: ReactNode }) {
  return <p className="rounded-xl border border-rule bg-paper-sunken/60 px-4 py-3 text-[13px] leading-6 text-ink-muted">{children}</p>
}

function Alert({ tone, children }: { tone: 'danger'; children: ReactNode }) {
  return (
    <div
      role="alert"
      className={cn('rounded-xl border px-4 py-3 text-[13px] leading-6 text-ink', tone === 'danger' && 'border-danger/30 bg-danger/8')}
    >
      {children}
    </div>
  )
}
