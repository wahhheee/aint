import { describe, expect, it } from 'vitest'

import type { Command } from '@/api/types'

import { parseInput } from './commands'

const follow: Command = {
  name: 'follow',
  title: '跟随',
  description: '',
  args: [{ name: 'who', required: true }],
  present: ['slash', 'button'],
  ai_visible: true,
}

describe('parseInput', () => {
  it('普通文字是自由输入；选择了模式时带上模式', () => {
    expect(parseInput('推开门', [])).toEqual({ ok: true, input: { kind: 'free', text: '推开门' } })
    expect(parseInput('信是老陈拿走的', [], '推论')).toEqual({ ok: true, input: { kind: 'free', text: '信是老陈拿走的', mode: '推论' } })
  })

  it('内置命令', () => {
    expect(parseInput('/ooc 节奏慢一点', [])).toEqual({ ok: true, input: { kind: 'ooc', text: '节奏慢一点' } })
    expect(parseInput('/help', [])).toEqual({ ok: true, local: 'help' })
  })

  it('作品命令：按位置与 key=value 解析参数', () => {
    expect(parseInput('/follow 林舟', [follow])).toEqual({
      ok: true,
      input: { kind: 'command', command: { name: 'follow', args: { who: '林舟' } } },
    })
    expect(parseInput('/follow who="老 陈"', [follow])).toEqual({
      ok: true,
      input: { kind: 'command', command: { name: 'follow', args: { who: '老 陈' } } },
    })
  })

  it('缺少必填参数或命令不存在', () => {
    expect(parseInput('/follow', [follow])).toMatchObject({ ok: false, error: 'arg_missing' })
    expect(parseInput('/fly', [follow])).toMatchObject({ ok: false, error: 'not_found' })
  })
})

it('重写要求完整保留，缺少要求时不进入故事输入', () => {
  expect(parseInput('/rewrite 更含蓄，保留“蓝伞”\n暂时不提供选项', [])).toEqual({
    ok: true,
    operation: 'rewrite',
    instructions: '更含蓄，保留“蓝伞”\n暂时不提供选项',
  })
  expect(parseInput('/rewrite', [])).toMatchObject({ ok: false, error: 'arg_missing' })
  expect(parseInput('/choices', [])).toEqual({ ok: true, operation: 'choices', instructions: '' })
})
