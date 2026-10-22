/**
 * 输入栏的图片附件（《02-技术方案》15.1）：选择或粘贴图片后立即上传（POST /uploads），提交回合时以哈希引用。
 * 服务端会重新编码图片（去除 EXIF 等元数据）；这里只做能在本地做的检查（类型、大小、张数）。
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { api } from '@/api/endpoints'
import { errorMessage } from '@/api/errors'

/** 与后端一致：一次输入最多附带的图片数、单张图片的大小上限。 */
export const MAX_ATTACHMENTS = 6
const MAX_BYTES = 20 << 20
const ACCEPT = ['image/png', 'image/jpeg', 'image/webp', 'image/gif']
export const ACCEPT_ATTR = ACCEPT.join(',')

export interface Attachment {
  id: number
  /** 本地预览（object URL） */
  preview: string
  status: 'uploading' | 'done' | 'error'
  hash?: string
}

export function useAttachments() {
  const { t } = useTranslation()
  const [items, setItems] = useState<Attachment[]>([])
  const seq = useRef(0)
  const live = useRef(items)
  useEffect(() => {
    live.current = items
  }, [items])

  // 离开时释放预览图
  useEffect(() => () => live.current.forEach((a) => URL.revokeObjectURL(a.preview)), [])

  const add = useCallback(
    (files: Iterable<File>) => {
      const images = [...files].filter((f) => ACCEPT.includes(f.type))
      if (images.length === 0) return
      const room = MAX_ATTACHMENTS - live.current.length
      if (images.length > room) toast(t('reader.attachLimit', { count: MAX_ATTACHMENTS }))
      for (const file of images.slice(0, Math.max(room, 0))) {
        if (file.size > MAX_BYTES) {
          toast.error(t('reader.attachTooLarge', { name: file.name }))
          continue
        }
        const id = ++seq.current
        setItems((list) => [...list, { id, preview: URL.createObjectURL(file), status: 'uploading' }])
        const form = new FormData()
        form.append('file', file)
        api.uploads
          .create(form)
          .then((u) => setItems((list) => list.map((a) => (a.id === id ? { ...a, status: 'done', hash: u.hash } : a))))
          .catch((err: unknown) => {
            toast.error(errorMessage(err))
            setItems((list) => list.map((a) => (a.id === id ? { ...a, status: 'error' } : a)))
          })
      }
    },
    [t],
  )

  const remove = useCallback((id: number) => {
    setItems((list) => {
      const a = list.find((x) => x.id === id)
      if (a) URL.revokeObjectURL(a.preview)
      return list.filter((x) => x.id !== id)
    })
  }, [])

  const clear = useCallback(() => {
    setItems((list) => {
      list.forEach((a) => URL.revokeObjectURL(a.preview))
      return []
    })
  }, [])

  return {
    items,
    add,
    remove,
    clear,
    /** 已上传完成的图片哈希 */
    hashes: items.filter((a) => a.status === 'done').map((a) => a.hash!),
    uploading: items.some((a) => a.status === 'uploading'),
  }
}
