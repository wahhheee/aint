/**
 * 国际化初始化。首版只有简体中文；新增语言时添加资源文件并在 resources 中登记。
 */
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import zhCN from './zh-CN'

void i18n.use(initReactI18next).init({
  resources: { 'zh-CN': { translation: zhCN } },
  lng: 'zh-CN',
  fallbackLng: 'zh-CN',
  interpolation: { escapeValue: false }, // React 已负责转义
  returnNull: false,
})

export default i18n
