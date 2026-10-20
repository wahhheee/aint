/**
 * 后端 API 的数据结构（与 Go 结构体的 json 标签一一对应）。
 *
 * 约定：字段名保持后端的 snake_case，不做转换——排查问题时，浏览器网络面板、日志、Go 代码里看到的是同一个名字。
 * 修改后端结构体时同步这里；对应关系写在每个类型的注释里。
 */

/** buildinfo.Info */
export interface BuildInfo {
  version: string
  commit: string
  date: string
  go_version: string
  platform: string
  modified: boolean
}

/** server.SystemInfo（GET /system/info） */
export interface SystemInfo {
  build: BuildInfo
  instance_id: string
  data_dir: string
  log_dir: string
  listen: string
  fake_llm: boolean
  trace_llm: boolean
  prompt_version: string
  started_at: number
}

// ---------------------------------------------------------------- 设置

/** settings.Settings（GET/PATCH /settings） */
export interface Settings {
  theme: ThemePreference
  use_work_theme: boolean
  reading: ReadingSettings
  audio: { master: number; bgm: number; sfx: number; muted: boolean }
  models: { narrator: string; light: string }
  play: { pace: Pace; reasoning: Reasoning; cache_ttl: '5m' | '1h'; title_on_exit: 'auto' | 'ask' | 'off'; helpers: boolean }
  onboarded: boolean
  /** 开发者选项（SET-07，D-31）：显示检查器、回合检查、完整状态与状态编辑 */
  developer: boolean
}

export type ThemePreference = 'system' | 'light' | 'dark'
export type Pace = 'short' | 'medium' | 'long' | (string & {})
export type Reasoning = 'off' | 'low' | 'medium' | 'high'

export interface ReadingSettings {
  font_family: 'work' | 'serif' | 'sans'
  font_size: number
  line_height: number
  paragraph_spacing: number
  max_width: number
  indent: 'work' | 'on' | 'off'
  typewriter: 'instant' | 'normal' | 'slow'
  reduce_motion: 'system' | 'on' | 'off'
  stage_intensity: number
  tool_hints: boolean
}

/** 递归的部分更新类型（PATCH /settings 只需提交变化的字段） */
export type DeepPartial<T> = { [K in keyof T]?: T[K] extends object ? DeepPartial<T[K]> : T[K] }

// ---------------------------------------------------------------- 作品

/** work.Source */
export interface WorkSource {
  type: 'zip' | 'dir' | 'github' | 'url' | 'dev' | 'builtin'
  name?: string
  path?: string
  repo?: string
  ref?: string
  commit?: string
  subdir?: string
  url?: string
}

/** work.VersionInfo */
export interface VersionInfo {
  id: string
  version: string
  source: WorkSource
  is_dev: boolean
  imported_at: number
  errors: number
  warnings: number
}

export interface Author {
  name: string
  url?: string
}

/** work.Summary（GET /works） */
export interface WorkSummary {
  id: string
  title: string
  subtitle?: string
  description?: string
  tags: string[]
  content_rating?: 'general' | 'teen' | 'mature'
  authors: Author[]
  latest: VersionInfo
  save_count: number
  updated_at: number
}

/** work.FormField（开局表单） */
export interface FormField {
  key: string
  label: string
  type: 'text' | 'textarea' | 'number' | 'select' | 'multiselect' | 'toggle'
  default?: unknown
  options?: string[]
  min?: number
  max?: number
  max_length?: number
  hint?: string
  required?: boolean
}

/** work.Issue */
export interface Issue {
  level: 'error' | 'warning' | 'info'
  code: string
  path?: string
  line?: number
  message: string
  hint?: string
}

/** work.Detail（GET /works/{id}） */
export interface WorkDetail extends WorkSummary {
  license?: string
  requires: { vision: 'none' | 'preferred' | 'required' }
  narrator: { instructions: string; length: Pace; reasoning?: Reasoning }
  new_game: { fields?: FormField[] }
  changelog?: string
  readme_html?: string
  versions: VersionInfo[]
  report: { issues: Issue[] }
}

/** work.TagDef（作品样式标签） */
export interface TagDef {
  name: string
  description: string
  style: {
    color?: string
    dark_color?: string
    background?: string
    weight?: 'normal' | 'bold'
    italic?: boolean
    underline?: 'none' | 'solid' | 'dotted' | 'wavy'
    size?: number
    font?: string
    letter_spacing?: number
    opacity?: number
    effect?: string
  }
  icon?: string
  class?: string
  tooltip?: string
}

/** work.Command */
export interface Command {
  name: string
  title: string
  description: string
  args?: { name: string; hint?: string; options?: string[]; required?: boolean }[]
  present: ('slash' | 'mode' | 'button')[]
  mode_label?: string
  mode_style?: string
  icon?: string
  confirm?: string
  ai_visible: boolean
}

// ---------------------------------------------------------------- 存档与回合

/** save.Settings */
export interface SaveSettings {
  narrator_model: string
  pace: Pace
  reasoning: Reasoning
  helpers: boolean
  cache_ttl?: '5m' | '1h'
}

/** save.Save */
export interface Save {
  id: string
  work_id: string
  work_version_id: string
  work_title: string
  title: string
  title_source: '' | 'generated' | 'manual'
  head_node_id: string
  turn_count: number
  settings: SaveSettings
  created_at: number
  updated_at: number
  last_played_at: number
}

export type InputKind = 'free' | 'choice' | 'command' | 'continue' | 'opening' | 'ooc'

/** save.TurnInput（POST /saves/{id}/turns） */
export interface TurnInput {
  kind: InputKind
  text?: string
  mode?: string
  choice_id?: string
  choice_label?: string
  command?: { name: string; args?: Record<string, string> }
  attachments?: string[]
  pace?: Pace
}

/** save.ChoiceItem */
export interface ChoiceItem {
  id: string
  label: string
  detail?: string
  style?: string
  command?: string
}

/** save.DisplayItem：正文流中的一个显示项（按 t 区分） */
export type DisplayItem =
  | { t: 'input'; kind: InputKind; mode?: string; text?: string; attachments?: string[] }
  | { t: 'text'; markup: string }
  | { t: 'tools'; summary: string }
  | { t: 'dice'; expr: string; rolls: number[]; total: number }
  | { t: 'image'; source: 'work' | 'upload' | 'generated'; asset: string; status: 'pending' | 'ready' | 'failed' }
  | { t: 'choices'; prompt?: string; items: ChoiceItem[]; free?: boolean; selected?: string }
  | { t: 'notice'; level: 'info' | 'warn' | 'error'; text: string }

/** save.TimelineNode */
export interface TimelineNode {
  id: string
  parent_id: string
  kind: 'root' | 'turn' | 'compaction' | 'edit'
  status: 'committed' | 'interrupted'
  turn_no: number
  created_at: number
  display: DisplayItem[]
  stop?: string
  error?: string
  versions: { index: number; count: number }
}

/** save.Timeline（GET /saves/{id}/timeline） */
export interface Timeline {
  save_id: string
  head_id: string
  nodes: TimelineNode[]
  has_more: boolean
}

/** runtime.Presentation（GET /saves/{id}）：阅读页所需的全部呈现信息 */
export interface Presentation {
  save: Save
  running_turn_id?: string
  needs_opening: boolean
  /** 作品来自开发目录：不论开发者选项如何都显示开发者工具（D-31） */
  dev_work: boolean
  model: { id: string; name: string; display_name: string; vision: boolean; context_window: number }
  ui: {
    features: { free_input: boolean; ooc: boolean; dice: boolean; timed_choices: boolean }
    builtin_tags: string[]
    tags: TagDef[]
    panels: PanelMeta[]
    command_bar: string[]
    commands: Command[]
    characters: Character[]
    assets: AssetRef[]
    pace: Pace
  }
}

/** runtime.AssetRef：作品素材 */
export interface AssetRef {
  id: string
  kind: 'image' | 'audio' | 'font'
  role: 'background' | 'cg' | 'avatar' | 'bgm' | 'sfx' | 'font' | 'reference'
  url: string
  description?: string
}

/** runtime.PanelMeta */
export interface PanelMeta {
  id: string
  title?: string
  place: 'sidebar' | 'topbar' | 'drawer' | 'float'
  order: number
  collapsed?: boolean
}

/** runtime.Character */
export interface Character {
  id: string
  name: string
  aliases?: string[]
  color?: string
  avatar?: string
}

/** event.PanelRender */
export interface PanelRender {
  id: string
  html: string
  hash: string
}

// ---------------------------------------------------------------- 模型

/** provider.ProviderView */
export interface ProviderView {
  id: string
  name: string
  protocol: 'openai-responses' | 'anthropic' | 'gemini' | 'fake'
  base_url: string
  headers?: Record<string, string>
  key_masked: string
  created_at: number
}

/** llm.Model */
export interface Model {
  id: string
  provider_id: string
  name: string
  display_name?: string
  context_window: number
  max_output: number
  caps: { vision: boolean; tools: boolean; reasoning: string; cache: string; image_max_edge: number }
}

/** llm.Usage */
export interface Usage {
  input_uncached: number
  cache_read: number
  cache_write: number
  output: number
  reasoning: number
}
