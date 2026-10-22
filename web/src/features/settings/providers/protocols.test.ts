import { describe, expect, it } from 'vitest'

import { formatHeaders, parseHeaders, parsePrice, priceSummary } from './protocols'

describe('请求头文本', () => {
  it('逐行解析「名称: 值」，忽略空行，值中可以有冒号', () => {
    expect(parseHeaders('anthropic-beta: a\n\n  X-Trace : http://x:1 ')).toEqual({ 'anthropic-beta': 'a', 'X-Trace': 'http://x:1' })
  })
  it('没有冒号的行视为格式错误', () => {
    expect(parseHeaders('just-a-name')).toBeNull()
  })
  it('格式化后能原样解析回来', () => {
    const h = { a: '1', b: '2' }
    expect(parseHeaders(formatHeaders(h))).toEqual(h)
  })
})

describe('价格', () => {
  it('摘要显示输入 / 输出单价并乘以倍率', () => {
    expect(priceSummary({ input: 1.5, output: 7.5, currency: 'USD' }, 2)).toBe('$3 / $15')
    expect(priceSummary({ input: 5, output: 25, currency: 'CNY' })).toBe('¥5 / ¥25')
  })
  it('未知价格不显示', () => {
    expect(priceSummary(undefined)).toBeNull()
    expect(priceSummary({ currency: 'USD' })).toBeNull()
  })
  it('空输入为未知，负数与非数字无效', () => {
    expect(parsePrice('')).toBeUndefined()
    expect(parsePrice('-1')).toBeUndefined()
    expect(parsePrice('abc')).toBeUndefined()
    expect(parsePrice('0.15')).toBe(0.15)
  })
})
