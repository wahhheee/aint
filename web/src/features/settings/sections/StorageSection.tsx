import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useCollectStorage, useSystemInfo } from '@/api/queries'
import { errorMessage } from '@/api/errors'
import { formatBytes } from '@/lib/format'
import { Button } from '@/components/ui/button'

import { SectionTitle, SettingGroup, SettingRow } from './SettingRow'

/** 数据目录与未使用内容的清理。 */
export function StorageSection() {
  const { t } = useTranslation()
  const { data } = useSystemInfo()
  const collect = useCollectStorage()
  if (!data) return null
  const copy = (text: string) => {
    void navigator.clipboard.writeText(text).then(() => toast(t('common.copied')))
  }
  const path = (p: string) => <span className="font-mono break-all">{p}</span>
  return (
    <section>
      <SectionTitle description={t('settings.storage.dataDirHint')}>{t('settings.sections.storage')}</SectionTitle>
      <SettingGroup>
        <SettingRow label={t('settings.storage.dataDir')} hint={path(data.data_dir)}>
          <Button variant="ghost" size="icon-sm" aria-label={t('common.copy')} onClick={() => copy(data.data_dir)}>
            <Copy />
          </Button>
        </SettingRow>
        <SettingRow label={t('settings.storage.logDir')} hint={path(data.log_dir)}>
          <Button variant="ghost" size="icon-sm" aria-label={t('common.copy')} onClick={() => copy(data.log_dir)}>
            <Copy />
          </Button>
        </SettingRow>
      </SettingGroup>
      <SettingGroup>
        <SettingRow label={t('settings.storage.cleanup')} hint={t('distribution.cleanupHint')}>
          <Button
            variant="outline"
            size="sm"
            disabled={collect.isPending}
            onClick={() =>
              collect.mutate(undefined, {
                onSuccess: (result) => toast(t('distribution.cleanupDone', { count: result.blobs, size: formatBytes(result.bytes) })),
                onError: (e) => toast.error(errorMessage(e)),
              })
            }
          >
            {t('settings.storage.cleanupAction')}
          </Button>
        </SettingRow>
      </SettingGroup>
    </section>
  )
}
