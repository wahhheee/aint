import { describe, expect, it } from 'vitest'

import { formatCosts, formatMoney, formatTokens } from './format'

describe('formatMoney', () => {
  it('常见货币用符号，其余写货币代码', () => {
    expect(formatMoney(210_000, 'CNY')).toBe('¥0.21')
    expect(formatMoney(14_800_000, 'USD')).toBe('$14.80')
    expect(formatMoney(1_500_000, 'EUR')).toBe('EUR 1.50')
  })

  it('不足 0.1 时保留两位有效数字，不显示成 0.00', () => {
    expect(formatMoney(3_412, 'CNY')).toBe('¥0.0034')
    expect(formatMoney(1, 'USD')).toBe('$0.000001')
    expect(formatMoney(0, 'USD')).toBe('$0.00')
  })
})

describe('formatCosts', () => {
  it('多种货币分别列出，不做汇率换算', () => {
    expect(formatCosts({ USD: 1_200_000, CNY: 3_500_000 })).toBe('¥3.50 + $1.20')
    expect(formatCosts({})).toBe('')
  })
})

describe('formatTokens', () => {
  it('千位以上写成 k', () => {
    expect(formatTokens(812)).toBe('812')
    expect(formatTokens(81_200)).toBe('81.2k')
    expect(formatTokens(128_000)).toBe('128k')
  })
})
