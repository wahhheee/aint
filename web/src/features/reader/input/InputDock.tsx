/**
 * 输入栏（《04-交互与界面设计》13）
 *
 *   命令栏、行动输入框与底部工具栏（模型 / 推理、预算、篇幅、附件、发送）。
 *
 *   - Enter 发送、Shift+Enter 换行；输入 / 弹出命令面板；发送中按钮变为停止；
 *   - 📎 或粘贴附加图片（上传完成后才能发送；模型不能看图时提示）；
 *   - 输入草稿按存档保存在 localStorage，切换页面不丢失；
 *   - 自由输入被作品关闭时，只接受命令。
 */
import { ArrowUp, ChevronDown, LoaderCircle, Paperclip, Square, X } from 'lucide-react'
import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef, useState, type ClipboardEvent, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import type { Command, Save } from '@/api/types'
import { WorkIcon } from '@/components/common/WorkIcon'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { useReader } from '@/stores/reader'

import { useSaveSettings } from '@/features/save/useSaveSettings'

import { ModelSettings } from './ModelSettings'
import { BudgetSettings } from './BudgetSettings'
import { useReaderContext } from '../ReaderContext'
import { CommandPalette } from './CommandPalette'
import { paletteEntries, type PaletteEntry } from './paletteEntries'
import { ACCEPT_ATTR, useAttachments } from './useAttachments'

export interface InputDockHandle {
  focus: (text?: string) => void
}

/** 提交输入栏内容时的附加信息。 */
export interface SubmitOptions {
  mode?: string
  pace?: string
  attachments?: string[]
  /** 调用方已请玩家确认过（命令的 confirm） */
  confirmed?: boolean
}

interface Props {
  busy: boolean
  save: Save
  onSubmit: (text: string, opts?: SubmitOptions) => Promise<boolean | 'help'>
  onStop: () => void
  onHelp: () => void
}

const PACES = ['short', 'medium', 'long'] as const

export const InputDock = forwardRef<InputDockHandle, Props>(function InputDock({ busy, save, onSubmit, onStop, onHelp }, ref) {
  const { t } = useTranslation()
  const { saveId, ui, vision } = useReaderContext()
  const connected = useReader((s) => s.connection === 'open')
  const attachments = useAttachments()
  const fileRef = useRef<HTMLInputElement>(null)
  const storageKey = `yanyi.draft.${saveId}`
  const [text, setText] = useState(() => localStorage.getItem(storageKey) ?? '')
  const [mode, setMode] = useState<Command | null>(null)
  const pace = save.settings.pace || ui.pace
  const { update: updateSettings, disabled: settingsDisabled } = useSaveSettings(save.id)
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
    fitHeight(taRef.current)
  }, [text, storageKey])

  // 宽度变化（窗口缩放、页面在隐藏状态下完成首次布局）后重新计算高度
  useEffect(() => {
    const ta = taRef.current
    if (!ta || typeof ResizeObserver === 'undefined') return
    let width = ta.clientWidth
    const ro = new ResizeObserver(() => {
      if (ta.clientWidth !== width) {
        width = ta.clientWidth
        fitHeight(ta)
      }
    })
    ro.observe(ta)
    return () => ro.disconnect()
  }, [])

  useEffect(() => setActive(0), [query])

  const pick = (e: PaletteEntry) => {
    setText(`/${e.name} `)
    taRef.current?.focus()
  }

  const canSend = connected && (!!text.trim() || attachments.hashes.length > 0) && !attachments.uploading

  const send = async () => {
    const value = text.trim()
    if (!canSend || busy || updateSettings.isPending) return
    if (!ui.features.free_input && !value.startsWith('/')) {
      toast(t('reader.placeholderChoicesOnly'))
      return
    }
    const res = await onSubmit(value, {
      mode: mode?.name,
      attachments: attachments.hashes.length > 0 ? attachments.hashes : undefined,
    })
    if (res === 'help') onHelp()
    if (res) {
      setText('')
      if (res !== 'help') attachments.clear()
    }
  }

  // 粘贴图片即附加（文字照常粘贴）
  const onPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = [...e.clipboardData.files].filter((f) => f.type.startsWith('image/'))
    if (files.length === 0) return
    e.preventDefault()
    attachments.add(files)
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
    // 宽度与正文列一致（--reading-width 以正文字号为单位换算，所以这里也用正文字号）
    <div
      className="relative z-10 mx-auto w-full max-w-[calc(var(--reading-width)+3rem)] px-4 pb-4 sm:px-6 sm:pb-5"
      style={{ fontSize: 'var(--reading-size)' }}
    >
      {commandBar.length > 0 && (
        <div className="mb-2.5 flex flex-wrap gap-1.5">
          {commandBar.map((c) => (
            <Button
              key={c.name}
              variant="outline"
              size="xs"
              className="rounded-full border-rule bg-paper-raised/70 text-ink-muted backdrop-blur hover:text-ink"
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

      {attachments.items.length > 0 && (
        <div className="mb-2 flex flex-wrap items-end gap-2 font-sans">
          {attachments.items.map((a) => (
            <span key={a.id} className="group relative size-14 overflow-hidden rounded-lg border border-rule-strong bg-paper-raised">
              <img src={a.preview} alt="" className={cn('size-full object-cover', a.status !== 'done' && 'opacity-50')} />
              {a.status === 'uploading' && (
                <LoaderCircle className="absolute inset-0 m-auto size-4 animate-spin text-ink" aria-label={t('common.loading')} />
              )}
              <button
                type="button"
                onClick={() => attachments.remove(a.id)}
                aria-label={t('reader.attachRemove')}
                className="absolute top-0.5 right-0.5 flex size-5 items-center justify-center rounded-full bg-black/60 text-white opacity-80 hover:opacity-100"
              >
                <X className="size-3" />
              </button>
            </span>
          ))}
          {!vision && <span className="pb-1 text-xs text-ink-faint">{t('reader.attachNoVision')}</span>}
        </div>
      )}

      <div
        className={cn(
          'rounded-2xl border border-rule-strong bg-paper-overlay/92 p-2 text-sm shadow-overlay backdrop-blur-xl transition-[border-color]',
          'focus-within:border-ring/60',
        )}
      >
        <div className="flex items-end gap-1">
          {modes.length > 0 && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm" className={cn('shrink-0 gap-1 text-ink', mode?.mode_style && `mk-${mode.mode_style}`)}>
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
            onPaste={onPaste}
            placeholder={placeholder}
            aria-label={placeholder}
            className="min-h-8 min-w-0 flex-1 resize-none bg-transparent px-2 py-[3px] font-serif text-[15px] leading-[26px] text-ink outline-none placeholder:font-sans placeholder:text-sm placeholder:text-ink-faint"
          />
        </div>
        <div className="mt-1 flex min-w-0 items-center gap-0.5" role="group" aria-label={t('playSettings.toolbar')}>
          <ModelSettings save={save} />
          <div className="flex-1" aria-hidden />
          <BudgetSettings save={save} />
          <DropdownMenu>
            <Tooltip>
              <TooltipTrigger asChild>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="text-[13px] font-normal"
                    aria-label={t('playSettings.pace')}
                    disabled={settingsDisabled}
                  >
                    {t(`pace.${(PACES as readonly string[]).includes(pace) ? (pace as 'short') : 'medium'}`)}
                  </Button>
                </DropdownMenuTrigger>
              </TooltipTrigger>
              <TooltipContent>{t('playSettings.pace')}</TooltipContent>
            </Tooltip>
            <DropdownMenuContent align="end" side="top">
              {PACES.map((p) => (
                <DropdownMenuItem key={p} onSelect={() => updateSettings.mutate({ pace: p })} className="justify-between gap-6">
                  <span>{t(`pace.${p}`)}</span>
                  <span className="text-xs text-ink-faint">{t(`paceHint.${p}`)}</span>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label={t('reader.attach')} onClick={() => fileRef.current?.click()}>
                <Paperclip />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('reader.attach')}</TooltipContent>
          </Tooltip>
          <input
            ref={fileRef}
            type="file"
            accept={ACCEPT_ATTR}
            multiple
            hidden
            onChange={(e) => {
              if (e.target.files) attachments.add(e.target.files)
              e.target.value = ''
            }}
          />

          {busy ? (
            <Button size="icon-sm" variant="secondary" onClick={onStop} aria-label={t('reader.stop')}>
              <Square className="size-3 fill-current" />
            </Button>
          ) : (
            <Button
              size="icon-sm"
              onClick={() => void send()}
              disabled={!canSend || updateSettings.isPending}
              aria-label={t('reader.send')}
            >
              <ArrowUp />
            </Button>
          )}
        </div>
      </div>
    </div>
  )
})

/** 输入框自适应高度（1–6 行）。 */
function fitHeight(ta: HTMLTextAreaElement | null) {
  if (!ta) return
  ta.style.height = 'auto'
  ta.style.height = `${Math.min(ta.scrollHeight, 6 * 26)}px`
}
