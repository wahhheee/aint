/**
 * 导入流程的状态（《04-交互与界面设计》5：解析阶段只读取与校验，不写入；确认后才导入）：
 *
 *   select ──选择 / 拖入──► reading（读取拖入的文件夹）──► uploading（上传，随后服务端解析校验）
 *      ▲                                                      │
 *      │                                       ┌──────────────┴─────────────┐
 *      └──── 重新选择 ──── failed（不能导入：超限、不是 zip、网络……） preview（报告与摘要）──确认──► done
 *
 * 放弃（关闭对话框、重新选择、卸载）时取消上传，并删除服务端暂存的导入。
 * 书架页与导入对话框共用同一个实例：拖入书架页的内容也从这里开始。
 */
import { useEffect, useEffectEvent, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiError, errorMessage } from '@/api/errors'
import { useImportWork } from '@/api/queries'
import type { ImportPreview, ImportResult } from '@/api/types'
import { formatBytes } from '@/lib/format'

import { buildImportForm, checkInput, IMPORT_LIMITS, inputName, inputSize, problem, type InputProblem, type Picked } from './importSource'

export type ImportStage =
  | { step: 'select' }
  | { step: 'reading' }
  | { step: 'uploading'; name: string; loaded: number; total: number }
  | { step: 'preview'; preview: ImportPreview }
  | { step: 'done'; preview: ImportPreview; result: ImportResult }
  | { step: 'failed'; message: string; requestId?: string }

export interface WorkImport {
  stage: ImportStage
  /** 正在读取、上传或确认：此时不接受新的内容 */
  busy: boolean
  /** 确认导入的请求 */
  confirming: boolean
  confirmError: unknown
  /** 从选择的内容开始导入 */
  start: (picked: Picked) => void
  /** 从异步得到的内容开始导入（拖入的文件夹需要先遍历） */
  startFrom: (pending: Promise<Picked>) => void
  confirm: () => void
  /** 放弃当前导入，回到选择 */
  reset: () => void
}

export function useWorkImport(): WorkImport {
  const { t } = useTranslation()
  const { preview, confirm, cancel } = useImportWork()
  const [stage, setStage] = useState<ImportStage>({ step: 'select' })

  // 每次开始导入递增；异步结果回来时已不是当前这一次（玩家放弃或重新选择了），就丢弃它
  const run = useRef(0)
  const abort = useRef<AbortController | null>(null)
  // 服务端暂存、尚未确认的导入任务：放弃时删除
  const staged = useRef<string | undefined>(undefined)

  const discard = () => {
    run.current++
    abort.current?.abort()
    abort.current = null
    if (staged.current) cancel.mutate(staged.current) // 失败也无妨：服务端 30 分钟后自动清理
    staged.current = undefined
    confirm.reset()
  }

  // 离开书架页时放弃进行中的导入
  const onUnmount = useEffectEvent(discard)
  useEffect(() => () => onUnmount(), [])

  const problemMessage = (p: InputProblem) =>
    t(`import.problems.${p}`, { size: formatBytes(IMPORT_LIMITS.maxTotal), count: IMPORT_LIMITS.maxFiles })

  const upload = async (picked: Picked, id: number) => {
    if (picked.kind === 'problem') {
      setStage({ step: 'failed', message: problemMessage(picked.problem) })
      return
    }
    const tooBig = checkInput(picked)
    if (tooBig) {
      setStage({ step: 'failed', message: problemMessage(tooBig) })
      return
    }
    const name = inputName(picked)
    const ctl = new AbortController()
    abort.current = ctl
    setStage({ step: 'uploading', name, loaded: 0, total: inputSize(picked) })
    try {
      const p = await preview.mutateAsync({
        form: buildImportForm(picked),
        signal: ctl.signal,
        onProgress: (loaded, total) => {
          if (run.current === id) setStage({ step: 'uploading', name, loaded, total })
        },
      })
      if (run.current !== id) {
        if (p.task) cancel.mutate(p.task) // 结果回来之前玩家已放弃
        return
      }
      staged.current = p.task
      setStage({ step: 'preview', preview: p })
    } catch (err) {
      if (run.current !== id || (err instanceof DOMException && err.name === 'AbortError')) return
      setStage({ step: 'failed', message: errorMessage(err), requestId: err instanceof ApiError ? err.requestId : undefined })
    } finally {
      if (abort.current === ctl) abort.current = null
    }
  }

  const start = (picked: Picked) => {
    discard()
    void upload(picked, run.current)
  }

  const startFrom = (pending: Promise<Picked>) => {
    discard()
    const id = run.current
    setStage({ step: 'reading' })
    void pending
      .catch(() => problem('readFailed'))
      .then((picked) => {
        if (run.current === id) void upload(picked, id)
      })
  }

  const doConfirm = () => {
    if (stage.step !== 'preview' || !stage.preview.task) return
    const p = stage.preview
    const id = run.current
    confirm.mutate(stage.preview.task, {
      onSuccess: (result) => {
        staged.current = undefined // 已写入，服务端已删除暂存
        if (run.current === id) setStage({ step: 'done', preview: p, result })
      },
    })
  }

  const reset = () => {
    discard()
    setStage({ step: 'select' })
  }

  return {
    stage,
    busy: stage.step === 'reading' || stage.step === 'uploading' || confirm.isPending,
    confirming: confirm.isPending,
    confirmError: confirm.error,
    start,
    startFrom,
    confirm: doConfirm,
    reset,
  }
}
