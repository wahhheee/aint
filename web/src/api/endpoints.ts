/**
 * 后端接口（《02-技术方案》18.2）。组件不直接调用这里，而是通过 queries.ts 的 hooks。
 */
import { http, type UploadOptions } from './client'
import type {
  DeepPartial,
  ImportPreview,
  ImportResult,
  Model,
  ModelInput,
  PanelRender,
  Presentation,
  ProviderInput,
  ProviderView,
  RemoteModel,
  Report,
  Save,
  SaveSettings,
  Settings,
  StateView,
  SystemInfo,
  TestReport,
  Timeline,
  TurnInput,
  Upload,
  WorkDetail,
  WorkSummary,
} from './types'

export interface CreateSaveParams {
  work_id: string
  version_id?: string
  title?: string
  form?: Record<string, unknown>
  settings: Partial<SaveSettings>
}

export const api = {
  system: {
    info: () => http.get<SystemInfo>('/system/info'),
  },
  settings: {
    get: () => http.get<Settings>('/settings'),
    patch: (patch: DeepPartial<Settings>) => http.patch<Settings>('/settings', patch),
  },
  works: {
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
    recent: (limit = 6) => http.get<Save[]>(`/saves?limit=${limit}`),
    create: (p: CreateSaveParams) => http.post<Save>('/saves', p),
    get: (id: string) => http.get<Presentation>(`/saves/${id}`),
    rename: (id: string, title: string) => http.patch<Save>(`/saves/${id}`, { title }),
    remove: (id: string) => http.del(`/saves/${id}`),
    timeline: (id: string, before?: string, limit = 50) =>
      http.get<Timeline>(`/saves/${id}/timeline?limit=${limit}${before ? `&before=${before}` : ''}`),
    panels: (id: string) => http.get<{ panels: PanelRender[] }>(`/saves/${id}/panels`),
    state: (id: string) => http.get<StateView>(`/saves/${id}/state`),
    submit: (id: string, input: TurnInput) => http.post<{ turn_id: string }>(`/saves/${id}/turns`, input),
    cancel: (id: string, turnId: string) => http.post<void>(`/saves/${id}/turns/${turnId}/cancel`),
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
  models: {
    list: () => http.get<Model[]>('/models'),
    create: (input: ModelInput) => http.post<Model>('/models', input),
    update: (id: string, input: ModelInput) => http.patch<Model>(`/models/${id}`, input),
    remove: (id: string) => http.del(`/models/${id}`),
  },
}

/** 上传图片的显示地址（w：缩略图的长边，像素）。 */
export function uploadURL(hash: string, w?: number): string {
  return `/api/v1/uploads/${hash.replace(/^sha256:/, '')}${w ? `?w=${w}` : ''}`
}
