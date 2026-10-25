/**
 * 阅读页会话：订阅存档事件流，把事件分发给 reader store 与查询缓存。
 *
 * 数据流：
 *   SSE ─┬─ turn.* ──────────► useReader.dispatch（草稿回合）
 *        ├─ turn.committed ──► 刷新时间线、存档信息、累计用量、书架"继续阅读"
 *        ├─ panels ──────────► 直接写入面板缓存（不额外请求）
 *        ├─ timeline.changed、compaction.done ─► 刷新时间线、累计用量、存档信息、记忆、分支
 *        ├─ compaction.failed ─► 有说明时提示
 *        └─ session.snapshot ─► 恢复完整会话状态并刷新已提交数据
 */
import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { toast } from 'sonner'

import { qk } from '@/api/queries'
import { openEventStream } from '@/api/sse'
import { useReader } from '@/stores/reader'

export function useReaderSession(saveId: string) {
  const qc = useQueryClient()
  const resyncVersion = useReader((s) => s.resyncVersion)

  useEffect(() => {
    const { reset, dispatch, setConnection } = useReader.getState()
    reset(saveId)
    let initialized = false
    return openEventStream(`/saves/${saveId}/events`, {
      onStatus: (status) => setConnection(status === 'open' && !initialized ? 'connecting' : status),
      onResync: () => {
        initialized = false
        setConnection('reconnecting')
      },
      onEvent: (ev) => {
        dispatch(ev)
        switch (ev.type) {
          case 'session.snapshot':
            initialized = true
            setConnection('open')
            void qc.invalidateQueries({ queryKey: qk.save(saveId) })
            void qc.invalidateQueries({ queryKey: qk.recentSaves })
            break
          case 'turn.committed':
            void qc.invalidateQueries({ queryKey: qk.timeline(saveId) })
            void qc.invalidateQueries({ queryKey: qk.save(saveId) })
            void qc.invalidateQueries({ queryKey: qk.usage(saveId) })
            void qc.invalidateQueries({ queryKey: qk.recentSaves })
            void qc.invalidateQueries({ queryKey: qk.memory(saveId) })
            void qc.invalidateQueries({ queryKey: qk.tree(saveId) })
            break
          case 'turn.failed':
            void qc.invalidateQueries({ queryKey: qk.save(saveId) })
            break
          case 'panels':
            qc.setQueryData(qk.panels(saveId), { panels: ev.data.panels })
            break
          case 'timeline.changed':
          case 'compaction.done':
            void qc.invalidateQueries({ queryKey: qk.timeline(saveId) })
            void qc.invalidateQueries({ queryKey: qk.usage(saveId) })
            void qc.invalidateQueries({ queryKey: qk.save(saveId) })
            void qc.invalidateQueries({ queryKey: qk.memory(saveId) })
            void qc.invalidateQueries({ queryKey: qk.tree(saveId) })
            void qc.invalidateQueries({ queryKey: qk.recentSaves })
            break
          case 'compaction.failed':
            if (ev.data.message) toast(ev.data.message)
            break
        }
      },
    })
  }, [saveId, qc, resyncVersion])
}
