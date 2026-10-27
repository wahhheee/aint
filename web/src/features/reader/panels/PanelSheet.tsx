/**
 * 面板抽屉（《03-作品格式规范》11.1、《04-交互与界面设计》8.3）：drawer 面板是抽屉中的标签页；
 * 窄屏（< 1200 px）没有右侧栏，sidebar 面板也逐个成为标签页。手机上从底部拉出，平板上从右侧滑出。
 */
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { PanelMeta, PanelRender } from '@/api/types'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { useMediaQuery } from '@/lib/useMediaQuery'

import { byPlace } from './layout'
import { PanelHTML } from './PanelHTML'

export function PanelSheet({
  open,
  onOpenChange,
  meta,
  rendered,
  includeSidebar,
  title,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  meta: PanelMeta[]
  rendered?: PanelRender[]
  /** 窄屏：sidebar 面板也放进抽屉 */
  includeSidebar: boolean
  title: string
}) {
  const { t } = useTranslation()
  const panels = [...(includeSidebar ? byPlace(meta, rendered, 'sidebar') : []), ...byPlace(meta, rendered, 'drawer')]
  const phone = useMediaQuery('(max-width: 767px)')
  const [tab, setTab] = useState<string>()
  const active = panels.some((p) => p.meta.id === tab) ? tab : panels[0]?.meta.id

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side={phone ? 'bottom' : 'right'}
        // 抽屉渲染在页面之外（portal），加上 work-root 使作品主题与样式标签在抽屉中同样生效
        className={cn('work-root gap-0 bg-paper', phone ? 'max-h-[75dvh] rounded-t-2xl px-5 pb-6' : 'w-[340px] px-6')}
      >
        <SheetHeader className="px-0 pt-5 pb-3">
          <SheetTitle className="title-book text-base">{title}</SheetTitle>
        </SheetHeader>
        {panels.length === 0 ? (
          <p className="py-8 text-center text-[13px] text-ink-faint">{t('reader.panelsEmpty')}</p>
        ) : (
          <Tabs value={active} onValueChange={setTab} className="min-h-0 flex-1">
            {panels.length > 1 && (
              <TabsList variant="line" className="w-full shrink-0 justify-start gap-5 overflow-x-auto">
                {panels.map(({ meta: m }) => (
                  <TabsTrigger key={m.id} value={m.id} className="flex-none">
                    {m.title ?? m.id}
                  </TabsTrigger>
                ))}
              </TabsList>
            )}
            {panels.map(({ meta: m, render }) => (
              <TabsContent key={m.id} value={m.id} className="min-h-0 overflow-y-auto pt-4">
                <PanelHTML html={render.html} />
              </TabsContent>
            ))}
          </Tabs>
        )}
      </SheetContent>
    </Sheet>
  )
}
