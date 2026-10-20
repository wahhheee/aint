import { createContext, useContext } from 'react'

import type { ChoiceItem, Presentation, ReadingSettings } from '@/api/types'

/** 阅读页内共享的作品界面配置与操作，避免层层传参。 */
export interface ReaderContextValue {
  saveId: string
  ui: Presentation['ui']
  reading?: ReadingSettings
  choose: (item: ChoiceItem) => void
  focusInput: () => void
  /** 执行命令（面板按钮、命令栏）：传入 "/follow 林舟" 这样的文字 */
  runCommand: (text: string) => void
}

export const ReaderContext = createContext<ReaderContextValue | null>(null)

export function useReaderContext(): ReaderContextValue {
  const ctx = useContext(ReaderContext)
  if (!ctx) throw new Error('useReaderContext 必须在 ReaderContext.Provider 内使用')
  return ctx
}
