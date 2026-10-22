/**
 * 后端接口（《02-技术方案》18.2）。组件不直接调用这里，而是通过 queries.ts 的 hooks。
 */
import { http, type UploadOptions } from './client'
import type {
  DeepPartial,
  ImportPreview,
  ImportResult,
  Model,
  PanelRender,
  Presentation,
  ProviderView,
  Save,
  SaveSettings,
  Settings,
  SystemInfo,
  Timeline,
  TurnInput,
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
    submit: (id: string, input: TurnInput) => http.post<{ turn_id: string }>(`/saves/${id}/turns`, input),
    cancel: (id: string, turnId: string) => http.post<void>(`/saves/${id}/turns/${turnId}/cancel`),
  },
  providers: {
    list: () => http.get<ProviderView[]>('/providers'),
    models: () => http.get<Model[]>('/models'),
  },
}
