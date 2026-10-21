import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useSystemInfo } from '@/api/queries'
import { Button } from '@/components/ui/button'

import { SectionTitle, SettingGroup, SettingRow } from './SettingRow'

/** 存储（《04-交互与界面设计》9.7）。TODO(M6-5)：各部分占用、清理未使用的数据（GC）。 */
export function StorageSection() {
  const { t } = useTranslation()
  const { data } = useSystemInfo()
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
        <SettingRow label={t('settings.storage.cleanup')} hint={t('common.notImplemented')}>
          <Button variant="outline" size="sm" disabled>
            {t('settings.storage.cleanupAction')}
          </Button>
        </SettingRow>
      </SettingGroup>
    </section>
  )
}
