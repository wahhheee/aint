import type { TimelineNode } from '@/api/types'

/**
 * 只显示到 parentId 为止：重新生成、修改输入时新回合的父节点较早，父节点之后的回合先隐去。
 * 父节点是 root（重新生成第一个回合）时它不在时间线中，以"父节点为它的回合"为界。
 */
export function cutAfter(nodes: TimelineNode[], parentId: string | null | undefined): TimelineNode[] {
  if (!parentId) return nodes
  const i = nodes.findIndex((n) => n.id === parentId)
  if (i >= 0) return nodes.slice(0, i + 1)
  const j = nodes.findIndex((n) => n.parent_id === parentId)
  return j >= 0 ? nodes.slice(0, j) : nodes
}
