/**
 * 界面偏好（只影响本机浏览器的布局状态，不需要同步到后端）。
 */
import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface UIState {
  /** 阅读页右侧面板栏是否展开（桌面） */
  sidebarOpen: boolean
  setSidebarOpen: (open: boolean) => void
  /** 阅读页回合目录是否展开（桌面） */
  turnIndexOpen: boolean
  setTurnIndexOpen: (open: boolean) => void
}

export const useUI = create<UIState>()(
  persist(
    (set) => ({
      sidebarOpen: true,
      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
      turnIndexOpen: true,
      setTurnIndexOpen: (turnIndexOpen) => set({ turnIndexOpen }),
    }),
    { name: 'aint.ui' },
  ),
)
