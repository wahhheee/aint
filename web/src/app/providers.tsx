/**
 * 全局 Provider：数据请求、提示框、浮层提示、主题同步。
 */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'

import { isApiError } from '@/api/errors'
import { useSettings } from '@/api/queries'
import { Toaster } from '@/components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { applyMotion, useTheme } from '@/lib/theme'

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        // 本地服务：失败通常是程序没在运行或参数错误，重试一次就够；4xx 不重试
        retry: (count, err) => count < 1 && !(isApiError(err) && err.status >= 400 && err.status < 500),
        refetchOnWindowFocus: false,
      },
    },
  })
}

export function Providers({ children }: { children: ReactNode }) {
  const [client] = useState(createQueryClient)
  return (
    <QueryClientProvider client={client}>
      <TooltipProvider delayDuration={300}>
        <SettingsSync />
        {children}
        <Toaster position="bottom-center" />
      </TooltipProvider>
    </QueryClientProvider>
  )
}

/** 设置加载或变化后，同步主题与动画偏好（设置的权威来源在后端）。 */
function SettingsSync() {
  const { data } = useSettings()
  const setPreference = useTheme((s) => s.setPreference)
  useEffect(() => {
    if (!data) return
    setPreference(data.theme)
    applyMotion(data.reading.reduce_motion)
  }, [data, setPreference])
  return null
}
