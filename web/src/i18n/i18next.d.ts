// 让 t('key') 的键名获得类型检查：写错键名会在编译期报错。
import 'i18next'

import type { Resources } from './zh-CN'

declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation'
    resources: { translation: Resources }
  }
}
