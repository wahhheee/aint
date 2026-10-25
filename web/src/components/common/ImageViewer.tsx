import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'

/** 图片只使用已解析的素材地址，放大时保留文字说明与键盘关闭操作。 */
export function ImageViewer({ src, caption }: { src: string; caption: string }) {
  const [open, setOpen] = useState(false)
  const { t } = useTranslation()
  return (
    <>
      <figure className="cg-card">
        <button type="button" className="block w-full cursor-zoom-in" aria-label={t('reader.enlargeImage')} onClick={() => setOpen(true)}>
          <img src={src} alt={caption} loading="lazy" />
        </button>
        {caption && <figcaption>{caption}</figcaption>}
      </figure>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[95dvh] sm:max-w-[95vw]">
          <DialogHeader className="sr-only">
            <DialogTitle>{t('reader.enlargeImage')}</DialogTitle>
            <DialogDescription>{caption || t('reader.enlargeImage')}</DialogDescription>
          </DialogHeader>
          <img src={src} alt={caption} className="mx-auto max-h-[80dvh] max-w-full object-contain" />
          {caption && <p className="text-center text-sm text-ink-muted">{caption}</p>}
        </DialogContent>
      </Dialog>
    </>
  )
}
