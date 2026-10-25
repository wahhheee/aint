/**
 * 开局设置（《04-交互与界面设计》7）：存档名称、作品的开局表单、执笔参数。
 */
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'

import { errorMessage, isApiError } from '@/api/errors'
import { useCreateSave, useModels, useSettings } from '@/api/queries'
import type { FormField, Pace, Reasoning, SaveSettings, WorkDetail } from '@/api/types'
import { AdvancedPlaySettings } from './AdvancedPlaySettings'
import { confirmAction } from '@/components/common/confirm'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

const PACES: Pace[] = ['short', 'medium', 'long']
const REASONING: Reasoning[] = ['off', 'low', 'medium', 'high']

interface Props {
  work: WorkDetail
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function NewGameDialog({ work, open, onOpenChange }: Props) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const settings = useSettings()
  const models = useModels()
  const create = useCreateSave()

  const fields = work.new_game.fields ?? []
  const [advanced, setAdvanced] = useState<Partial<SaveSettings>>({})
  const [modelId, setModelId] = useState('')
  const [helpers, setHelpers] = useState<boolean | undefined>()
  const [title, setTitle] = useState('')
  const [form, setForm] = useState<Record<string, unknown>>(() => Object.fromEntries(fields.map((f) => [f.key, f.default ?? emptyOf(f)])))
  const [pace, setPace] = useState<Pace>(work.narrator.length || 'medium')
  const [reasoning, setReasoning] = useState<Reasoning>(work.narrator.reasoning ?? settings.data?.play.reasoning ?? 'low')

  const model = models.data?.find((m) => m.id === (modelId || settings.data?.models.narrator)) ?? models.data?.[0]
  const noModel = models.data !== undefined && models.data.length === 0

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (helpers && !settings.data?.play.helpers && !(await confirmAction(t('background.helperCost')))) return
    create.mutate(
      {
        work_id: work.id,
        title,
        form,
        settings: { ...advanced, pace, reasoning, narrator_model: model?.id, helpers: helpers ?? settings.data?.play.helpers ?? false },
      },
      { onSuccess: (sv) => navigate(`/play/${sv.id}`) },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="title-book">{t('newGame.title')}</DialogTitle>
          <p className="text-[13px] text-ink-muted">{work.title}</p>
          <DialogDescription className="sr-only">{t('newGame.title')}</DialogDescription>
        </DialogHeader>

        <form onSubmit={submit} className="space-y-8">
          <div className="space-y-2">
            <Label htmlFor="save-title">{t('newGame.saveName')}</Label>
            <Input
              id="save-title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder={t('newGame.saveNamePlaceholder')}
            />
          </div>

          {fields.length > 0 && (
            <fieldset className="space-y-5">
              <legend className="mb-4 w-full border-b border-rule pb-2 text-[13px] font-medium text-ink-muted">
                {t('newGame.workSection')}
              </legend>
              {fields.map((f) => (
                <FieldInput key={f.key} field={f} value={form[f.key]} onChange={(v) => setForm((s) => ({ ...s, [f.key]: v }))} />
              ))}
            </fieldset>
          )}

          <fieldset className="space-y-5">
            <legend className="mb-4 w-full border-b border-rule pb-2 text-[13px] font-medium text-ink-muted">
              {t('newGame.runSection')}
            </legend>
            <div className="flex items-baseline justify-between gap-4">
              <Label htmlFor="new-model">{t('newGame.model')}</Label>
              {noModel ? (
                <span className="text-sm text-warn">
                  {t('newGame.noModel')}
                  <Link to="/settings/providers" className="ml-2 underline underline-offset-4">
                    {t('newGame.goSettings')}
                  </Link>
                </span>
              ) : (
                <select
                  id="new-model"
                  className="h-9 max-w-[65%] rounded-lg border border-rule bg-paper px-2 text-sm"
                  value={model?.id ?? ''}
                  onChange={(e) => setModelId(e.target.value)}
                >
                  {models.data?.map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.display_name || m.name}
                    </option>
                  ))}
                </select>
              )}
            </div>
            {/* 作品需要看图而主笔模型不能看图（《02-技术方案》15.1）：提示，但不阻止开始 */}
            {model && !model.caps.vision && work.requires.vision !== 'none' && (
              <p className="-mt-2 text-[13px] leading-6 text-warn">
                {t(work.requires.vision === 'required' ? 'newGame.visionRequired' : 'newGame.visionPreferred')}
              </p>
            )}
            <div className="flex items-center justify-between gap-4">
              <Label>{t('newGame.pace')}</Label>
              <ToggleGroup type="single" variant="segmented" size="sm" value={pace} onValueChange={(v) => v && setPace(v)}>
                {PACES.map((p) => (
                  <ToggleGroupItem key={p} value={p} aria-label={t(`paceHint.${p as 'short'}`)}>
                    {t(`pace.${p as 'short'}`)}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </div>
            <div className="flex items-center justify-between gap-4">
              <Label>
                {t('newGame.reasoning')}
                {work.narrator.reasoning && (
                  <span className="ml-2 text-xs font-normal text-ink-faint">
                    {t('newGame.recommended')}：{t(`reasoning.${work.narrator.reasoning}`)}
                  </span>
                )}
              </Label>
              <ToggleGroup
                type="single"
                variant="outline"
                size="sm"
                value={reasoning}
                onValueChange={(v) => v && setReasoning(v as Reasoning)}
              >
                {REASONING.map((r) => (
                  <ToggleGroupItem key={r} value={r}>
                    {t(`reasoning.${r}`)}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </div>
            <label className="flex items-center justify-between text-sm">
              <span>{t('background.helpers')}</span>
              <Switch
                disabled={!settings.data?.models.light}
                checked={helpers ?? settings.data?.play.helpers ?? false}
                onCheckedChange={setHelpers}
              />
            </label>
            <AdvancedPlaySettings
              value={{ ...advanced, pace }}
              onChange={(v) => {
                if (v.pace) setPace(v.pace)
                setAdvanced((s) => ({ ...s, ...v }))
              }}
            />
          </fieldset>

          {create.isError && (
            <p role="alert" className="rounded-xl border border-danger/30 bg-danger/8 px-4 py-3 text-[13px] leading-6 text-ink">
              {errorMessage(create.error)}
              {isApiError(create.error, 'model_missing') && (
                <Link to="/settings/providers" className="ml-2 underline underline-offset-4">
                  {t('newGame.goSettings')}
                </Link>
              )}
            </p>
          )}

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={create.isPending || noModel} className="min-w-24">
              {t('newGame.start')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function emptyOf(f: FormField): unknown {
  switch (f.type) {
    case 'number':
      return f.min ?? 0
    case 'toggle':
      return false
    case 'multiselect':
      return []
    default:
      return ''
  }
}

/** 按字段类型渲染开局表单的输入控件。 */
function FieldInput({ field: f, value, onChange }: { field: FormField; value: unknown; onChange: (v: unknown) => void }) {
  const id = `field-${f.key}`
  const label = (
    <Label htmlFor={id}>
      {f.label}
      {f.required && <span className="text-danger">*</span>}
    </Label>
  )
  const hint = f.hint && <p className="text-xs text-ink-faint">{f.hint}</p>

  switch (f.type) {
    case 'textarea':
      return (
        <div className="space-y-2">
          {label}
          <Textarea id={id} value={String(value ?? '')} maxLength={f.max_length} onChange={(e) => onChange(e.target.value)} />
          {hint}
        </div>
      )
    case 'number':
      return (
        <div className="space-y-2">
          {label}
          <Input
            id={id}
            type="number"
            min={f.min}
            max={f.max}
            value={Number(value ?? 0)}
            onChange={(e) => onChange(Number(e.target.value))}
          />
          {hint}
        </div>
      )
    case 'toggle':
      return (
        <div className="flex items-center justify-between gap-4">
          {label}
          <Switch id={id} checked={Boolean(value)} onCheckedChange={onChange} />
        </div>
      )
    case 'select':
      return (
        <div className="space-y-2.5">
          {label}
          <RadioGroup value={String(value ?? '')} onValueChange={onChange} className="flex flex-wrap gap-x-6 gap-y-2">
            {(f.options ?? []).map((o) => (
              <label key={o} className="flex items-center gap-2 text-sm">
                <RadioGroupItem value={o} />
                {o}
              </label>
            ))}
          </RadioGroup>
          {hint}
        </div>
      )
    case 'multiselect':
      return (
        <div className="space-y-2.5">
          {label}
          <ToggleGroup
            type="multiple"
            variant="outline"
            size="sm"
            value={(value as string[]) ?? []}
            onValueChange={onChange}
            className="flex-wrap"
          >
            {(f.options ?? []).map((o) => (
              <ToggleGroupItem key={o} value={o}>
                {o}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          {hint}
        </div>
      )
    default:
      return (
        <div className="space-y-2">
          {label}
          <Input id={id} value={String(value ?? '')} maxLength={f.max_length} onChange={(e) => onChange(e.target.value)} />
          {hint}
        </div>
      )
  }
}
