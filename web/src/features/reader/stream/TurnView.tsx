import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import type { DisplayItem } from '@/api/types'
import { ImageViewer } from '@/components/common/ImageViewer'
import { useContext } from 'react'
import { MarkupContext, findAsset } from '@/markup/context'
import { Markup } from '@/markup/render'

import { useReaderContext } from '../ReaderContext'
import { ChoiceList, PastChoices } from './ChoiceList'
import { DiceCard } from './DiceCard'
import { Notice, QuietLine } from './Notice'
import { PlayerLine } from './PlayerLine'

interface Props {
  items: DisplayItem[]
  /** 正文仍在流式生成（最后一段带光标） */
  drafting?: boolean
  /** 选项是否可交互（只有最新回合的选项可选） */
  interactive?: boolean
  /** 已经选择的选项（等待新回合开始期间高亮） */
  selected?: string
  /** 附加在提示条上的操作（如中断回合的"继续"） */
  noticeActions?: ReactNode
}

/** 一个回合：玩家输入行 → 正文 → 工具提示 → 选项 / 提示条。已提交回合与草稿回合共用。 */
export function TurnView({ items, drafting, interactive, selected, noticeActions }: Props) {
  const { t } = useTranslation()
  const markup = useContext(MarkupContext)
  const { reading } = useReaderContext()
  const lastText = items.findLastIndex((it) => it.t === 'text')

  return (
    <article className="turn">
      {items.map((it, i) => {
        switch (it.t) {
          case 'input':
            return <PlayerLine key={i} item={it} />
          case 'text':
            return (
              <div key={i} className="prose-book">
                <Markup source={it.markup} drafting={drafting && i === lastText} />
              </div>
            )
          case 'tools':
            return reading?.tool_hints === false ? null : <QuietLine key={i}>{it.summary}</QuietLine>
          case 'choices':
            return interactive ? <ChoiceList key={i} item={it} selected={selected} /> : <PastChoices key={i} item={it} />
          case 'notice':
            return (
              <Notice key={i} level={it.level} actions={noticeActions}>
                {it.text}
              </Notice>
            )
          case 'dice':
            return <DiceCard key={i} item={it} />
          case 'image': {
            const asset = findAsset(it.asset, markup)
            return asset?.kind === 'image' ? (
              <ImageViewer key={i} src={asset.url} caption={asset.description ?? ''} />
            ) : (
              <QuietLine key={i}>{t('reader.imageMissing')}</QuietLine>
            )
          }
        }
      })}
    </article>
  )
}
