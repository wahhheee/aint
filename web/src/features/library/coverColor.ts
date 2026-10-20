import { hashString } from '@/lib/format'

/** 传统染色：磁青、黛蓝、苍绿、赭石、绛紫、玄青。按作品 ID 稳定选取：同一部作品在任何地方都是同一种颜色。 */
const DYES = ['#23345a', '#2f4152', '#2d4a41', '#6a4231', '#4b2f45', '#26292f'] as const

export function coverColor(id: string): string {
  return DYES[hashString(id) % DYES.length]
}
