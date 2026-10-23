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
 *   drawers/           记忆、时间线与分支、状态（开发者工具）抽屉
 *
 * TODO(M5-2)：舞台层（背景、转场、BGM）。TODO(M5-13)：退出时 POST /saves/{id}/exit 与 pagehide 信标。
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useParams } from 'react-router'
import { toast } from 'sonner'

import { useDebugActions, useDebugStatus, usePanels, usePresentation, useSettings, useTimeline } from '@/api/queries'
import { errorMessage } from '@/api/errors'
import { LoadError } from '@/components/common/LoadError'
import { MarkupProvider, useMarkupContext } from '@/markup/context'
import { workTagCSS } from '@/markup/workStyles'
import { selectBusy, useReader } from '@/stores/reader'
import { useUI } from '@/stores/ui'

import { HelpDialog } from './chrome/HelpDialog'
import { ReaderTopBar } from './chrome/ReaderTopBar'
import { DebugControl } from './debug/DebugControl'
import { DebugInspector } from './debug/DebugInspector'
import { BranchDrawer } from './drawers/BranchDrawer'
import { MemoryDrawer } from './drawers/MemoryDrawer'
import { StateDrawer } from './drawers/StateDrawer'
import { InputDock, type InputDockHandle } from './input/InputDock'
import { hasPanels } from './panels/layout'
import { FloatPanels, SidebarPanels, TopbarPanels } from './panels/Panels'
import { PanelSheet } from './panels/PanelSheet'
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
      key={saveId}
      saveId={saveId}
      presentation={presentation.data}
      timeline={timeline.data}
      panels={panels.data?.panels}
      reading={settings.data?.reading}
      workTheme={settings.data?.use_work_theme !== false}
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
  /** 使用作品主题（设置中可关闭；样式标签不受影响） */
  workTheme: boolean
  /** 显示状态编辑工具：开发者选项开启，或作品来自开发目录。 */
  devTools: boolean
}

function ReaderView({ saveId, presentation, timeline, panels, reading, workTheme, devTools }: ViewProps) {
  const { ui, save } = presentation
  const actions = useTurnActions(saveId, ui.commands)
  const busy = useReader(selectBusy)
  const connection = useReader((s) => s.connection)
  const dockRef = useRef<InputDockHandle>(null)
  const [helpOpen, setHelpOpen] = useState(false)
  const sidebarOpen = useUI((s) => s.sidebarOpen)
  const setSidebarOpen = useUI((s) => s.setSidebarOpen)
  const [sheetOpen, setSheetOpen] = useState(false)
  const [drawer, setDrawer] = useState<'memory' | 'branches' | 'state' | null>(null)
  const debug = useDebugStatus(saveId)
  const debugActions = useDebugActions(saveId)
  const [debugOpen, setDebugOpen] = useState(false)
  const [debugTurn, setDebugTurn] = useState<string>()
  const debugAvailable = !!debug.data?.enabled || !!debug.data?.calls.length
  const inspect = useCallback((turnId?: string) => {
    setDebugTurn(turnId)
    setDebugOpen(true)
  }, [])
  const toggleDebug = (enabled: boolean) =>
    debugActions.toggle.mutate(enabled, {
      onSuccess: () => {
        if (enabled) inspect()
      },
      onError: (err) => toast.error(errorMessage(err)),
    })

  // 打开存档（或存档信息刷新）时，用呈现信息初始化上下文用量与整理状态；回合中的用量由 turn.usage 更新
  const initReader = useReader((s) => s.init)
  useEffect(() => {
    initReader(presentation.context, presentation.compacting)
  }, [initReader, presentation.context, presentation.compacting])

  const markupCtx = useMarkupContext(ui.builtin_tags, ui.tags, ui.characters, ui.assets)
  const theme = workTheme ? ui.theme : undefined
  const readingStyle = useReadingStyle(reading, theme?.reading)
  useWorkStyles(workTagCSS(ui.tags) + (theme?.css ? '\n' + theme.css : ''))

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
      if (e.defaultPrevented || (e.target as HTMLElement).closest('[role="dialog"]')) return
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

  const runCommand = useCallback((text: string, confirmed?: boolean) => void actions.submitText(text, { confirmed }), [actions])
  const ctx = useMemo<ReaderContextValue>(
    () => ({
      saveId,
      ui,
      vision: presentation.model.vision,
      reading,
      choose: (item) => void actions.choose(item),
      focusInput: () => dockRef.current?.focus(),
      runCommand,
      actions,
      inspect: debugAvailable ? inspect : undefined,
    }),
    [saveId, ui, presentation.model.vision, reading, actions, runCommand, debugAvailable, inspect],
  )

  // 舞台背景：进度线上（含进行中的回合）最近一次 <scene bg>
  const draft = useReader((s) => s.draft)
  const sceneBg = useMemo(
    () => lastSceneBackground([...(timeline?.nodes.map((n) => n.display) ?? []), draft?.items ?? []]),
    [timeline, draft?.items],
  )
  const lastTurnId = useMemo(() => timeline?.nodes.findLast((n) => n.kind === 'turn')?.id, [timeline])
  const stageAsset = ui.builtin_tags.includes('scene') ? ui.assets.find((a) => a.id === sceneBg && a.kind === 'image') : undefined

  // 宽屏：sidebar 面板在右侧栏，drawer 面板在抽屉；窄屏没有右侧栏，两者都在抽屉里
  const hasSidebar = hasPanels(ui.panels, panels, 'sidebar')
  const hasDrawer = hasPanels(ui.panels, panels, 'drawer')
  const wide = () => window.matchMedia('(min-width: 1200px)').matches
  const [sheetWithSidebar, setSheetWithSidebar] = useState(false)
  const togglePanels = () => {
    if (wide() && hasSidebar) setSidebarOpen(!sidebarOpen)
    else {
      setSheetWithSidebar(!wide())
      setSheetOpen(true)
    }
  }
  const openDrawer = () => {
    setSheetWithSidebar(false)
    setSheetOpen(true)
  }

  return (
    <ReaderContext.Provider value={ctx}>
      <MarkupProvider value={markupCtx}>
        <div className="work-root relative isolate flex h-dvh overflow-hidden bg-paper" style={readingStyle}>
          {/* 作品主题的背景图（低透明度的纸纹、雾气等），在舞台背景之下 */}
          {theme?.reading.backdrop && <div aria-hidden className="work-backdrop pointer-events-none absolute inset-0 bg-cover bg-center" />}
          {/* 舞台背景铺满整页（侧栏是半透明的磨砂面，背景从下面透出来） */}
          <StageBackground asset={stageAsset} intensity={reading?.stage_intensity} />

          <div className="relative flex min-w-0 flex-1 flex-col">
            {connection === 'reconnecting' && (
              <div
                role="status"
                className="absolute inset-x-0 top-0 z-20 flex h-7 items-center justify-center gap-2 bg-warn/12 text-xs text-warn backdrop-blur"
              >
                <span aria-hidden className="size-1.5 animate-pulse rounded-full bg-warn" />
                连接已断开，正在重连……
              </div>
            )}
            <ReaderTopBar
              saveId={saveId}
              save={save}
              lastTurnId={lastTurnId}
              onTogglePanels={togglePanels}
              panelsAvailable={hasSidebar || hasDrawer}
              onOpenDrawer={hasSidebar && hasDrawer ? openDrawer : undefined}
              devTools={devTools}
              onOpenMemory={() => setDrawer('memory')}
              onOpenBranches={() => setDrawer('branches')}
              onOpenState={() => setDrawer('state')}
              compactions={presentation.compactions}
              onCompact={() => void actions.compact()}
              onInspect={debugAvailable ? () => inspect() : undefined}
              debugControl={
                <DebugControl
                  enabled={!!debug.data?.enabled}
                  pending={debug.isPending || debugActions.toggle.isPending}
                  available={debugAvailable}
                  onToggle={toggleDebug}
                  onOpen={() => inspect()}
                />
              }
            />
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
              className="scroll-stable relative hidden w-[320px] shrink-0 border-l border-rule bg-paper/75 px-6 pt-[18px] pb-6 backdrop-blur-xl min-[1200px]:block"
              aria-label="面板"
            >
              <SidebarPanels meta={ui.panels} rendered={panels} />
            </aside>
          )}
        </div>

        <PanelSheet
          open={sheetOpen}
          onOpenChange={setSheetOpen}
          meta={ui.panels}
          rendered={panels}
          includeSidebar={sheetWithSidebar}
          title={save.work_title}
        />

        <HelpDialog open={helpOpen} onOpenChange={setHelpOpen} />
        <DebugInspector
          saveId={saveId}
          open={debugOpen}
          onOpenChange={setDebugOpen}
          status={debug.data}
          pending={debugActions.toggle.isPending || debugActions.clear.isPending}
          onToggle={toggleDebug}
          onClear={() => debugActions.clear.mutate(undefined, { onError: (err) => toast.error(errorMessage(err)) })}
          turnId={debugTurn}
          onAllTurns={() => setDebugTurn(undefined)}
        />
        <MemoryDrawer saveId={saveId} open={drawer === 'memory'} onOpenChange={(o) => setDrawer(o ? 'memory' : null)} />
        <BranchDrawer saveId={saveId} open={drawer === 'branches'} onOpenChange={(o) => setDrawer(o ? 'branches' : null)} />
        {devTools && <StateDrawer saveId={saveId} open={drawer === 'state'} onOpenChange={(o) => setDrawer(o ? 'state' : null)} />}
      </MarkupProvider>
    </ReaderContext.Provider>
  )
}

/** 注入作品的 CSS（样式标签与主题），离开阅读页时移除。 */
function useWorkStyles(css: string) {
  useEffect(() => {
    const el = document.createElement('style')
    el.dataset.work = ''
    el.textContent = css
    document.head.appendChild(el)
    return () => el.remove()
  }, [css])
}
