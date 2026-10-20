/**
 * 回合操作：提交输入、选择选项、停止。所有提交都经过这里（统一的等待态与错误提示）。
 */
import { useCallback, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage, isApiError } from '@/api/errors'
import type { ChoiceItem, Command, TurnInput } from '@/api/types'
import { useReader } from '@/stores/reader'

import { parseInput } from './commands'

export function useTurnActions(saveId: string, commands: Command[]) {
  const { t } = useTranslation()
  const lastInput = useRef<TurnInput | null>(null)

  const submit = useCallback(
    async (input: TurnInput) => {
      const { setPending, pending, draft } = useReader.getState()
      if (pending || draft?.status === 'streaming') return false
      lastInput.current = input
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

  /** 提交输入栏文字（含斜杠命令）；返回 true 表示已提交，可以清空输入框。 */
  const submitText = useCallback(
    async (text: string, mode?: string, pace?: string) => {
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
        toast(t('common.notImplemented')) // TODO(M3-6)：/compact → POST /saves/{id}/compact
        return true
      }
      return submit(pace ? { ...r.input, pace } : r.input)
    },
    [commands, submit, t],
  )

  const choose = useCallback(
    (item: ChoiceItem) => {
      if (item.command) {
        const r = parseInput(item.command, commands)
        if (r.ok && 'input' in r) return submit(r.input)
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

  const retry = useCallback(() => (lastInput.current ? submit(lastInput.current) : Promise.resolve(false)), [submit])

  // 返回稳定的对象：阅读页的副作用（快捷键、自动开场）依赖它，不应每次渲染都重新订阅
  return useMemo(() => ({ submit, submitText, choose, stop, retry }), [submit, submitText, choose, stop, retry])
}
