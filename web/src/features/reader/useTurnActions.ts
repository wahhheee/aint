/**
 * 回合操作：提交输入、选择选项、停止，以及分支操作（重新生成、修改、回到此处、切换版本、丢弃）与手动整理。
 * 所有提交都经过这里（统一的等待态与错误提示）。
 */
import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage, isApiError } from '@/api/errors'
import type { Presentation } from '@/api/types'
import { qk } from '@/api/queries'
import type { ChoiceItem, Command, TurnInput } from '@/api/types'
import { confirmAction } from '@/components/common/confirm'
import { useReader } from '@/stores/reader'

import { parseInput } from './commands'
import type { SubmitOptions } from './input/InputDock'

export function useTurnActions(saveId: string, commands: Command[]) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const lastInput = useRef<TurnInput | null>(null)
  const lastRegeneration = useRef<{ nodeId: string; parentId: string; input?: TurnInput; shown?: TurnInput } | null>(null)

  const refresh = useCallback(() => {
    void qc.invalidateQueries({ queryKey: qk.timeline(saveId) })
    void qc.invalidateQueries({ queryKey: qk.save(saveId) })
    void qc.invalidateQueries({ queryKey: qk.memory(saveId) })
    void qc.invalidateQueries({ queryKey: qk.tree(saveId) })
  }, [qc, saveId])

  const busy = () => {
    const { pending, draft } = useReader.getState()
    return pending !== null || draft?.status === 'streaming'
  }

  const submit = useCallback(
    async (input: TurnInput) => {
      const { setPending } = useReader.getState()
      if (busy()) return false
      lastInput.current = input
      lastRegeneration.current = null
      setPending(input)
      try {
        await api.saves.submit(saveId, input)
        return true
      } catch (err) {
        setPending(null)
        // 409：已有回合在进行（例如另一个标签页提交了），等待它的事件即可
        if (!isApiError(err, 'conflict')) toast.error(errorMessage(err))
        return false
      }
    },
    [saveId],
  )

  /** 手动整理上下文（/compact 与用量环中的按钮）。 */
  const compact = useCallback(async () => {
    try {
      await api.saves.compact(saveId)
      toast(t('reader.compactStarted'))
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }, [saveId, t])

  /**
   * 提交输入栏文字（含斜杠命令）与附件；返回 true 表示已提交，可以清空输入框。
   * 作品命令带 confirm 时先确认（输入模式除外：玩家已主动选择了这个模式）；confirmed 表示调用方已经确认过。
   */
  const submitText = useCallback(
    async (text: string, { mode, pace, attachments, confirmed = false }: SubmitOptions = {}) => {
      const r = parseInput(text, commands, mode)
      if (!r.ok) {
        toast.error(
          r.error === 'not_found'
            ? t('reader.commandNotFound', { name: r.name })
            : t('reader.commandArgMissing', { name: r.name, arg: r.arg ?? '' }),
        )
        return false
      }
      if ('local' in r) {
        if (r.local === 'help') return 'help' as const
        await compact()
        return true
      }
      if (!confirmed && !(await confirmCommand(r.input, commands))) return false
      return submit({ ...r.input, ...(pace && { pace }), ...(attachments && { attachments }) })
    },
    [commands, submit, compact, t],
  )

  const choose = useCallback(
    async (item: ChoiceItem) => {
      if (item.command) {
        const r = parseInput(item.command, commands)
        if (r.ok && 'input' in r) {
          if (!(await confirmCommand(r.input, commands))) return false
          return submit({ ...r.input, choice_id: item.id })
        }
      }
      return submit({ kind: 'choice', choice_id: item.id })
    },
    [commands, submit],
  )

  const stop = useCallback(async () => {
    const { draft } = useReader.getState()
    if (!draft) return
    try {
      await api.saves.cancel(saveId, draft.turnId)
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }, [saveId])

  /**
   * 重新生成回合 nodeId（input 非空时为修改玩家输入）。parentId 是它的父节点：等待期间正文流只显示到父节点为止；
   * shown 是等待期间显示的输入行（重新生成时沿用原回合的输入，见 displayedInput）。
   */
  const regenerate = useCallback(
    async (nodeId: string, parentId: string, input?: TurnInput, shown?: TurnInput) => {
      const { setPending } = useReader.getState()
      if (busy()) return false
      lastRegeneration.current = { nodeId, parentId, input, shown }
      const presentation = qc.getQueryData<Presentation>(qk.save(saveId))
      if (presentation && presentation.save.branch_count <= 1 && !sessionStorage.getItem('yanyi.branchHint')) {
        toast(t('reader.firstBranchHint'), { duration: 8000 })
        sessionStorage.setItem('yanyi.branchHint', '1')
      }
      setPending(input ?? shown ?? { kind: 'continue' }, parentId)
      try {
        await api.saves.regenerate(saveId, nodeId, input)
        return true
      } catch (err) {
        setPending(null)
        toast.error(errorMessage(err))
        return false
      }
    },
    [saveId, qc, t],
  )

  const retry = useCallback(() => {
    const { failure, draft } = useReader.getState()
    if (failure?.nodeId && failure.parentId) {
      return regenerate(failure.nodeId, failure.parentId, undefined, draft?.input ?? lastInput.current ?? undefined)
    }
    const previous = lastRegeneration.current
    if (previous) return regenerate(previous.nodeId, previous.parentId, previous.input, previous.shown)
    return lastInput.current ? submit(lastInput.current) : Promise.resolve(false)
  }, [regenerate, submit])

  /** 不调用模型、直接改存档的操作：成功后刷新时间线（同一存档的其他标签页经 timeline.changed 刷新）。 */
  const direct = useCallback(
    async (op: () => Promise<unknown>) => {
      if (busy()) {
        toast(t('reader.busy'))
        return false
      }
      try {
        await op()
        refresh()
        return true
      } catch (err) {
        toast.error(errorMessage(err))
        return false
      }
    },
    [refresh, t],
  )

  const rewrite = useCallback((nodeId: string, text: string) => direct(() => api.saves.rewrite(saveId, nodeId, text)), [direct, saveId])
  const backHere = useCallback((nodeId: string) => direct(() => api.saves.setHead(saveId, nodeId, 'here')), [direct, saveId])
  /** 切换到节点所在分支的最新进度（切换版本、回到最新进度、在分支视图中切换）。 */
  const switchTo = useCallback((nodeId: string) => direct(() => api.saves.setHead(saveId, nodeId, 'latest-leaf')), [direct, saveId])
  const discard = useCallback(
    async (nodeId: string) => (await confirmAction(t('reader.discardConfirm'))) && direct(() => api.saves.discard(saveId, nodeId)),
    [direct, saveId, t],
  )

  // 返回稳定的对象：阅读页的副作用（快捷键、自动开场）依赖它，不应每次渲染都重新订阅
  return useMemo(
    () => ({ submit, submitText, choose, stop, retry, compact, regenerate, rewrite, backHere, switchTo, discard }),
    [submit, submitText, choose, stop, retry, compact, regenerate, rewrite, backHere, switchTo, discard],
  )
}

export type TurnActions = ReturnType<typeof useTurnActions>

/** 作品命令带 confirm 时请玩家确认；其他输入直接放行。 */
function confirmCommand(input: TurnInput, commands: Command[]): Promise<boolean> {
  if (input.kind !== 'command' || !input.command) return Promise.resolve(true)
  const message = commands.find((c) => c.name === input.command?.name)?.confirm
  return message ? confirmAction(message) : Promise.resolve(true)
}
