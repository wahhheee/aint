/**
 * 打字机效果（《04-交互与界面设计》8.5）：后端增量先进缓冲，前端以自适应速率逐字显示——
 * 缓冲越多越快（约 1.5 秒内追上），保证显示不落后网络太多。即时模式下到达即显示。
 *
 * 返回当前应显示的字符数与 revealAll（点击正文或按空格立即显示全部已到达内容）。
 * TODO(M5-1)：<pause ms> 停顿；段落完成后以段为单位淡入。
 */
import { useCallback, useEffect, useRef, useState } from 'react'

import type { ReadingSettings } from '@/api/types'

const BASE_CPS: Record<ReadingSettings['typewriter'], number> = { instant: Infinity, normal: 60, slow: 28 }

export function useTypewriter(total: number, mode: ReadingSettings['typewriter'], key: string | undefined) {
  const [state, setState] = useState({ key, shown: 0 })
  // 换了回合：从头开始（渲染期间根据 key 派生状态，React 推荐的写法）
  if (state.key !== key) setState({ key, shown: 0 })

  const totalRef = useRef(total)
  useEffect(() => {
    totalRef.current = total
  })

  useEffect(() => {
    if (mode === 'instant') return
    let raf = 0
    let last = performance.now()
    const tick = (now: number) => {
      const dt = Math.min(100, now - last) / 1000
      last = now
      setState((s) => {
        const backlog = totalRef.current - s.shown
        if (backlog <= 0) return s // 返回同一对象：不触发重新渲染
        const cps = Math.max(BASE_CPS[mode], backlog / 1.5)
        return { ...s, shown: Math.min(totalRef.current, s.shown + Math.max(1, Math.round(cps * dt))) }
      })
      raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [mode])

  const revealAll = useCallback(() => setState((s) => ({ ...s, shown: totalRef.current })), [])

  const shown = mode === 'instant' ? total : Math.min(state.key === key ? state.shown : 0, total)
  return { shown, done: shown >= total, revealAll }
}
