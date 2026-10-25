import type { Model, ProviderView } from '@/api/types'

/** 模型与供应商一起显示，同名模型仍可区分。 */
export function modelLabel(model: Model, providers: ProviderView[] = []) {
  const provider = providers.find((p) => p.id === model.provider_id)
  return `${model.display_name || model.name} · ${provider?.name || model.provider_id}`
}
