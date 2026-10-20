/**
 * 轻舞台的背景层（《04-交互与界面设计》8.6）：位于正文之后、铺满阅读区，
 * 默认高斯模糊 + 以纸色覆盖（深色模式下是墨色），保证正文对比度；场景切换时交叉淡化（600ms）。
 *
 * 舞台状态从时间线推导（最近一次 <scene bg>），回看或切换分支时自然恢复到对应位置的舞台。
 * TODO(M5-2)：transition（fade / cut / blur）、BGM 与音效、"减少动画"时直接替换。
 */
import { AnimatePresence, motion } from 'motion/react'

import type { AssetRef } from '@/api/types'

export function StageBackground({ asset, intensity = 0.6 }: { asset?: AssetRef; intensity?: number }) {
  return (
    <div className="pointer-events-none absolute inset-0 overflow-hidden" aria-hidden>
      <AnimatePresence>
        {asset && (
          <motion.img
            key={asset.url}
            src={asset.url}
            alt=""
            initial={{ opacity: 0 }}
            animate={{ opacity: intensity }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.6, ease: 'easeInOut' }}
            className="absolute inset-0 size-full scale-110 object-cover blur-[10px] saturate-[0.85]"
          />
        )}
      </AnimatePresence>
      {/* 纸色覆盖：四周透出背景，正文区域更"实"，保证可读 */}
      <div className="absolute inset-0 bg-[radial-gradient(ellipse_62%_85%_at_50%_45%,color-mix(in_oklch,var(--paper)_90%,transparent)_35%,color-mix(in_oklch,var(--paper)_30%,transparent)_100%)]" />
    </div>
  )
}
