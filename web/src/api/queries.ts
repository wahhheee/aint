/**
 * TanStack Query 封装：查询键集中定义（失效与 SSE 推送更新时要用同一个键），组件只用这里的 hooks。
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import type { UploadOptions } from './client'
import { api, type CreateSaveParams } from './endpoints'
import type { DeepPartial, ModelInput, ProviderInput, Settings } from './types'

/** 查询键工厂。 */
export const qk = {
  system: ['system'] as const,
  settings: ['settings'] as const,
  works: ['works'] as const,
  work: (id: string) => ['works', id] as const,
  workSaves: (id: string) => ['works', id, 'saves'] as const,
  recentSaves: ['saves', 'recent'] as const,
  save: (id: string) => ['saves', id] as const,
  timeline: (id: string) => ['saves', id, 'timeline'] as const,
  panels: (id: string) => ['saves', id, 'panels'] as const,
  providers: ['providers'] as const,
  models: ['models'] as const,
}

export const useSystemInfo = () => useQuery({ queryKey: qk.system, queryFn: api.system.info, staleTime: Infinity })

export const useSettings = () => useQuery({ queryKey: qk.settings, queryFn: api.settings.get, staleTime: Infinity })

export function useUpdateSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (patch: DeepPartial<Settings>) => api.settings.patch(patch),
    onSuccess: (s) => qc.setQueryData(qk.settings, s),
  })
}

export const useWorks = () => useQuery({ queryKey: qk.works, queryFn: api.works.list })
export const useWork = (id: string) => useQuery({ queryKey: qk.work(id), queryFn: () => api.works.get(id) })
export const useWorkSaves = (id: string) => useQuery({ queryKey: qk.workSaves(id), queryFn: () => api.works.saves(id) })
export const useRecentSaves = () => useQuery({ queryKey: qk.recentSaves, queryFn: () => api.saves.recent(6) })

/**
 * 导入作品（两段式）：预览（上传、解析与校验，不写入）→ 确认（写入，刷新书架）；放弃时删除暂存。
 * 状态流转见 features/library/useWorkImport.ts。
 */
export function useImportWork() {
  const qc = useQueryClient()
  return {
    preview: useMutation({
      mutationFn: ({ form, ...opts }: { form: FormData } & UploadOptions) => api.works.importPreview(form, opts),
    }),
    confirm: useMutation({
      mutationFn: (task: string) => api.works.importConfirm(task),
      // 作品列表、详情与存档都以 ['works'] 开头，一并刷新
      onSuccess: () => void qc.invalidateQueries({ queryKey: qk.works }),
    }),
    cancel: useMutation({ mutationFn: (task: string) => api.works.importCancel(task) }),
  }
}

export function useCreateSave() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (p: CreateSaveParams) => api.saves.create(p),
    onSuccess: (sv) => {
      void qc.invalidateQueries({ queryKey: qk.workSaves(sv.work_id) })
      void qc.invalidateQueries({ queryKey: qk.recentSaves })
      void qc.invalidateQueries({ queryKey: qk.works })
    },
  })
}

export function useRenameSave() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, title }: { id: string; title: string }) => api.saves.rename(id, title),
    onSuccess: (sv) => {
      void qc.invalidateQueries({ queryKey: qk.workSaves(sv.work_id) })
      void qc.invalidateQueries({ queryKey: qk.recentSaves })
      void qc.invalidateQueries({ queryKey: qk.save(sv.id) })
    },
  })
}

export function useDeleteSave() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id }: { id: string; workId: string }) => api.saves.remove(id),
    onSuccess: (_, { workId }) => {
      void qc.invalidateQueries({ queryKey: qk.workSaves(workId) })
      void qc.invalidateQueries({ queryKey: qk.recentSaves })
      void qc.invalidateQueries({ queryKey: qk.works })
    },
  })
}

export const usePresentation = (id: string) => useQuery({ queryKey: qk.save(id), queryFn: () => api.saves.get(id), staleTime: 30_000 })

export const useTimeline = (id: string) =>
  useQuery({ queryKey: qk.timeline(id), queryFn: () => api.saves.timeline(id), staleTime: Infinity })

export const usePanels = (id: string) => useQuery({ queryKey: qk.panels(id), queryFn: () => api.saves.panels(id), staleTime: Infinity })

export const useProviders = () => useQuery({ queryKey: qk.providers, queryFn: api.providers.list })
export const useModels = () => useQuery({ queryKey: qk.models, queryFn: api.models.list })

/** 供应商与模型的增删改：成功后刷新两个列表（删除供应商会级联删除模型）。 */
export function useProviderMutations() {
  const qc = useQueryClient()
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: qk.providers })
    void qc.invalidateQueries({ queryKey: qk.models })
  }
  return {
    createProvider: useMutation({ mutationFn: api.providers.create, onSuccess: refresh }),
    updateProvider: useMutation({
      mutationFn: ({ id, input }: { id: string; input: ProviderInput }) => api.providers.update(id, input),
      onSuccess: refresh,
    }),
    removeProvider: useMutation({ mutationFn: api.providers.remove, onSuccess: refresh }),
    testProvider: useMutation({
      mutationFn: ({ id, model_id, model }: { id: string; model_id?: string; model?: string }) =>
        api.providers.test(id, { model_id, model }),
      onSettled: refresh,
    }),
    createModel: useMutation({ mutationFn: api.models.create, onSuccess: refresh }),
    updateModel: useMutation({
      mutationFn: ({ id, input }: { id: string; input: ModelInput }) => api.models.update(id, input),
      onSuccess: refresh,
    }),
    removeModel: useMutation({ mutationFn: api.models.remove, onSuccess: refresh }),
  }
}
