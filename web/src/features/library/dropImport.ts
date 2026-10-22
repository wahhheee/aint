/**
 * 拖放导入：从 DataTransfer 中取出一个 zip 包，或递归读取拖入的文件夹。
 *
 * DataTransfer 的条目只在 drop 事件处理期间有效，所以 pickedFromDrop 先同步取出全部条目，再异步遍历。
 * 目录遍历用 File and Directory Entries API（webkitGetAsEntry，主流浏览器都支持）。
 */
import { IMPORT_LIMITS, isAlwaysIgnored, isZipFile, pickedFromFolder, problem, type FolderEntry, type Picked } from './importSource'

/** 拖入的一项：目录条目（浏览器支持时）与文件。 */
export interface DroppedItem {
  entry: FileSystemEntry | null
  file: File | null
}

/** 拖放是否带有文件（拖动页面上的文字、链接时不处理）。 */
export function hasFiles(dt: DataTransfer | null): boolean {
  return !!dt && Array.from(dt.types).includes('Files')
}

/** 从 drop 事件取出要导入的内容。必须在事件处理函数中同步调用（之后条目即失效）。 */
export function pickedFromDrop(dt: DataTransfer): Promise<Picked> {
  const items = Array.from(dt.items ?? []).filter((i) => i.kind === 'file')
  const dropped: DroppedItem[] =
    items.length > 0
      ? items.map((i) => ({ entry: i.webkitGetAsEntry?.() ?? null, file: i.getAsFile() }))
      : Array.from(dt.files, (file) => ({ entry: null, file }))
  return pickedFromDropped(dropped)
}

/** 拖入的条目 → 导入内容：一个文件夹，或一个 zip 包。 */
export async function pickedFromDropped(items: DroppedItem[]): Promise<Picked> {
  if (items.length === 0) return problem('empty')
  if (items.length > 1) return problem('multiple')
  const { entry, file } = items[0]
  if (entry?.isDirectory) {
    try {
      const entries = await readTree(entry as FileSystemDirectoryEntry, IMPORT_LIMITS.maxFiles * 10)
      return entries ? pickedFromFolder(entries) : problem('tooManyFiles')
    } catch {
      return problem('readFailed')
    }
  }
  return file && isZipFile(file) ? { kind: 'zip', file } : problem('notZip')
}

/**
 * 递归读取文件夹（广度优先），跳过始终忽略的文件与文件夹（不进入 .git、node_modules）。
 * 文件数超过 limit 时返回 null。路径取自 fullPath（以 "/<文件夹名>/" 开头）。
 */
export async function readTree(root: FileSystemDirectoryEntry, limit: number): Promise<FolderEntry[] | null> {
  const out: FolderEntry[] = []
  const queue: FileSystemDirectoryEntry[] = [root]
  for (let dir = queue.shift(); dir; dir = queue.shift()) {
    for (const child of await readAll(dir)) {
      if (isAlwaysIgnored(child.name)) continue
      if (child.isDirectory) {
        queue.push(child as FileSystemDirectoryEntry)
      } else if (child.isFile) {
        if (out.length >= limit) return null
        out.push({ path: child.fullPath, file: await fileOf(child as FileSystemFileEntry) })
      }
    }
  }
  return out
}

/** readEntries 每次最多返回一批（Chrome 为 100 个），要反复调用直到返回空数组。 */
async function readAll(dir: FileSystemDirectoryEntry): Promise<FileSystemEntry[]> {
  const reader = dir.createReader()
  const all: FileSystemEntry[] = []
  for (;;) {
    const batch = await new Promise<FileSystemEntry[]>((resolve, reject) => reader.readEntries(resolve, reject))
    if (batch.length === 0) return all
    all.push(...batch)
  }
}

function fileOf(entry: FileSystemFileEntry): Promise<File> {
  return new Promise((resolve, reject) => entry.file(resolve, reject))
}
