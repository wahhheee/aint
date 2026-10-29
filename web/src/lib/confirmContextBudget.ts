import { confirmAction } from '@/components/common/confirm'
import i18n from '@/i18n'

/** 超过 256K 的预算变更须确认费用提示；沿用当前预算无需重复确认。 */
export async function confirmContextBudget(budget: number, currentBudget?: number): Promise<boolean> {
  return budget <= 256_000 || budget === currentBudget || confirmAction(i18n.t('settings.context.highBudgetWarning'))
}
