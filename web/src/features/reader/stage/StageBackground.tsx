/**
 * 轻舞台的背景层（《04-交互与界面设计》8.6）：铺满阅读页、位于一切内容之后。
 *
 *   图片：高斯模糊、略降饱和，透明度取阅读设置的舞台强度；场景切换时交叉淡化（600ms）
 *   暗角：版心一带以底色（夜读时是纯黑）覆盖得最实，四周逐渐透出画面——保证正文对比度
 *   上下：顶栏与输入栏所在的两端再压暗一些，让它们"落"在画面上
 *   颗粒：极淡的噪点，消除深色渐变的色带
 *
 * 舞台状态从时间线推导（最近一次 <scene bg>），回看或切换分支时自然恢复到对应位置的舞台。
 * 转场支持淡化、切换与模糊；减少动画时直接替换。
 */
import { AnimatePresence, motion } from 'motion/react'

import type { AssetRef } from '@/api/types'

const GRAIN =
  "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E\")"

export function StageBackground({
  asset,
  intensity = 0.6,
  transition = 'fade',
  reduce = false,
}: {
  asset?: AssetRef
  intensity?: number
  transition?: string
  reduce?: boolean
}) {
  return (
    <div className="pointer-events-none absolute inset-0 -z-10 overflow-hidden" aria-hidden>
      <AnimatePresence>
        {asset && (
          <motion.div
            key={asset.url}
            className="absolute inset-0"
            initial={reduce || transition === 'cut' ? false : { opacity: 0, filter: transition === 'blur' ? 'blur(20px)' : 'blur(0px)' }}
            animate={{ opacity: 1, filter: 'blur(0px)' }}
            exit={{ opacity: 0 }}
            transition={{ duration: reduce || transition === 'cut' ? 0 : 0.6, ease: 'easeInOut' }}
          >
            <img
              src={asset.url}
              alt=""
              className="absolute inset-0 size-full scale-110 object-cover blur-[14px] saturate-[0.85]"
              style={{ opacity: intensity }}
            />
            <div className="absolute inset-0 bg-[radial-gradient(ellipse_58%_80%_at_50%_46%,color-mix(in_oklch,var(--paper)_92%,transparent)_30%,color-mix(in_oklch,var(--paper)_55%,transparent)_70%,color-mix(in_oklch,var(--paper)_25%,transparent)_100%)]" />
            <div className="absolute inset-0 bg-[linear-gradient(to_bottom,color-mix(in_oklch,var(--paper)_70%,transparent),transparent_16%,transparent_72%,color-mix(in_oklch,var(--paper)_85%,transparent))]" />
            <div className="absolute inset-0 opacity-[0.05] mix-blend-overlay" style={{ backgroundImage: GRAIN }} />
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}
