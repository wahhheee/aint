import { describe, expect, it } from 'vitest'

import {
  buildImportForm,
  checkInput,
  IMPORT_LIMITS,
  inputName,
  inputSize,
  isAlwaysIgnored,
  isZipFile,
  pickedFromFileList,
  pickedFromFolder,
  pickedFromZip,
  type FolderEntry,
  type ImportInput,
} from './importSource'

const file = (name: string, content = 'x', type = '') => new File([content], name, { type })

/** 带相对路径的文件（<input webkitdirectory> 选出来的样子）。 */
function relFile(path: string, content = 'x'): File {
  const f = file(path.split('/').pop() ?? path, content)
  Object.defineProperty(f, 'webkitRelativePath', { value: path })
  return f
}

/** 只有大小的假文件：测试体积上限时不必真的分配几百 MB。 */
function sized(name: string, size: number): File {
  const f = file(name)
  Object.defineProperty(f, 'size', { value: size })
  return f
}

describe('isAlwaysIgnored', () => {
  it('任意层级的 .git、node_modules 等', () => {
    expect(isAlwaysIgnored('misty/.git/HEAD')).toBe(true)
    expect(isAlwaysIgnored('misty/web/node_modules/a/b.js')).toBe(true)
    expect(isAlwaysIgnored('misty/assets/.DS_Store')).toBe(true)
    expect(isAlwaysIgnored('__MACOSX/misty/._work.yaml')).toBe(true)
    expect(isAlwaysIgnored('misty/work.yaml')).toBe(false)
    expect(isAlwaysIgnored('misty/.gitignore')).toBe(false)
  })
})

describe('isZipFile / pickedFromZip', () => {
  it('按扩展名或 MIME 判断', () => {
    expect(isZipFile(file('作品.ZIP'))).toBe(true)
    expect(isZipFile(file('download', 'x', 'application/x-zip-compressed'))).toBe(true)
    expect(isZipFile(file('work.yaml', 'x', 'text/yaml'))).toBe(false)
  })

  it('只接受一个 zip 文件', () => {
    const zip = file('misty.zip')
    expect(pickedFromZip([zip])).toEqual({ kind: 'zip', file: zip })
    expect(pickedFromZip([])).toEqual({ kind: 'problem', problem: 'empty' })
    expect(pickedFromZip(null)).toEqual({ kind: 'problem', problem: 'empty' })
    expect(pickedFromZip([zip, file('b.zip')])).toEqual({ kind: 'problem', problem: 'multiple' })
    expect(pickedFromZip([file('cover.png')])).toEqual({ kind: 'problem', problem: 'notZip' })
  })
})

describe('pickedFromFolder', () => {
  it('规范化路径、去掉始终忽略的文件，文件夹名取第一段', () => {
    const entries: FolderEntry[] = [
      { path: '/misty/work.yaml', file: file('work.yaml') },
      { path: 'misty\\lore\\lin.md', file: file('lin.md') },
      { path: '/misty/.git/HEAD', file: file('HEAD') },
      { path: '/misty/Thumbs.db', file: file('Thumbs.db') },
    ]
    const got = pickedFromFolder(entries)
    expect(got.kind).toBe('folder')
    if (got.kind !== 'folder') return
    expect(got.name).toBe('misty')
    expect(got.entries.map((e) => e.path)).toEqual(['misty/work.yaml', 'misty/lore/lin.md'])
  })

  it('全部被忽略或为空时不能导入', () => {
    expect(pickedFromFolder([])).toEqual({ kind: 'problem', problem: 'empty' })
    expect(pickedFromFolder([{ path: 'misty/.git/HEAD', file: file('HEAD') }])).toEqual({ kind: 'problem', problem: 'empty' })
  })

  it('<input webkitdirectory> 的文件用 webkitRelativePath', () => {
    const got = pickedFromFileList([relFile('雾港来信/work.yaml'), relFile('雾港来信/node_modules/x.js')])
    expect(got).toMatchObject({ kind: 'folder', name: '雾港来信', entries: [{ path: '雾港来信/work.yaml' }] })
  })
})

describe('checkInput', () => {
  const folder = (...files: File[]): ImportInput => ({
    kind: 'folder',
    name: 'misty',
    entries: files.map((f) => ({ path: `misty/${f.name}`, file: f })),
  })

  it('zip 包按上传上限（作品总量加余量）检查', () => {
    expect(checkInput({ kind: 'zip', file: sized('a.zip', IMPORT_LIMITS.maxTotal + IMPORT_LIMITS.uploadSlack) })).toBeNull()
    expect(checkInput({ kind: 'zip', file: sized('a.zip', IMPORT_LIMITS.maxTotal + IMPORT_LIMITS.uploadSlack + 1) })).toBe('tooLarge')
  })

  it('文件夹按总量与文件数检查', () => {
    const half = IMPORT_LIMITS.maxTotal / 2
    expect(checkInput(folder(sized('a.png', half), sized('b.png', half)))).toBeNull()
    expect(checkInput(folder(sized('a.png', half), sized('b.png', half + 1)))).toBe('tooLarge')
    const many = Array.from({ length: IMPORT_LIMITS.maxFiles * 10 + 1 }, (_, i) => sized(`${i}.md`, 1))
    expect(checkInput(folder(...many))).toBe('tooManyFiles')
  })

  it('名称与大小', () => {
    const input = folder(sized('a.png', 3), sized('b.png', 4))
    expect(inputName(input)).toBe('misty')
    expect(inputSize(input)).toBe(7)
    expect(inputName({ kind: 'zip', file: file('雾港来信.zip') })).toBe('雾港来信.zip')
  })
})

describe('buildImportForm', () => {
  it('zip：source、name、file 依次排列', () => {
    const zip = file('misty.zip', 'PK')
    const form = buildImportForm({ kind: 'zip', file: zip })
    expect([...form.keys()]).toEqual(['source', 'name', 'file'])
    expect(form.get('source')).toBe('zip')
    expect(form.get('name')).toBe('misty.zip')
    expect((form.get('file') as File).name).toBe('misty.zip')
  })

  it('文件夹：每个文件的 path 紧挨在 file 之前', () => {
    const a = file('work.yaml')
    const b = file('lin.md')
    const form = buildImportForm({
      kind: 'folder',
      name: 'misty',
      entries: [
        { path: 'misty/work.yaml', file: a },
        { path: 'misty/lore/lin.md', file: b },
      ],
    })
    expect([...form.entries()].map(([k, v]) => (typeof v === 'string' ? `${k}=${v}` : `${k}:${v.name}`))).toEqual([
      'source=dir',
      'name=misty',
      'path=misty/work.yaml',
      'file:work.yaml',
      'path=misty/lore/lin.md',
      'file:lin.md',
    ])
  })
})
