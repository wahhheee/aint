import { describe, expect, it } from 'vitest'

import { hasFiles, pickedFromDropped, readTree } from './dropImport'

/**
 * 假的目录条目（File and Directory Entries API）。readEntries 每批最多返回 2 个，
 * 验证实现会反复调用直到返回空数组。
 */
type Node = string | { [name: string]: Node }

function fileEntry(fullPath: string, content: string): FileSystemFileEntry {
  const name = fullPath.split('/').pop() ?? ''
  return {
    isFile: true,
    isDirectory: false,
    name,
    fullPath,
    file: (ok: (f: File) => void) => ok(new File([content], name)),
  } as unknown as FileSystemFileEntry
}

function dirEntry(fullPath: string, children: Record<string, Node>): FileSystemDirectoryEntry {
  const name = fullPath.split('/').pop() ?? ''
  const entries = Object.entries(children).map(([n, c]) =>
    typeof c === 'string' ? fileEntry(`${fullPath}/${n}`, c) : dirEntry(`${fullPath}/${n}`, c),
  )
  return {
    isFile: false,
    isDirectory: true,
    name,
    fullPath,
    createReader: () => {
      let i = 0
      return {
        readEntries: (ok: (batch: FileSystemEntry[]) => void) => {
          ok(entries.slice(i, i + 2))
          i += 2
        },
      }
    },
  } as unknown as FileSystemDirectoryEntry
}

const misty = () =>
  dirEntry('/misty', {
    'work.yaml': 'format: 1',
    'README.md': '# 雾港来信',
    lore: { 'lin.md': '林舟', 'chen.md': '老陈', places: { 'harbor.md': '港口' } },
    '.git': { HEAD: 'ref', objects: { ab: 'x' } },
    node_modules: { x: { 'index.js': '' } },
    '.DS_Store': '',
  })

describe('readTree', () => {
  it('递归读取全部文件，跳过 .git、node_modules 等', async () => {
    const entries = await readTree(misty(), 100)
    expect(entries?.map((e) => e.path).sort()).toEqual([
      '/misty/README.md',
      '/misty/lore/chen.md',
      '/misty/lore/lin.md',
      '/misty/lore/places/harbor.md',
      '/misty/work.yaml',
    ])
  })

  it('超过文件数上限时返回 null', async () => {
    expect(await readTree(misty(), 4)).toBeNull()
    expect(await readTree(misty(), 5)).toHaveLength(5)
  })
})

describe('pickedFromDropped', () => {
  it('拖入文件夹：路径含文件夹名', async () => {
    const got = await pickedFromDropped([{ entry: misty(), file: null }])
    expect(got).toMatchObject({ kind: 'folder', name: 'misty' })
    if (got.kind === 'folder') expect(got.entries.map((e) => e.path)).toContain('misty/lore/places/harbor.md')
  })

  it('拖入 zip 包', async () => {
    const zip = new File(['PK'], '雾港来信.zip', { type: 'application/zip' })
    expect(await pickedFromDropped([{ entry: fileEntry('/雾港来信.zip', 'PK'), file: zip }])).toEqual({ kind: 'zip', file: zip })
    // 不支持目录条目的浏览器：只有文件
    expect(await pickedFromDropped([{ entry: null, file: zip }])).toEqual({ kind: 'zip', file: zip })
  })

  it('不能导入的情况', async () => {
    const png = new File(['x'], 'cover.png')
    expect(await pickedFromDropped([])).toEqual({ kind: 'problem', problem: 'empty' })
    expect(await pickedFromDropped([{ entry: null, file: png }])).toEqual({ kind: 'problem', problem: 'notZip' })
    expect(
      await pickedFromDropped([
        { entry: misty(), file: null },
        { entry: null, file: png },
      ]),
    ).toEqual({ kind: 'problem', problem: 'multiple' })
    expect(await pickedFromDropped([{ entry: dirEntry('/empty', { '.git': { HEAD: 'x' } }), file: null }])).toEqual({
      kind: 'problem',
      problem: 'empty',
    })
  })

  it('读取失败', async () => {
    const broken = {
      isFile: false,
      isDirectory: true,
      name: 'broken',
      fullPath: '/broken',
      createReader: () => ({ readEntries: (_ok: unknown, fail: (e: Error) => void) => fail(new Error('NotReadableError')) }),
    } as unknown as FileSystemDirectoryEntry
    expect(await pickedFromDropped([{ entry: broken, file: null }])).toEqual({ kind: 'problem', problem: 'readFailed' })
  })
})

describe('hasFiles', () => {
  it('只处理带文件的拖放', () => {
    expect(hasFiles(null)).toBe(false)
    expect(hasFiles({ types: ['text/plain'] } as unknown as DataTransfer)).toBe(false)
    expect(hasFiles({ types: ['Files'] } as unknown as DataTransfer)).toBe(true)
  })
})
