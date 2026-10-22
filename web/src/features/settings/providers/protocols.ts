/**
 * 供应商设置的纯函数与常量（便于单测）。
 */
import type { Price, Protocol } from '@/api/types'

export type RealProtocol = Exclude<Protocol, 'fake'>

export const PROTOCOLS: { id: RealProtocol; label: string; baseUrl: string }[] = [
  { id: 'anthropic', label: 'Anthropic Messages', baseUrl: 'https://api.anthropic.com' },
  { id: 'openai-responses', label: 'OpenAI Responses', baseUrl: 'https://api.openai.com/v1' },
  { id: 'gemini', label: 'Gemini', baseUrl: 'https://generativelanguage.googleapis.com' },
]

export const PROTOCOL_LABEL: Record<Protocol, string> = {
  'openai-responses': 'OpenAI Responses',
  anthropic: 'Anthropic Messages',
  gemini: 'Gemini',
  fake: '演示模型',
}

export const defaultBaseUrl = (p: RealProtocol) => PROTOCOLS.find((x) => x.id === p)?.baseUrl ?? ''

/** 把「名称: 值」逐行文本解析为请求头；空行忽略，没有冒号的行报错（返回 null）。 */
export function parseHeaders(text: string): Record<string, string> | null {
  const out: Record<string, string> = {}
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line) continue
    const i = line.indexOf(':')
    if (i <= 0) return null
    out[line.slice(0, i).trim()] = line.slice(i + 1).trim()
  }
  return out
}

export function formatHeaders(h?: Record<string, string>): string {
  return Object.entries(h ?? {})
    .map(([k, v]) => `${k}: ${v}`)
    .join('\n')
}

const CURRENCY_SYMBOL: Record<string, string> = { CNY: '¥', USD: '$' }

/** 价格摘要："¥5 / ¥25（输入 / 输出，每百万 token）"中的前半部分；未知返回 null。 */
export function priceSummary(p?: Price, multiplier = 1): string | null {
  if (!p || (p.input === undefined && p.output === undefined)) return null
  const sym = CURRENCY_SYMBOL[p.currency ?? 'USD'] ?? `${p.currency} `
  const fmt = (v?: number) => (v === undefined ? '?' : `${sym}${trim(v * multiplier)}`)
  return `${fmt(p.input)} / ${fmt(p.output)}`
}

function trim(v: number): string {
  return Number.isInteger(v) ? String(v) : String(Number(v.toFixed(4)))
}

/** 数字输入框：空字符串表示未知（undefined），非法输入也视为未知。 */
export function parsePrice(s: string): number | undefined {
  if (s.trim() === '') return undefined
  const v = Number(s)
  return Number.isFinite(v) && v >= 0 ? v : undefined
}
