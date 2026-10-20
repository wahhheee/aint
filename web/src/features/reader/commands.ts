/**
 * 输入栏文字 → TurnInput（《02-技术方案》11.3）。
 *
 *   /ooc 文本           场外对话
 *   /continue           请执笔者接着写
 *   /follow 林舟        作品命令：参数按位置或 key=value，带空格的值用引号
 *   其他文字            自由输入（若选择了输入模式，则以该模式提交）
 */
import type { Command, TurnInput } from '@/api/types'

export const BUILTIN_COMMANDS = ['ooc', 'continue', 'compact', 'help'] as const
export type BuiltinCommand = (typeof BUILTIN_COMMANDS)[number]

export type ParseResult =
  | { ok: true; input: TurnInput }
  | { ok: true; local: 'help' | 'compact' }
  | { ok: false; error: 'not_found' | 'arg_missing'; name: string; arg?: string }

/** 拆分参数：支持引号与 key=value。 */
export function splitArgs(s: string): string[] {
  const out: string[] = []
  // 可选的 key= 前缀 + 引号包围的值或普通词：who="老 陈" → who=老 陈
  const re = /([\w-]+=)?(?:"([^"]*)"|'([^']*)'|(\S+))/g
  for (const m of s.matchAll(re)) out.push((m[1] ?? '') + (m[2] ?? m[3] ?? m[4]))
  return out
}

export function parseInput(text: string, commands: Command[], mode?: string): ParseResult {
  const trimmed = text.trim()
  if (!trimmed.startsWith('/')) {
    return { ok: true, input: mode ? { kind: 'free', text: trimmed, mode } : { kind: 'free', text: trimmed } }
  }
  const [, name = '', rest = ''] = /^\/(\S+)\s*([\s\S]*)$/.exec(trimmed) ?? []
  switch (name) {
    case 'ooc':
      return { ok: true, input: { kind: 'ooc', text: rest } }
    case 'continue':
      return { ok: true, input: { kind: 'continue', text: rest || undefined } }
    case 'help':
    case 'compact':
      return { ok: true, local: name }
  }
  const cmd = commands.find((c) => c.name === name && (c.present.includes('slash') || c.present.includes('button')))
  if (!cmd) return { ok: false, error: 'not_found', name }

  const args: Record<string, string> = {}
  const positional: string[] = []
  for (const tok of splitArgs(rest)) {
    const kv = /^([\w-]+)=(.*)$/.exec(tok)
    if (kv && cmd.args?.some((a) => a.name === kv[1])) args[kv[1]] = kv[2]
    else positional.push(tok)
  }
  for (const a of cmd.args ?? []) {
    if (args[a.name] === undefined && positional.length > 0) args[a.name] = positional.shift()!
  }
  const missing = cmd.args?.find((a) => a.required && !args[a.name])
  if (missing) return { ok: false, error: 'arg_missing', name, arg: missing.hint ?? missing.name }
  const leftover = positional.join(' ')
  return { ok: true, input: { kind: 'command', command: { name, args }, text: leftover || undefined } }
}
