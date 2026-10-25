import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'
import { qk } from '@/api/queries'
import type { SaveSettings } from '@/api/types'
import { selectBusy, useReader } from '@/stores/reader'

/** 只提交当前入口修改的字段，刷新后才允许继续操作。 */
export function useSaveSettings(saveId: string, onDone?: () => void) {
  const qc = useQueryClient()
  const busy = useReader(selectBusy)
  const compacting = useReader((s) => s.compacting)
  const update = useMutation({
    mutationFn: (patch: Partial<SaveSettings>) => api.saves.settings(saveId, patch),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: qk.save(saveId) })
      onDone?.()
    },
    onError: (err) => toast.error(errorMessage(err)),
  })
  return { update, disabled: busy || compacting || update.isPending }
}
