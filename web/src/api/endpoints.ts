/**
 * 后端接口（《02-技术方案》18.2）。组件不直接调用这里，而是通过 queries.ts 的 hooks。
 */
import { http, type UploadOptions } from './client'
import type {
  BranchTree,
  DeepPartial,
  DebugStatus,
  DebugCall,
  DebugFileContent,
  ImportPreview,
  ImportResult,
  MemoryFile,
  MemoryView,
  Model,
  ModelInput,
  ModelPreset,
  PanelRender,
  Presentation,
  ProviderInput,
  ProviderView,
  RemoteModel,
  Report,
  Save,
  SaveSettings,
  Settings,
  StateChange,
  StateOp,
  StateView,
  SystemInfo,
  TestReport,
  Timeline,
  TurnInput,
  Upload,
  UsageSummary,
  WorkDetail,
  WorkSummary,
} from './types'

/** 用量统计的范围：各条件同时满足；都省略时为全局累计 */
export interface UsageFilter {
  save_id?: string
  work_id?: string
  /** 某个回合或整理节点 */
  node_id?: string
}

export interface CreateSaveParams {
  opening_id?: string
  work_id: string
  version_id?: string
  title?: string
  form?: Record<string, unknown>
  settings: Partial<SaveSettings>
}

export interface RemoteImportParams {
  source: 'github' | 'url' | 'dev' | 'dir'
  url?: string
  path?: string
  ref?: string
  subdir?: string
  timeout_seconds?: number
  max_download_mib?: number
}

export const api = {
  access: {
    status: () => http.get<{ enabled: boolean; authenticated: boolean; exposed: boolean }>('/auth/status'),
    login: (password: string) => http.post('/auth/login', { password }),
    logout: () => http.post('/auth/logout'),
    password: (current: string, password: string) => http.put('/auth/password', { current_password: current, password }),
    revoke: () => http.del('/auth/sessions'),
    github: () => http.get<{ configured: boolean }>('/settings/github-token'),
    setGitHub: (token: string) => http.put<{ configured: boolean }>('/settings/github-token', { token }),
  },
  debug: {
    imageURL: (id: string, call: string, blob: string) => `/api/v1/saves/${id}/debug/calls/${call}/images/${blob.replace(/^sha256:/, '')}`,
    status: (id: string) => http.get<DebugStatus>(`/saves/${id}/debug`),
    set: (id: string, enabled: boolean) => http.patch<DebugStatus>(`/saves/${id}/debug`, { enabled }),
    clear: (id: string) => http.del<DebugStatus>(`/saves/${id}/debug`),
    call: (id: string, call: string) => http.get<DebugCall>(`/saves/${id}/debug/calls/${call}`),
    file: (id: string, call: string, phase: string, path: string, offset: number) =>
      http.get<DebugFileContent>(`/saves/${id}/debug/calls/${call}/file?${new URLSearchParams({ phase, path, offset: String(offset) })}`),
  },
  system: {
    collect: () => http.post<{ blobs: number; bytes: number }>('/storage/gc'),
    info: () => http.get<SystemInfo>('/system/info'),
  },
  settings: {
    get: () => http.get<Settings>('/settings'),
    patch: (patch: DeepPartial<Settings>) => http.patch<Settings>('/settings', patch),
  },
  works: {
    remotePreview: (params: RemoteImportParams, signal?: AbortSignal) => http.post<ImportPreview>('/works/import', params, signal),
    checkUpdate: (id: string, versionId: string) =>
      http.post<{ changed: boolean; preview?: ImportPreview }>(`/works/${encodeURIComponent(id)}/check-update`, { version_id: versionId }),
    remove: (id: string, versionId?: string) => http.del(`/works/${encodeURIComponent(id)}${versionId ? `/versions/${versionId}` : ''}`),
    exportURL: (id: string, versionId: string) => `/api/v1/works/${encodeURIComponent(id)}/versions/${versionId}/export`,
    list: () => http.get<WorkSummary[]>('/works'),
    get: (id: string) => http.get<WorkDetail>(`/works/${encodeURIComponent(id)}`),
    saves: (id: string) => http.get<Save[]>(`/works/${encodeURIComponent(id)}/saves`),
    report: (id: string, versionId: string) => http.get<Report>(`/works/${encodeURIComponent(id)}/versions/${versionId}/report`),
    /** 导入第一步：上传 zip 包或文件夹（表单见 features/library/importSource.ts），解析与校验，不写入 */
    importPreview: (form: FormData, opts?: UploadOptions) => http.upload<ImportPreview>('/works/import', form, opts),
    /** 导入第二步：写入作品库（同作品同内容时复用已有版本） */
    importConfirm: (task: string) => http.post<ImportResult>(`/works/import/${encodeURIComponent(task)}/confirm`),
    /** 放弃导入，删除暂存文件 */
    importCancel: (task: string) => http.del(`/works/import/${encodeURIComponent(task)}`),
  },
  saves: {
    upgrade: (id: string, versionId: string) => http.post<void>(`/saves/${id}/upgrade`, { version_id: versionId }),
    title: (id: string) => http.post<void>(`/saves/${id}/title`),
    exit: (id: string) => http.post<void>(`/saves/${id}/exit`),
    settings: (id: string, settings: Partial<SaveSettings>) => http.patch<Save>(`/saves/${id}`, { settings }),
    recent: (limit = 6) => http.get<Save[]>(`/saves?limit=${limit}`),
    create: (p: CreateSaveParams) => http.post<Save>('/saves', p),
    get: (id: string) => http.get<Presentation>(`/saves/${id}`),
    switchOpening: (id: string, openingId: string, expectedHeadId: string) =>
      http.put<Presentation>(`/saves/${id}/opening`, { opening_id: openingId, expected_head_id: expectedHeadId }),
    rename: (id: string, title: string) => http.patch<Save>(`/saves/${id}`, { title }),
    remove: (id: string) => http.del(`/saves/${id}`),
    timeline: (id: string, before?: string, limit = 50) =>
      http.get<Timeline>(`/saves/${id}/timeline?limit=${limit}${before ? `&before=${before}` : ''}`),
    panels: (id: string) => http.get<{ panels: PanelRender[] }>(`/saves/${id}/panels`),
    state: (id: string) => http.get<StateView>(`/saves/${id}/state`),
    submit: (id: string, input: TurnInput) => http.post<{ turn_id: string }>(`/saves/${id}/turns`, input),
    cancel: (id: string, turnId: string) => http.post<void>(`/saves/${id}/turns/${turnId}/cancel`),
    // 分支操作（技术方案 6.2）
    tree: (id: string) => http.get<BranchTree>(`/saves/${id}/tree`),
    /** 回到此处（here）/ 切换到该节点所在分支的最新进度（latest-leaf） */
    setHead: (id: string, nodeId: string, mode: 'here' | 'latest-leaf') =>
      http.post<{ head_id: string }>(`/saves/${id}/head`, { node_id: nodeId, mode }),
    /** 重新生成；带 input 时为修改玩家输入 */
    revise: (id: string, nodeId: string, instructions: string, expectedHead: string) =>
      http.post<{ turn_id: string }>(`/saves/${id}/nodes/${nodeId}/revise`, { instructions, expected_head: expectedHead }),
    requestChoices: (id: string, nodeId: string, expectedHead: string) =>
      http.post<{ turn_id: string }>(`/saves/${id}/nodes/${nodeId}/choices`, { expected_head: expectedHead }),
    regenerate: (id: string, nodeId: string, input?: TurnInput) =>
      http.post<{ turn_id: string }>(`/saves/${id}/nodes/${nodeId}/regenerate`, input ? { input } : {}),
    rewrite: (id: string, nodeId: string, text: string) =>
      http.post<{ node_id: string; head_id: string }>(`/saves/${id}/nodes/${nodeId}/rewrite`, { text }),
    discard: (id: string, nodeId: string) => http.post<{ head_id: string }>(`/saves/${id}/nodes/${nodeId}/discard`),
    deleteBranch: (id: string, nodeId: string) => http.del(`/saves/${id}/branches/${nodeId}`),
    copy: (id: string, nodeId: string) => http.post<Save>(`/saves/${id}/nodes/${nodeId}/copy`),
    compact: (id: string) => http.post<void>(`/saves/${id}/compact`),
    // 记忆与状态（修改产生编辑节点，下一回合告知执笔者）
    memory: (id: string) => http.get<MemoryView>(`/saves/${id}/memory`),
    memoryFile: (id: string, path: string) => http.get<MemoryFile>(`/saves/${id}/memory/${memoryPath(path)}`),
    putMemory: (id: string, path: string, content: string) =>
      http.put<{ node_id?: string }>(`/saves/${id}/memory/${memoryPath(path)}`, { content }),
    deleteMemory: (id: string, path: string) => http.del(`/saves/${id}/memory/${memoryPath(path)}`),
    patchState: (id: string, ops: StateOp[]) => http.patch<{ changes: StateChange[] }>(`/saves/${id}/state`, { ops }),
  },
  uploads: {
    /** 上传图片（表单字段 file），返回引用；提交回合时放进 attachments */
    create: (form: FormData, opts?: UploadOptions) => http.upload<Upload>('/uploads', form, opts),
  },
  providers: {
    list: () => http.get<ProviderView[]>('/providers'),
    create: (input: ProviderInput) => http.post<ProviderView>('/providers', input),
    update: (id: string, input: ProviderInput) => http.patch<ProviderView>(`/providers/${id}`, input),
    remove: (id: string) => http.del(`/providers/${id}`),
    /** 连接测试：model_id 为已添加的模型，model 为尚未添加的模型名；都省略时用第一个模型 */
    test: (id: string, body: { model_id?: string; model?: string } = {}) => http.post<TestReport>(`/providers/${id}/test`, body),
    remoteModels: (id: string) => http.get<RemoteModel[]>(`/providers/${id}/remote-models`),
  },
  usage: {
    summary: (f: UsageFilter = {}) => {
      const q = new URLSearchParams(Object.entries(f).filter((kv): kv is [string, string] => !!kv[1])).toString()
      return http.get<UsageSummary>(`/usage${q ? `?${q}` : ''}`)
    },
  },
  models: {
    preset: (id: string) => http.get<ModelPreset | null>(`/models/${id}/preset`),
    list: () => http.get<Model[]>('/models'),
    create: (input: ModelInput) => http.post<Model>('/models', input),
    update: (id: string, input: ModelInput) => http.patch<Model>(`/models/${id}`, input),
    remove: (id: string) => http.del(`/models/${id}`),
  },
}

/** 记忆文件在接口路径中的写法：去掉开头的 /memory/，各段分别编码。 */
function memoryPath(path: string): string {
  return path
    .replace(/^\/?memory\//, '')
    .split('/')
    .map(encodeURIComponent)
    .join('/')
}

/** 上传图片的显示地址（w：缩略图的长边，像素）。 */
export function uploadURL(hash: string, w?: number): string {
  return `/api/v1/uploads/${hash.replace(/^sha256:/, '')}${w ? `?w=${w}` : ''}`
}
