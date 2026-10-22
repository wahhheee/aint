import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { Outlet } from 'react-router'

import { qk } from '@/api/queries'
import { openEventStream } from '@/api/sse'
import { ConfirmHost } from '@/components/common/ConfirmDialog'

/** 根布局：订阅全局事件流（作品重载、存档标题生成），据此刷新相关数据；挂载全局的确认框。 */
export function RootLayout() {
  const qc = useQueryClient()
  useEffect(
    () =>
      openEventStream('/events', {
        onEvent: (ev) => {
          switch (ev.type) {
            case 'work.reloaded':
              void qc.invalidateQueries({ queryKey: qk.work(ev.data.work_id) })
              void qc.invalidateQueries({ queryKey: qk.works })
              break
            case 'save.titled':
              void qc.invalidateQueries({ queryKey: qk.recentSaves })
              void qc.invalidateQueries({ queryKey: qk.save(ev.data.save_id) })
              break
          }
        },
      }),
    [qc],
  )
  return (
    <>
      <Outlet />
      <ConfirmHost />
    </>
  )
}
