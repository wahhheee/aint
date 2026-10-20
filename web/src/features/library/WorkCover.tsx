/**
 * 作品封面：没有封面图的作品以"线装书"的样子呈现（《04-交互与界面设计》4：用标题生成排版封面）。
 *
 *   ┌───────────────┬┐
 *   │ ┌──┐          ││   ← 右侧装订：一道书脊线 + 四眼订线
 *   │ │题│          ├┤
 *   │ │签│          ││
 *   │ │竖│          ├┤
 *   │ │排│          ││
 *   │ └──┘          ├┤
 *   └───────────────┴┘
 *
 * 封面颜色取自传统染色，按作品 ID 稳定选取：同一部作品在任何地方都是同一种颜色。
 * 这是整个界面唯一的"记忆点"，其余元素保持克制。
 *
 * TODO(M2-10)：作品提供 cover 素材时显示封面图。
 */
import { cn } from '@/lib/utils'

import { coverColor } from './coverColor'

interface Props {
  id: string
  title: string
  className?: string
  /** 开发模式作品：右下角一枚小印"开发" */
  dev?: boolean
}

export function WorkCover({ id, title, className, dev }: Props) {
  // 题签高度固定，标题越长字越小（容器查询单位 cqw 随封面宽度缩放）
  const len = Math.max(2, Array.from(title).length)
  return (
    <div
      className={cn(
        'relative aspect-[5/7] w-full overflow-hidden rounded-[2px_4px_4px_2px] select-none [container-type:inline-size]',
        'shadow-[inset_-1px_0_0_rgb(255_255_255/0.06),0_1px_0_rgb(0_0_0/0.18),0_10px_22px_-14px_rgb(0_0_0/0.55)]',
        className,
      )}
      style={{ backgroundColor: coverColor(id) }}
      aria-label={title}
      role="img"
    >
      {/* 布面纹理：极淡的经纬线 */}
      <div
        className="absolute inset-0 opacity-[0.07] mix-blend-overlay"
        style={{
          backgroundImage:
            'repeating-linear-gradient(0deg, #fff 0 1px, transparent 1px 3px), repeating-linear-gradient(90deg, #000 0 1px, transparent 1px 4px)',
        }}
      />
      {/* 光泽：左上微亮 */}
      <div className="absolute inset-0 bg-[radial-gradient(120%_80%_at_15%_0%,rgb(255_255_255/0.10),transparent_60%)]" />

      {/* 右侧装订线 */}
      <svg className="absolute inset-y-0 right-0 h-full w-[13%]" viewBox="0 0 13 140" preserveAspectRatio="none" aria-hidden>
        <line x1="3" y1="0" x2="3" y2="140" stroke="rgb(0 0 0 / 0.28)" strokeWidth="0.8" />
        <line x1="3.8" y1="0" x2="3.8" y2="140" stroke="rgb(255 255 255 / 0.10)" strokeWidth="0.4" />
        {[16, 52, 88, 124].map((y) => (
          <g key={y}>
            <line x1="3" y1={y} x2="13" y2={y} stroke="rgb(240 236 226 / 0.55)" strokeWidth="1.1" />
            <circle cx="3" cy={y} r="0.9" fill="rgb(0 0 0 / 0.45)" />
          </g>
        ))}
      </svg>

      {/* 题签 */}
      <div className="absolute top-[7%] left-[11%] flex h-[60%] w-[23%] justify-center bg-[#ece6d6] p-[2.2cqw] shadow-[0_1px_2px_rgb(0_0_0/0.25)]">
        <div className="flex h-full w-full justify-center border border-[#23345a]/25 py-[3cqw]">
          <span
            className="font-serif font-semibold text-[#23345a] [text-orientation:upright] [writing-mode:vertical-rl]"
            style={{ fontSize: `min(9.5cqw, ${(70 / len).toFixed(2)}cqw)`, letterSpacing: '0.12em', lineHeight: 1 }}
          >
            {title}
          </span>
        </div>
      </div>

      {dev && (
        <span className="absolute right-[16%] bottom-[5%] rounded-[2px] border border-[#ece6d6]/60 px-[1.6cqw] py-[0.6cqw] font-serif text-[max(9px,5.5cqw)] leading-none text-[#ece6d6]/80">
          开发
        </span>
      )}
    </div>
  )
}
