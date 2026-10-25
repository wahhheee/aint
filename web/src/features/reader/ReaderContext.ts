import { createContext, useContext } from 'react'

import type { ChoiceItem, Presentation, ReadingSettings } from '@/api/types'

import type { TurnActions } from './useTurnActions'

/** 阅读页内共享的作品界面配置与操作，避免层层传参。 */
export interface ReaderContextValue {
  saveId: string
  ui: Presentation['ui']
  /** 主笔模型能看图（附件、图片相关的提示） */
  vision: boolean
  reading?: ReadingSettings
  choose: (item: ChoiceItem) => Promise<boolean>
  focusInput: () => void
  /** 执行命令（面板按钮、命令栏）：传入 "/follow 林舟" 这样的文字；confirmed 表示调用方已请玩家确认过 */
  runCommand: (text: string, confirmed?: boolean) => void
  /** 回合与分支操作（重新生成、修改、回到此处、切换版本……） */
  actions: TurnActions
  /** 打开调试记录，可按回合过滤。 */
  inspect?: (turnId?: string) => void
}

export const ReaderContext = createContext<ReaderContextValue | null>(null)

export function useReaderContext(): ReaderContextValue {
  const ctx = useContext(ReaderContext)
  if (!ctx) throw new Error('useReaderContext 必须在 ReaderContext.Provider 内使用')
  return ctx
}
