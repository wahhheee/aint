/**
 * 应用内确认：作品命令与面板按钮的 confirm（《03-作品格式规范》9.1、11.2）。
 * 用法：`if (!(await confirmAction('确定要离开吗？'))) return`；确认框由根布局中的 ConfirmHost 渲染。
 */
import { create } from 'zustand'

interface ConfirmRequest {
  message: string
  resolve: (ok: boolean) => void
}

export const useConfirmStore = create<{ req: ConfirmRequest | null }>(() => ({ req: null }))

/** 显示确认框，玩家确定时返回 true。同一时刻只有一个确认框：新的请求会让旧的按"取消"结束。 */
export function confirmAction(message: string): Promise<boolean> {
  return new Promise((resolve) => {
    useConfirmStore.getState().req?.resolve(false)
    useConfirmStore.setState({ req: { message, resolve } })
  })
}

/** 结束当前的确认请求。 */
export function settleConfirm(ok: boolean) {
  const req = useConfirmStore.getState().req
  useConfirmStore.setState({ req: null })
  req?.resolve(ok)
}
