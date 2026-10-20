/**
 * 路由表（一处看全）。主导航只有书架与设置；阅读页全屏，不带外壳。
 *
 *   /                      书架
 *   /works/:workId         作品详情（简介、存档、版本、校验）
 *   /play/:saveId          阅读页
 *   /settings/:section?    设置
 */
import { createBrowserRouter } from 'react-router'

import { LibraryPage } from '@/features/library/LibraryPage'
import { ReaderPage } from '@/features/reader/ReaderPage'
import { SettingsPage } from '@/features/settings/SettingsPage'
import { WorkDetailPage } from '@/features/work/WorkDetailPage'

import { NotFound, RouteError } from './RouteError'
import { RootLayout } from './RootLayout'
import { ShellLayout } from './ShellLayout'

export const router = createBrowserRouter([
  {
    element: <RootLayout />,
    errorElement: <RouteError />,
    children: [
      {
        element: <ShellLayout />,
        children: [
          { index: true, element: <LibraryPage /> },
          { path: 'works/:workId', element: <WorkDetailPage /> },
          { path: 'settings/:section?', element: <SettingsPage /> },
          { path: '*', element: <NotFound /> },
        ],
      },
      { path: 'play/:saveId', element: <ReaderPage /> },
    ],
  },
])
