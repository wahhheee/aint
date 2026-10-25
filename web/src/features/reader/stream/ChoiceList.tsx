import { ChevronRight, PenLine } from 'lucide-react'
import { motion, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { DisplayItem } from '@/api/types'
import { cn } from '@/lib/utils'

import { useReaderContext } from '../ReaderContext'

type ChoicesItem = Extract<DisplayItem, { t: 'choices' }>

const LETTERS = 'ABCDEF'

/**
 * 选项（《04-交互与界面设计》8.7）：作为正文的延续出现，逐项淡入上移；数字键 1–6 选择。
 * 选中后所选项保持高亮，其余淡出，随后开始新回合。
 */
export function ChoiceList({ item, selected }: { item: ChoicesItem; selected?: string }) {
  const { t } = useTranslation()
  const { choose, focusInput, ui, reading } = useReaderContext()
  const systemReduce = useReducedMotion()
  const reduce = reading?.reduce_motion === 'on' || (reading?.reduce_motion !== 'off' && systemReduce)
  const [expired, setExpired] = useState(false)
  const locked = selected !== undefined || expired
  const [remaining, setRemaining] = useState(item.timeout_seconds ?? 0)
  const chosen = useRef(false)
  const paused = useRef(false)
  const timed = !!item.timeout_seconds && reading?.timed_choices !== false
  useEffect(() => {
    if (!timed || locked) return
    let last = performance.now()
    const timer = window.setInterval(() => {
      const now = performance.now()
      const elapsed = (now - last) / 1000
      last = now
      if (
        document.hidden ||
        paused.current ||
        document.querySelector('[role="dialog"], [data-slot="popover-content"]') ||
        document.activeElement?.closest('input, textarea, [contenteditable="true"]')
      )
        return
      setRemaining((r) => Math.max(0, r - elapsed))
    }, 100)
    return () => window.clearInterval(timer)
  }, [timed, locked])
  useEffect(() => {
    if (!timed || remaining > 0 || locked || chosen.current) return
    const choice = item.items.find((c) => c.id === item.default_choice)
    if (choice) {
      const timer = window.setTimeout(() => {
        chosen.current = true
        setExpired(true)
        void choose(choice).then((accepted) => {
          if (!accepted) setExpired(false)
        })
      }, 0)
      return () => window.clearTimeout(timer)
    }
  }, [timed, remaining, locked, choose, item])

  // 键盘：1–6 选择对应项（焦点不在输入框时）
  useEffect(() => {
    if (locked) return
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement
      if (
        e.defaultPrevented ||
        el.closest('input, textarea, [contenteditable="true"], [role="dialog"]') ||
        e.ctrlKey ||
        e.metaKey ||
        e.altKey
      )
        return
      const n = Number(e.key)
      if (n >= 1 && n <= item.items.length) {
        e.preventDefault()
        choose(item.items[n - 1])
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [item.items, choose, locked])

  return (
    <div
      onFocusCapture={() => {
        paused.current = true
      }}
      onBlurCapture={() => {
        paused.current = false
      }}
      className="mt-8 mb-2 font-serif"
      aria-label={t('reader.choiceCount', { count: item.items.length })}
    >
      {timed && (
        <div className="mb-3">
          <div className="mb-1 flex justify-between font-sans text-xs text-ink-faint">
            <span>{t('reader.timedHint')}</span>
            <span>{Math.ceil(remaining)} s</span>
          </div>
          <progress
            aria-label={t('reader.timeRemaining')}
            className="h-1 w-full accent-[var(--gold)]"
            max={item.timeout_seconds}
            value={remaining}
          />
        </div>
      )}
      {item.prompt && <p className="mb-3 text-[0.92em] text-ink-muted">{item.prompt}</p>}
      <ol className="overflow-hidden rounded-xl border border-rule bg-paper-raised/75 backdrop-blur-md">
        {item.items.map((c, i) => {
          const isSelected = selected === c.id
          return (
            <motion.li
              key={c.id}
              initial={reduce ? false : { opacity: 0, y: 6 }}
              animate={{ opacity: locked && !isSelected ? 0.35 : 1, y: 0 }}
              transition={{ duration: 0.18, delay: reduce ? 0 : i * 0.06, ease: 'easeOut' }}
              className="border-b border-rule last:border-b-0"
            >
              <button
                type="button"
                disabled={locked}
                onClick={() => choose(c)}
                className={cn(
                  'group flex w-full items-baseline gap-4 px-5 py-3.5 text-left transition-[background-color,box-shadow] duration-150',
                  'hover:bg-hover hover:shadow-[inset_2px_0_0_var(--gold)] focus-visible:bg-hover focus-visible:shadow-[inset_2px_0_0_var(--gold)] focus-visible:outline-none',
                  isSelected && 'bg-selected shadow-[inset_2px_0_0_var(--gold)]',
                  c.style && `mk-${c.style}`,
                )}
              >
                <span className="w-4 shrink-0 text-[0.85em] font-semibold text-gold-text tabular" aria-hidden>
                  {LETTERS[i]}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block text-[0.95em] leading-relaxed text-ink">{c.label}</span>
                  {c.detail && <span className="mt-0.5 block font-sans text-[12.5px] text-ink-muted">{c.detail}</span>}
                </span>
                {c.command && (
                  <ChevronRight className="size-4 shrink-0 self-center text-ink-faint" aria-label={t('reader.executeCommand')} />
                )}
                <kbd className="flex size-5 shrink-0 items-center justify-center self-center rounded border border-rule font-sans text-[11px] text-ink-faint opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
                  {i + 1}
                </kbd>
              </button>
            </motion.li>
          )
        })}
        {item.free && ui.features.free_input && !locked && (
          <li>
            <button
              type="button"
              onClick={focusInput}
              className="flex w-full items-center gap-4 px-5 py-3 text-left font-sans text-[13px] text-ink-faint transition-colors hover:bg-hover hover:text-ink-muted"
            >
              <PenLine className="size-3.5" />
              {t('reader.freeEntry')}
            </button>
          </li>
        )}
      </ol>
    </div>
  )
}

/** 历史回合中的选项：折叠为一行，可展开查看当时的全部选项。 */
export function PastChoices({ item }: { item: ChoicesItem }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <div className="mt-4 mb-2 font-sans text-xs text-ink-faint">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="inline-flex items-center gap-1 hover:text-ink-muted"
      >
        <ChevronRight className={cn('size-3 transition-transform', open && 'rotate-90')} />
        {t('reader.pastChoices')}（{item.items.length}）
      </button>
      {open && (
        <ol className="mt-2 space-y-1 border-l border-rule pl-4">
          {item.items.map((c, i) => (
            <li key={c.id}>
              <span className="mr-2 tabular">{LETTERS[i]}</span>
              {c.label}
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
