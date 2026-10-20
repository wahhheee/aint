import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import '@/i18n'
import '@/styles/globals.css'

import { App } from '@/app/App'
import { initTheme } from '@/lib/theme'

// 首次渲染前应用缓存的主题，避免闪烁
initTheme()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
