/**
 * 后端接口（《02-技术方案》18.2）。组件不直接调用这里，而是通过 queries.ts 的 hooks。
 */
import { http } from './client'
import type {
  DeepPartial,
  Model,
  ModelInput,
  PanelRender,
  Presentation,
  ProviderInput,
  ProviderView,
  RemoteModel,
  Save,
  SaveSettings,
  Settings,
  SystemInfo,
  TestReport,
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
    // TODO(M1-4)：两段式导入（解析校验 → 确认）
    importPreview: (body: unknown) => http.post<unknown>('/works/import', body),
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
