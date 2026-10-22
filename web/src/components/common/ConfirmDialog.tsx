/**
 * 应用内确认框的宿主：渲染 confirmAction（confirm.ts）发出的确认请求。
 * 文案来自作品（不可信），只当作纯文本显示。
 */
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

import { settleConfirm as settle, useConfirmStore } from './confirm'

/** 挂在根布局中，渲染当前的确认请求。 */
export function ConfirmHost() {
  const { t } = useTranslation()
  const req = useConfirmStore((s) => s.req)
  return (
    <AlertDialog open={req !== null} onOpenChange={(open) => !open && settle(false)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('common.confirmTitle')}</AlertDialogTitle>
          <AlertDialogDescription className="whitespace-pre-wrap">{req?.message}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel onClick={() => settle(false)}>{t('common.cancel')}</AlertDialogCancel>
          <AlertDialogAction onClick={() => settle(true)}>{t('common.confirm')}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
