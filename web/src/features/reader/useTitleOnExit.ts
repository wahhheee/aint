import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'
import { useSettings } from '@/api/queries'

/** 路由离开与 pagehide 都通知后台；延迟一个任务避免 StrictMode 的探测卸载。 */
export function useTitleOnExit(saveId: string) {
  const { data } = useSettings()
  const { t } = useTranslation()
  const mode = data?.play.title_on_exit ?? 'off'
  const light = !!data?.models.light
  useEffect(() => {
    if (!light || mode === 'off') return
    let hidden = false
    const pagehide = () => {
      hidden = true
      if (mode === 'auto') navigator.sendBeacon(`/api/v1/saves/${saveId}/exit`)
    }
    window.addEventListener('pagehide', pagehide)
    return () => {
      window.removeEventListener('pagehide', pagehide)
      setTimeout(() => {
        if (hidden || window.location.pathname === `/play/${saveId}`) return
        if (mode === 'auto') void api.saves.exit(saveId).catch((err) => toast.error(errorMessage(err)))
        else
          toast(t('background.askTitle'), {
            duration: 8000,
            action: {
              label: t('background.generateTitle'),
              onClick: () => {
                void api.saves
                  .title(saveId)
                  .then(() => toast(t('background.titleStarted')))
                  .catch((err) => toast.error(errorMessage(err)))
              },
            },
          })
      }, 0)
    }
  }, [saveId, mode, light, t])
}
