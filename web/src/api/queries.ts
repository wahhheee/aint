/**
 * TanStack Query 封装：查询键集中定义（失效与 SSE 推送更新时要用同一个键），组件只用这里的 hooks。
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'

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
  tree: (id: string) => ['saves', id, 'tree'] as const,
  memory: (id: string) => ['saves', id, 'memory'] as const,
  memoryFile: (id: string, path: string) => ['saves', id, 'memory', path] as const,
  state: (id: string) => ['saves', id, 'state'] as const,
  debug: (id: string) => ['debug', id] as const,
  debugCall: (id: string, call: string) => ['debug', id, 'call', call] as const,
  /** 存档累计用量；带 nodeId 时是该节点（回合）的用量 */
  usage: (id: string, nodeId?: string) => (nodeId ? (['saves', id, 'usage', nodeId] as const) : (['saves', id, 'usage'] as const)),
  providers: ['providers'] as const,
  models: ['models'] as const,
}

export function useDebugStatus(id: string) {
  return useQuery({
    queryKey: qk.debug(id),
    queryFn: () => api.debug.status(id),
    refetchInterval: (q) => (q.state.data?.enabled ? 1000 : 5000),
  })
}

export function useDebugActions(id: string) {
  const qc = useQueryClient()
  return {
    toggle: useMutation({
      mutationFn: (enabled: boolean) => api.debug.set(id, enabled),
      onSuccess: (s) => qc.setQueryData(qk.debug(id), s),
    }),
    clear: useMutation({
      mutationFn: () => api.debug.clear(id),
      onSuccess: (s) => {
        qc.removeQueries({ queryKey: [...qk.debug(id), 'call'] })
        qc.setQueryData(qk.debug(id), s)
      },
    }),
  }
}

export function useDebugCall(id: string, call: string, revision: number, enabled: boolean) {
  const query = useQuery({
    queryKey: qk.debugCall(id, call),
    queryFn: () => api.debug.call(id, call),
    enabled: enabled && !!call,
    gcTime: 0,
    retry: false,
  })
  const { refetch } = query
  const currentRevision = query.data?.revision
  useEffect(() => {
    if (enabled && call && currentRevision !== undefined && currentRevision < revision) void refetch()
  }, [call, enabled, revision, currentRevision, refetch])
  return query
}

export function useDebugFile(id: string, call: string, phase: string, path: string, offset: number, blob: string) {
  return useQuery({
    queryKey: [...qk.debugCall(id, call), 'file', phase, path, offset, blob],
    queryFn: () => api.debug.file(id, call, phase, path, offset),
    enabled: !!path,
    gcTime: 0,
    retry: false,
  })
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

/** 复制为新存档：从 nodeId（通常是 head）复制出一个独立的存档。 */
export function useCopySave() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, nodeId }: { id: string; nodeId: string }) => api.saves.copy(id, nodeId),
    onSuccess: (sv) => {
      void qc.invalidateQueries({ queryKey: qk.workSaves(sv.work_id) })
      void qc.invalidateQueries({ queryKey: qk.recentSaves })
      void qc.invalidateQueries({ queryKey: qk.works })
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

export const useBranchTree = (id: string, enabled = true) => useQuery({ queryKey: qk.tree(id), queryFn: () => api.saves.tree(id), enabled })

export const useMemory = (id: string, enabled = true) => useQuery({ queryKey: qk.memory(id), queryFn: () => api.saves.memory(id), enabled })

export const useMemoryFile = (id: string, path: string | null) =>
  useQuery({ queryKey: qk.memoryFile(id, path ?? ''), queryFn: () => api.saves.memoryFile(id, path!), enabled: !!path })

export const useSaveState = (id: string, enabled = true) =>
  useQuery({ queryKey: qk.state(id), queryFn: () => api.saves.state(id), enabled })

/** 存档累计用量（回合与整理提交后失效）。 */
export const useSaveUsage = (id: string, enabled = true) =>
  useQuery({ queryKey: qk.usage(id), queryFn: () => api.usage.summary({ save_id: id }), enabled })

/** 某个已提交回合的用量（提交后不再变化）。 */
export const useNodeUsage = (id: string, nodeId: string | undefined, enabled = true) =>
  useQuery({
    queryKey: qk.usage(id, nodeId),
    queryFn: () => api.usage.summary({ save_id: id, node_id: nodeId }),
    enabled: enabled && !!nodeId,
    staleTime: Infinity,
  })

export const useProviders = () => useQuery({ queryKey: qk.providers, queryFn: api.providers.list })
export const useModels = () => useQuery({ queryKey: qk.models, queryFn: api.models.list })
export const useModelPreset = (id: string) => useQuery({ queryKey: [...qk.models, id, 'preset'], queryFn: () => api.models.preset(id) })

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
