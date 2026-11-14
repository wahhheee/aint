import { useEffect, useRef, useState, type ReactNode } from 'react'
import { toast } from 'sonner'

import { errorMessage } from '@/api/errors'
import { useDebugActions } from '@/api/queries'

/** 每次进入存档时应用自动记录偏好，完成后才允许开场与玩家操作发起调用。 */
export function DebugCaptureGate({ saveId, autoCapture, children }: { saveId: string; autoCapture: boolean; children: ReactNode }) {
  const [captureOnEntry] = useState(autoCapture)
  const [ready, setReady] = useState(!captureOnEntry)
  const started = useRef(false)
  const { toggle } = useDebugActions(saveId)
  const { mutate } = toggle

  useEffect(() => {
    if (!captureOnEntry || started.current) return
    started.current = true
    mutate(true, {
      onError: (err) => toast.error(errorMessage(err)),
      onSettled: () => setReady(true),
    })
  }, [saveId, captureOnEntry, mutate])

  return ready ? children : <div className="h-dvh" aria-busy />
}
