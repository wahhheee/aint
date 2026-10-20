/**
 * 阅读页（《04-交互与界面设计》8）——游玩界面首先是一本书，其次才是应用。
 *
 *   ┌──────────────────────────────────────────────┬──────────────┐
 *   │ 顶栏：返回 · 作品 / 存档 · 用量环 · 工具        │              │
 *   │ 顶栏面板（局势一句话）                          │  侧栏面板     │
 *   │                                              │  （笔记本、   │
 *   │            正文流（居中的版心）                 │   封印……）    │
 *   │                                    悬浮面板   │              │
 *   │ 输入栏                                        │              │
 *   └──────────────────────────────────────────────┴──────────────┘
 *
 * 组件分工：
 *   useReaderSession   SSE → reader store / 查询缓存
 *   useTurnActions     提交输入、选择、停止
 *   stream/            正文流（已提交回合 + 草稿回合）
 *   input/             输入栏与命令面板
 *   panels/            面板（净化后的 HTML + ui-* 组件）
 *   chrome/            顶栏、用量环、阅读设置、帮助
 *
 * TODO(M5-2)：舞台层（背景、转场、BGM）。TODO(M5-13)：退出时 POST /saves/{id}/exit 与 pagehide 信标。
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useParams } from 'react-router'

import { usePanels, usePresentation, useSettings, useTimeline } from '@/api/queries'
import { LoadError } from '@/components/common/LoadError'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { MarkupProvider, useMarkupContext } from '@/markup/context'
import { workTagCSS } from '@/markup/workStyles'
import { selectBusy, useReader } from '@/stores/reader'
import { useUI } from '@/stores/ui'

import { HelpDialog } from './chrome/HelpDialog'
import { ReaderTopBar } from './chrome/ReaderTopBar'
import { InputDock, type InputDockHandle } from './input/InputDock'
import { hasSidebarPanels } from './panels/layout'
import { FloatPanels, SidebarPanels, TopbarPanels } from './panels/Panels'
import { ReaderContext, type ReaderContextValue } from './ReaderContext'
import { lastSceneBackground } from './stage/scene'
import { StageBackground } from './stage/StageBackground'
import { Stream } from './stream/Stream'
import { useReaderSession } from './useReaderSession'
import { useReadingStyle } from './useReadingStyle'
import { useTurnActions } from './useTurnActions'

export function ReaderPage() {
  const { saveId = '' } = useParams()
  const presentation = usePresentation(saveId)
  const timeline = useTimeline(saveId)
  const panels = usePanels(saveId)
  const settings = useSettings()
  useReaderSession(saveId)

  if (presentation.isError) {
    return (
      <div className="mx-auto max-w-lg px-6 py-[18vh]">
        <LoadError error={presentation.error} onRetry={() => void presentation.refetch()} />
      </div>
    )
  }
  if (!presentation.data) return <div className="h-dvh" aria-busy />

  return (
    <ReaderView
      saveId={saveId}
      presentation={presentation.data}
      timeline={timeline.data}
      panels={panels.data?.panels}
      reading={settings.data?.reading}
      devTools={!!settings.data?.developer || presentation.data.dev_work}
    />
  )
}

type ViewProps = {
  saveId: string
  presentation: NonNullable<ReturnType<typeof usePresentation>['data']>
  timeline: ReturnType<typeof useTimeline>['data']
  panels: NonNullable<ReturnType<typeof usePanels>['data']>['panels'] | undefined
  reading: ReaderContextValue['reading']
  /** 显示开发者工具（检查器等）：开发者选项开启，或作品来自开发目录 */
  devTools: boolean
}

function ReaderView({ saveId, presentation, timeline, panels, reading, devTools }: ViewProps) {
  const { ui, save } = presentation
  const actions = useTurnActions(saveId, ui.commands)
  const busy = useReader(selectBusy)
  const connection = useReader((s) => s.connection)
  const dockRef = useRef<InputDockHandle>(null)
  const [helpOpen, setHelpOpen] = useState(false)
  const sidebarOpen = useUI((s) => s.sidebarOpen)
  const setSidebarOpen = useUI((s) => s.setSidebarOpen)
  const [sheetOpen, setSheetOpen] = useState(false)

  const markupCtx = useMarkupContext(ui.builtin_tags, ui.tags, ui.characters, ui.assets)
  const readingStyle = useReadingStyle(reading)
  useWorkStyles(workTagCSS(ui.tags))

  // 还没有任何回合（生成式开场或作品没有开场）：自动开始故事
  const openedRef = useRef(false)
  useEffect(() => {
    if (presentation.needs_opening && !presentation.running_turn_id && !openedRef.current) {
      openedRef.current = true
      void actions.submit({ kind: 'opening' })
    }
  }, [presentation.needs_opening, presentation.running_turn_id, actions])

  // 快捷键：Esc 停止；/ 聚焦输入栏并打开命令面板
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const inField = (e.target as HTMLElement).closest('input, textarea, [contenteditable="true"]')
      if (e.key === 'Escape' && busy) void actions.stop()
      if (e.key === '/' && !inField) {
        e.preventDefault()
        dockRef.current?.focus('/')
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, actions])

  const runCommand = useCallback((text: string) => void actions.submitText(text), [actions])
  const ctx = useMemo<ReaderContextValue>(
    () => ({
      saveId,
      ui,
      reading,
      choose: (item) => void actions.choose(item),
      focusInput: () => dockRef.current?.focus(),
      runCommand,
    }),
    [saveId, ui, reading, actions, runCommand],
  )

  // 舞台背景：进度线上（含进行中的回合）最近一次 <scene bg>
  const draft = useReader((s) => s.draft)
  const sceneBg = useMemo(
    () => lastSceneBackground([...(timeline?.nodes.map((n) => n.display) ?? []), draft?.items ?? []]),
    [timeline, draft?.items],
  )
  const stageAsset = ui.builtin_tags.includes('scene') ? ui.assets.find((a) => a.id === sceneBg && a.kind === 'image') : undefined

  const hasSidebar = hasSidebarPanels(ui.panels, panels)
  const toggleSidebar = () => {
    if (window.matchMedia('(min-width: 1200px)').matches) setSidebarOpen(!sidebarOpen)
    else setSheetOpen(true)
  }

  return (
    <ReaderContext.Provider value={ctx}>
      <MarkupProvider value={markupCtx}>
        <div className="work-root flex h-dvh overflow-hidden bg-background" style={readingStyle}>
          <div className="relative flex min-w-0 flex-1 flex-col">
            <StageBackground asset={stageAsset} intensity={reading?.stage_intensity} />
            {connection === 'reconnecting' && (
              <div role="status" className="absolute inset-x-0 top-0 z-20 bg-warn/15 py-1 text-center text-xs text-warn">
                连接已断开，正在重连……
              </div>
            )}
            <ReaderTopBar save={save} onToggleSidebar={toggleSidebar} sidebarAvailable={hasSidebar} devTools={devTools} />
            <TopbarPanels meta={ui.panels} rendered={panels} />
            <Stream timeline={timeline} onRetry={() => void actions.retry()} onContinue={() => void actions.submit({ kind: 'continue' })} />
            <FloatPanels meta={ui.panels} rendered={panels} />
            <InputDock
              ref={dockRef}
              busy={busy}
              defaultPace={save.settings.pace || ui.pace}
              onSubmit={actions.submitText}
              onStop={() => void actions.stop()}
              onHelp={() => setHelpOpen(true)}
            />
          </div>

          {hasSidebar && sidebarOpen && (
            <aside
              className="hidden w-[300px] shrink-0 overflow-y-auto border-l border-rule px-6 py-6 min-[1200px]:block"
              aria-label="面板"
            >
              <SidebarPanels meta={ui.panels} rendered={panels} />
            </aside>
          )}
        </div>

        <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
          <SheetContent side="right" className="w-[320px] overflow-y-auto bg-background px-6">
            <SheetHeader className="px-0">
              <SheetTitle className="title-book">{save.work_title}</SheetTitle>
            </SheetHeader>
            <SidebarPanels meta={ui.panels} rendered={panels} />
          </SheetContent>
        </Sheet>

        <HelpDialog open={helpOpen} onOpenChange={setHelpOpen} />
      </MarkupProvider>
    </ReaderContext.Provider>
  )
}

/** 注入作品样式标签的 CSS，离开阅读页时移除。 */
function useWorkStyles(css: string) {
  useEffect(() => {
    const el = document.createElement('style')
    el.dataset.work = ''
    el.textContent = css
    document.head.appendChild(el)
    return () => el.remove()
  }, [css])
}
