/**
 * 导入来源：玩家选择或拖入的内容 → 上传表单。纯函数，便于测试（拖放的目录遍历见 dropImport.ts）。
 *
 * 上传表单（POST /works/import，multipart，字段按顺序读取，见 internal/server/api_works.go）：
 *   zip：   source=zip，name=文件名，file=zip 包
 *   文件夹：source=dir，name=文件夹名，然后每个文件一对 path（相对路径，含文件夹名）+ file
 */

const MiB = 1024 * 1024

/**
 * 与后端 work.DefaultLimits 一致（internal/work/fileset.go）。后端是权威，这里只为在上传前拦下明显超限的内容，
 * 不必等传完几百 MB 才被拒绝（超过请求体上限时服务端会直接断开连接，浏览器只能看到"网络错误"）。
 */
export const IMPORT_LIMITS = {
  /** 作品解压后的总体积 */
  maxTotal: 512 * MiB,
  /** 文件数（不含被忽略的文件） */
  maxFiles: 5000,
  /** 上传请求体比作品总量多留的余量（zip 头与表单分隔），见 work.Limits.UploadLimit */
  uploadSlack: 16 * MiB,
}

/**
 * 始终忽略的文件与文件夹名（任意层级），与后端 internal/work/ignore.go 的 alwaysIgnored 一致。
 * 选择文件夹时直接不上传：.git、node_modules 里常有成千上万个文件。作品的 .yanyiignore 由后端处理。
 */
export const ALWAYS_IGNORED = ['.git', '.github', '.DS_Store', 'Thumbs.db', 'desktop.ini', 'node_modules', '__MACOSX']

export function isAlwaysIgnored(path: string): boolean {
  return path.split('/').some((seg) => ALWAYS_IGNORED.includes(seg))
}

/** 文件夹中的一个文件：path 是相对路径，含所选文件夹本身的名字（如 misty-harbor/work.yaml）。 */
export interface FolderEntry {
  path: string
  file: File
}

/** 要导入的内容：一个 zip 包，或一个文件夹。 */
export type ImportInput = { kind: 'zip'; file: File } | { kind: 'folder'; name: string; entries: FolderEntry[] }

/** 选择或拖入的内容不能导入的原因（文案见 i18n 的 import.problems）。 */
export type InputProblem = 'empty' | 'notZip' | 'multiple' | 'tooLarge' | 'tooManyFiles' | 'readFailed'

/** 玩家选择或拖入的结果：可以导入的内容，或者不能导入的原因。 */
export type Picked = ImportInput | { kind: 'problem'; problem: InputProblem }

export const problem = (p: InputProblem): Picked => ({ kind: 'problem', problem: p })

/** 是否 zip 包：看扩展名与 MIME 类型（Windows 上 zip 的 MIME 常是 application/x-zip-compressed）。 */
export function isZipFile(f: { name: string; type: string }): boolean {
  return /\.zip$/i.test(f.name) || ['application/zip', 'application/x-zip-compressed'].includes(f.type)
}

/** 选择的 zip 文件 → 导入内容。 */
export function pickedFromZip(files: ArrayLike<File> | null | undefined): Picked {
  if (!files || files.length === 0) return problem('empty')
  if (files.length > 1) return problem('multiple')
  return isZipFile(files[0]) ? { kind: 'zip', file: files[0] } : problem('notZip')
}

/**
 * 文件夹中的文件 → 导入内容：规范化路径（'\' → '/'，去掉开头的 '/'），去掉始终忽略的文件。
 * 文件夹名取第一段：<input webkitdirectory> 的 webkitRelativePath 与拖放的 fullPath 都以所选文件夹开头。
 */
export function pickedFromFolder(entries: FolderEntry[]): Picked {
  const kept: FolderEntry[] = []
  for (const e of entries) {
    const path = e.path.replaceAll('\\', '/').replace(/^\/+/, '')
    if (path && !isAlwaysIgnored(path)) kept.push({ path, file: e.file })
  }
  if (kept.length === 0) return problem('empty')
  return { kind: 'folder', name: kept[0].path.split('/')[0], entries: kept }
}

/** <input webkitdirectory> 选择的文件 → 导入内容。 */
export function pickedFromFileList(files: ArrayLike<File> | null | undefined): Picked {
  return pickedFromFolder(Array.from(files ?? [], (file) => ({ path: file.webkitRelativePath || file.name, file })))
}

/** 导入内容的显示名（zip 文件名或文件夹名）。 */
export function inputName(input: ImportInput): string {
  return input.kind === 'zip' ? input.file.name : input.name
}

/** 要上传的字节数。 */
export function inputSize(input: ImportInput): number {
  return input.kind === 'zip' ? input.file.size : input.entries.reduce((n, e) => n + e.file.size, 0)
}

/** 上传前的限制检查：超过时返回原因。 */
export function checkInput(input: ImportInput): InputProblem | null {
  const size = inputSize(input)
  if (input.kind === 'zip') return size > IMPORT_LIMITS.maxTotal + IMPORT_LIMITS.uploadSlack ? 'tooLarge' : null
  // .yanyiignore 由后端处理，这里只能按上传的总量算；条目数与后端一样留出十倍余量
  if (size > IMPORT_LIMITS.maxTotal) return 'tooLarge'
  if (input.entries.length > IMPORT_LIMITS.maxFiles * 10) return 'tooManyFiles'
  return null
}

/** 生成上传表单。字段顺序有意义：后端流式读取，source 与 name 在前，每个文件的 path 紧挨在 file 之前。 */
export function buildImportForm(input: ImportInput): FormData {
  const form = new FormData()
  if (input.kind === 'zip') {
    form.append('source', 'zip')
    form.append('name', input.file.name)
    form.append('file', input.file, input.file.name)
    return form
  }
  form.append('source', 'dir')
  form.append('name', input.name)
  for (const e of input.entries) {
    form.append('path', e.path)
    form.append('file', e.file, e.file.name)
  }
  return form
}
