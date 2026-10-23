/**
 * 阅读页会话：订阅存档事件流，把事件分发给 reader store 与查询缓存。
 *
 * 数据流：
 *   SSE ─┬─ turn.* ──────────► useReader.dispatch（草稿回合）
 *        ├─ turn.committed ──► 刷新时间线、存档信息、累计用量、书架"继续阅读"
 *        ├─ panels ──────────► 直接写入面板缓存（不额外请求）
 *        ├─ timeline.changed、compaction.done ─► 刷新时间线、累计用量、存档信息、记忆、分支
 *        ├─ compaction.failed ─► 有说明时提示
 *        └─ resync ──────────► 重新拉取时间线与面板（断线太久，补发不了）
 */
import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { toast } from 'sonner'

import { qk } from '@/api/queries'
import { openEventStream } from '@/api/sse'
import { useReader } from '@/stores/reader'

export function useReaderSession(saveId: string) {
  const qc = useQueryClient()

  useEffect(() => {
    const { reset, dispatch, setConnection } = useReader.getState()
    reset(saveId)
    return openEventStream(`/saves/${saveId}/events`, {
      onStatus: setConnection,
      onResync: () => {
        void qc.invalidateQueries({ queryKey: qk.timeline(saveId) })
        void qc.invalidateQueries({ queryKey: qk.panels(saveId) })
        void qc.invalidateQueries({ queryKey: qk.save(saveId) })
      },
      onEvent: (ev) => {
        dispatch(ev)
        switch (ev.type) {
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
  }, [saveId, qc])
}
