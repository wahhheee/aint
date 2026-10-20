/**
 * 导入作品（《04-交互与界面设计》5）：文件 / 文件夹 / GitHub / 链接 / 开发目录。
 *
 * 流程：解析与校验（不写入）→ 展示报告 → 确认导入。
 * TODO(M1-4 / M6-1 / M6-2)：后端导入接口完成后接入；目前提交会收到 501，界面如实展示服务端的说明。
 */
import { useMutation } from '@tanstack/react-query'
import { FolderOpen, GitBranch, Link2, PackageOpen, SquareTerminal } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

type Source = 'file' | 'folder' | 'github' | 'url' | 'dev'

export function ImportDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation()
  const [source, setSource] = useState<Source>('github')
  const [value, setValue] = useState('')
  const preview = useMutation({ mutationFn: (body: unknown) => api.works.importPreview(body) })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const body = source === 'github' ? { github: value } : source === 'url' ? { url: value } : { dir: value }
    preview.mutate(body)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="title-book">{t('import.title')}</DialogTitle>
          <DialogDescription>{t('import.pending')}</DialogDescription>
        </DialogHeader>

        <Tabs value={source} onValueChange={(v) => setSource(v as Source)}>
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
            {/* TODO(M1-4)：拖拽 zip 到窗口任意位置即可导入（书架页监听 drop） */}
            <DropZone hint={t('import.fileHint')} />
          </TabsContent>
          <TabsContent value="folder">
            {/* TODO(M1-4)：<input webkitdirectory> 选择目录，以多文件 + 相对路径上传 */}
            <DropZone hint={t('import.folderHint')} />
          </TabsContent>
          {(['github', 'url', 'dev'] as const).map((s) => (
            <TabsContent key={s} value={s}>
              <form onSubmit={submit} className="space-y-2 pt-2">
                <Label htmlFor={`import-${s}`}>
                  {s === 'github' ? t('import.githubLabel') : s === 'url' ? t('import.urlLabel') : t('import.devLabel')}
                </Label>
                <div className="flex gap-2">
                  <Input
                    id={`import-${s}`}
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                    placeholder={
                      s === 'github'
                        ? 'https://github.com/someone/misty-harbor/tree/v0.3.0'
                        : s === 'url'
                          ? 'https://example.com/work.zip'
                          : 'D:\\works\\misty-harbor'
                    }
                  />
                  <Button type="submit" disabled={!value || preview.isPending}>
                    {t('import.parse')}
                  </Button>
                </div>
                <p className="text-xs text-ink-faint">{s === 'github' ? t('import.githubHint') : s === 'dev' ? t('import.devHint') : ''}</p>
              </form>
            </TabsContent>
          ))}
        </Tabs>

        {preview.isError && (
          <p role="alert" className="rounded-lg border-l-2 border-warn bg-paper-sunken px-4 py-3 text-ink-muted">
            {errorMessage(preview.error)}
          </p>
        )}
      </DialogContent>
    </Dialog>
  )
}

function DropZone({ hint }: { hint: string }) {
  return (
    <div className="mt-2 flex h-36 items-center justify-center rounded-xl border border-dashed border-rule bg-paper-sunken/50 text-ink-muted">
      {hint}
    </div>
  )
}
