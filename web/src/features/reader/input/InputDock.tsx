/**
 * 输入栏（《04-交互与界面设计》13）
 *
 *   [命令栏：作品的 command_bar 按钮]
 *   ┌────────────────────────────────────────────────────────┐
 *   │ [行动 ▾]  写下你的行动……                       中 ▾  📎  ➤ │
 *   └────────────────────────────────────────────────────────┘
 *
 *   - Enter 发送、Shift+Enter 换行；输入 / 弹出命令面板；发送中按钮变为停止；
 *   - 输入草稿按存档保存在 localStorage，切换页面不丢失；
 *   - 自由输入被作品关闭时，只接受命令。
 */
import { ArrowUp, ChevronDown, Paperclip, Square } from 'lucide-react'
import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import type { Command, Pace } from '@/api/types'
import { WorkIcon } from '@/components/common/WorkIcon'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { useReaderContext } from '../ReaderContext'
import { CommandPalette } from './CommandPalette'
import { paletteEntries, type PaletteEntry } from './paletteEntries'

export interface InputDockHandle {
  focus: (text?: string) => void
}

interface Props {
  busy: boolean
  defaultPace: Pace
  onSubmit: (text: string, mode?: string, pace?: string) => Promise<boolean | 'help'>
  onStop: () => void
  onHelp: () => void
}

const PACES = ['short', 'medium', 'long'] as const

export const InputDock = forwardRef<InputDockHandle, Props>(function InputDock({ busy, defaultPace, onSubmit, onStop, onHelp }, ref) {
  const { t } = useTranslation()
  const { saveId, ui } = useReaderContext()
  const storageKey = `yanyi.draft.${saveId}`
  const [text, setText] = useState(() => localStorage.getItem(storageKey) ?? '')
  const [mode, setMode] = useState<Command | null>(null)
  const [pace, setPace] = useState<Pace>(defaultPace)
  const [active, setActive] = useState(0)
  const taRef = useRef<HTMLTextAreaElement>(null)

  const modes = ui.commands.filter((c) => c.present.includes('mode'))
  const commandBar = ui.command_bar.map((n) => ui.commands.find((c) => c.name === n)).filter((c): c is Command => !!c)

  // 斜杠命令面板：输入以 / 开头且还没有输入参数时出现
  const query = /^\/(\S*)$/.exec(text)?.[1]
  const entries = useMemo(() => (query === undefined ? [] : paletteEntries(ui, query)), [ui, query])
  const paletteOpen = entries.length > 0

  useImperativeHandle(ref, () => ({
    focus: (prefill?: string) => {
      if (prefill !== undefined) setText(prefill)
      taRef.current?.focus()
    },
  }))

  useEffect(() => {
    if (text) localStorage.setItem(storageKey, text)
    else localStorage.removeItem(storageKey)
    // 自适应高度（1–6 行）
    const ta = taRef.current
    if (ta) {
      ta.style.height = 'auto'
      ta.style.height = `${Math.min(ta.scrollHeight, 6 * 26)}px`
    }
  }, [text, storageKey])

  useEffect(() => setActive(0), [query])

  const pick = (e: PaletteEntry) => {
    setText(`/${e.name} `)
    taRef.current?.focus()
  }

  const send = async () => {
    const value = text.trim()
    if (!value || busy) return
    if (!ui.features.free_input && !value.startsWith('/')) {
      toast(t('reader.placeholderChoicesOnly'))
      return
    }
    const res = await onSubmit(value, mode?.name, pace !== defaultPace ? pace : undefined)
    if (res === 'help') onHelp()
    if (res) setText('')
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (paletteOpen) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault()
        setActive((i) => (i + (e.key === 'ArrowDown' ? 1 : entries.length - 1)) % entries.length)
        return
      }
      if (e.key === 'Tab' || (e.key === 'Enter' && !e.shiftKey && entries[active].name !== query)) {
        e.preventDefault()
        pick(entries[active])
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        setText('')
        return
      }
    }
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      void send()
    }
  }

  const placeholder = !ui.features.free_input
    ? t('reader.placeholderChoicesOnly')
    : mode
      ? t('reader.placeholderMode', { mode: mode.mode_label || mode.title })
      : t('reader.placeholder')

  return (
    <div className="relative mx-auto w-full max-w-[calc(var(--reading-width)+3rem)] px-4 pb-4">
      {commandBar.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5 px-1">
          {commandBar.map((c) => (
            <Button
              key={c.name}
              variant="ghost"
              size="xs"
              className="text-ink-muted"
              disabled={busy}
              onClick={() => {
                // 需要参数的命令先填入输入框，否则直接执行
                if (c.args?.some((a) => a.required)) {
                  setText(`/${c.name} `)
                  taRef.current?.focus()
                } else void onSubmit(`/${c.name}`)
              }}
            >
              <WorkIcon name={c.icon} />
              {c.title}
            </Button>
          ))}
        </div>
      )}

      {paletteOpen && <CommandPalette entries={entries} active={active} onPick={pick} onHover={setActive} />}

      <div
        className={cn(
          'flex items-end gap-2 rounded-2xl border border-rule bg-paper-raised px-2 py-2 shadow-[0_8px_24px_-16px_rgb(0_0_0/0.35)] transition-colors',
          'focus-within:border-ring/50',
        )}
      >
        {modes.length > 0 && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" className={cn('shrink-0 gap-1', mode?.mode_style && `mk-${mode.mode_style}`)}>
                {mode ? mode.mode_label || mode.title : t('reader.modeAction')}
                <ChevronDown className="size-3.5 opacity-60" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" side="top">
              <DropdownMenuItem onSelect={() => setMode(null)}>{t('reader.modeAction')}</DropdownMenuItem>
              {modes.map((m) => (
                <DropdownMenuItem key={m.name} onSelect={() => setMode(m)} className="flex-col items-start gap-0.5">
                  <span className={cn(m.mode_style && `mk-${m.mode_style}`)}>{m.mode_label || m.title}</span>
                  <span className="max-w-64 text-xs text-ink-faint">{m.description}</span>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}

        <textarea
          ref={taRef}
          rows={1}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
          aria-label={placeholder}
          className="min-h-9 flex-1 resize-none bg-transparent px-2 py-1.5 font-serif text-[15px] leading-[26px] text-ink outline-none placeholder:font-sans placeholder:text-sm placeholder:text-ink-faint"
        />

        <div className="flex shrink-0 items-center gap-0.5">
          <DropdownMenu>
            <Tooltip>
              <TooltipTrigger asChild>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="sm" className="w-9 text-ink-muted" aria-label="本回合篇幅">
                    {t(`pace.${(PACES as readonly string[]).includes(pace) ? (pace as 'short') : 'medium'}`)}
                  </Button>
                </DropdownMenuTrigger>
              </TooltipTrigger>
              <TooltipContent>本回合篇幅</TooltipContent>
            </Tooltip>
            <DropdownMenuContent align="end" side="top">
              {PACES.map((p) => (
                <DropdownMenuItem key={p} onSelect={() => setPace(p)} className="justify-between gap-6">
                  <span>{t(`pace.${p}`)}</span>
                  <span className="text-xs text-ink-faint">{t(`paceHint.${p}`)}</span>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>

          <Tooltip>
            <TooltipTrigger asChild>
              {/* TODO(M2-10)：附加图片（POST /uploads），非视觉模型时提示 */}
              <Button
                variant="ghost"
                size="icon-sm"
                className="text-ink-faint"
                aria-label={t('reader.attach')}
                onClick={() => toast(t('common.notImplemented'))}
              >
                <Paperclip />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('reader.attach')}</TooltipContent>
          </Tooltip>

          {busy ? (
            <Button size="icon-sm" variant="secondary" onClick={onStop} aria-label={t('reader.stop')}>
              <Square className="fill-current" />
            </Button>
          ) : (
            <Button size="icon-sm" onClick={() => void send()} disabled={!text.trim()} aria-label={t('reader.send')}>
              <ArrowUp />
            </Button>
          )}
        </div>
      </div>
    </div>
  )
})
