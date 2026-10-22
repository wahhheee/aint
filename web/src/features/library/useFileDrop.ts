import { useEffect, useEffectEvent, useState } from 'react'

import { hasFiles } from './dropImport'

/**
 * 整页接收拖入的文件（书架页：把 zip 包或作品文件夹拖到窗口任意位置即可导入）。
 * 返回是否正在拖着文件经过窗口，用于显示"松开即可导入"的遮罩。
 *
 * dragenter / dragleave 在子元素之间移动时会成对触发，用计数判断是否真的离开了窗口。
 * 只处理带文件的拖放：拖动页面上的文字、链接不受影响。
 */
export function useFileDrop(onDrop: (dt: DataTransfer) => void): boolean {
  const [dragging, setDragging] = useState(false)
  const handleDrop = useEffectEvent(onDrop)

  useEffect(() => {
    let depth = 0
    const enter = (e: DragEvent) => {
      if (!hasFiles(e.dataTransfer)) return
      e.preventDefault()
      depth++
      setDragging(true)
    }
    const over = (e: DragEvent) => {
      if (!hasFiles(e.dataTransfer)) return
      e.preventDefault() // 不阻止默认行为就不会触发 drop（浏览器会直接打开文件）
      if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
    }
    const leave = (e: DragEvent) => {
      if (!hasFiles(e.dataTransfer)) return
      depth = Math.max(0, depth - 1)
      if (depth === 0) setDragging(false)
    }
    const drop = (e: DragEvent) => {
      if (!hasFiles(e.dataTransfer)) return
      e.preventDefault()
      depth = 0
      setDragging(false)
      if (e.dataTransfer) handleDrop(e.dataTransfer)
    }
    window.addEventListener('dragenter', enter)
    window.addEventListener('dragover', over)
    window.addEventListener('dragleave', leave)
    window.addEventListener('drop', drop)
    return () => {
      window.removeEventListener('dragenter', enter)
      window.removeEventListener('dragover', over)
      window.removeEventListener('dragleave', leave)
      window.removeEventListener('drop', drop)
    }
  }, [])

  return dragging
}
