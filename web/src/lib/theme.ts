/**
 * 主题（纸页 / 夜读 / 跟随系统）与动画偏好。
 *
 * 偏好的权威来源是后端设置（同一台机器上不同浏览器一致）；这里另在 localStorage 缓存一份，
 * 让页面在设置加载完成前就以正确的主题渲染（CSP 禁止内联脚本，无法在 <head> 中提前设置）。
 */
import { create } from 'zustand'

import type { ReadingSettings, ThemePreference } from '@/api/types'

export type ResolvedTheme = 'light' | 'dark'

const STORAGE_KEY = 'yanyi.theme'
const media = typeof window !== 'undefined' ? window.matchMedia('(prefers-color-scheme: dark)') : undefined

function resolve(pref: ThemePreference): ResolvedTheme {
  if (pref === 'system') return media?.matches ? 'dark' : 'light'
  return pref
}

function apply(resolved: ResolvedTheme) {
  document.documentElement.classList.toggle('dark', resolved === 'dark')
}

interface ThemeState {
  preference: ThemePreference
  resolved: ResolvedTheme
  setPreference: (p: ThemePreference) => void
}

export const useTheme = create<ThemeState>((set) => ({
  preference: 'system',
  resolved: 'light',
  setPreference: (preference) => {
    const resolved = resolve(preference)
    localStorage.setItem(STORAGE_KEY, preference)
    apply(resolved)
    set({ preference, resolved })
  },
}))

/** 在首次渲染前调用：按缓存的偏好应用主题，并跟随系统主题变化。 */
export function initTheme() {
  const cached = (localStorage.getItem(STORAGE_KEY) as ThemePreference | null) ?? 'system'
  useTheme.getState().setPreference(cached)
  media?.addEventListener('change', () => {
    const { preference, setPreference } = useTheme.getState()
    if (preference === 'system') setPreference('system')
  })
}

/** 应用"减少动画"偏好：system 跟随系统（CSS 媒体查询），on/off 强制。 */
export function applyMotion(pref: ReadingSettings['reduce_motion']) {
  const root = document.documentElement
  if (pref === 'on') root.dataset.motion = 'reduced'
  else if (pref === 'off') root.dataset.motion = 'full'
  else delete root.dataset.motion
}
