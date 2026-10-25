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
  turn_defaults: Pick<ContextSettings, 'max_calls' | 'max_tool_calls' | 'max_output'>
}

// ---------------------------------------------------------------- 设置

/** settings.Settings（GET/PATCH /settings） */
export interface Settings {
  theme: ThemePreference
  use_work_theme: boolean
  reading: ReadingSettings
  audio: { master: number; bgm: number; sfx: number; muted: boolean }
  models: { narrator: string; light: string }
  play: { pace: Pace; reasoning: Reasoning; cache_ttl: 'short' | 'long'; title_on_exit: 'auto' | 'ask' | 'off'; helpers: boolean }
  /** 上下文整理的默认参数（存档设置可覆盖） */
  context: ContextSettings
  onboarded: boolean
  /** 开发者选项（SET-07，D-31）：显示检查器、回合检查、完整状态与状态编辑 */
  developer: boolean
}

export type ThemePreference = 'system' | 'light' | 'dark'
export type Pace = 'short' | 'medium' | 'long' | (string & {})
export type Reasoning = 'off' | 'low' | 'medium' | 'high'

export interface ReadingSettings {
  timed_choices: boolean
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

/** settings.Context：上下文整理 */
export interface ContextSettings {
  max_calls: number
  max_tool_calls: number
  max_output: number
  /** 工作上下文预算（token）：上下文达到它（且不超过模型窗口 − 预留）时整理 */
  budget: number
  /** 整理后原样保留的最近回合（token） */
  keep_recent: number
  /** inline：主笔在原上下文中整理（复用缓存）；separate：轻量模型单独整理 */
  mode: CompactMode
}

export type CompactMode = 'inline' | 'separate'

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
  /** 作品提供封面图时的地址；没有时用标题生成排版封面 */
  cover_url?: string
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

/** work.Report：校验报告 */
export interface Report {
  issues: Issue[]
  /** 常驻部分的 token 估算（有 error 时没有） */
  tokens?: TokenEstimate
}

/** work.TokenEstimate：常驻部分（每回合都会发送、可被缓存的前缀）各层的 token 估算 */
export interface TokenEstimate {
  platform: number
  work: number
  index?: number
  tools: number
  resident: number
  images?: number
  total: number
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
  report: Report
}

/** work.ImportPreview（POST /works/import）：导入第一步的结果，解析与校验，尚未写入 */
export interface ImportPreview {
  /** 确认导入时使用；报告有 error 时没有（不能导入） */
  task?: string
  /** 毫秒时间戳：过期后需要重新选择作品 */
  expires_at?: number
  source: WorkSource
  work: ImportWork
  tree_hash: string
  /** 文件数与总字节数（不含被忽略的文件） */
  files: number
  size: number
  /** 书架上已有同 ID 作品时 */
  existing?: ImportExisting
  report: Report
}

/** work.ImportWork：待导入作品的摘要（来自 work.yaml；清单有错误时可能不完整） */
export interface ImportWork {
  id: string
  title: string
  subtitle?: string
  version: string
  description?: string
  tags: string[]
  content_rating?: 'general' | 'teen' | 'mature'
  authors: Author[]
  requires: { vision: 'none' | 'preferred' | 'required' }
}

/** work.ImportExisting：书架上已有的同 ID 作品 */
export interface ImportExisting {
  title: string
  latest_version: string
  /** 内容完全相同（文件树哈希一致）的已有版本：确认后直接复用 */
  same_version_id?: string
  same_version?: string
}

/** work.ImportResult（POST /works/import/{task}/confirm） */
export interface ImportResult {
  work_id: string
  version_id: string
  /** false 表示同内容版本已存在，直接复用 */
  created: boolean
  report: Report
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
  max_calls?: number
  max_tool_calls?: number
  max_output?: number
  narrator_model: string
  pace: Pace
  reasoning: Reasoning
  helpers: boolean
  cache_ttl?: 'short' | 'long'
  /** 上下文整理的存档级覆盖；省略表示沿用全局设置 */
  context_budget?: number
  keep_recent?: number
  compact_mode?: CompactMode
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
  /** 分支数（叶子数） */
  branch_count: number
  /** 复制为新存档时记录的来源 */
  forked_from?: { save_id: string; node_id: string; turn_no: number; title?: string }
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
  /** 所选选项的 ID；选项附带命令时也保留，由服务端校验并补全 choice_label */
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
  | { t: 'dice'; expr: string; rolls: number[]; total: number; text?: string }
  | { t: 'image'; source: 'work' | 'upload' | 'generated'; asset: string; status: 'pending' | 'ready' | 'failed' }
  | {
      t: 'choices'
      timeout_seconds?: number
      default_choice?: string
      prompt?: string
      items: ChoiceItem[]
      free?: boolean
      selected?: string
    }
  | { t: 'notice'; level: 'info' | 'warn' | 'error'; text: string }

/**
 * save.TimelineNode：回合带显示项与版本信息；编辑节点（玩家修改了状态或记忆）带注记；整理节点只标出位置。
 * 编辑、整理节点的 turn_no 是它们所跟随的回合。
 */
export interface TimelineNode {
  id: string
  parent_id: string
  kind: 'root' | 'turn' | 'compaction' | 'edit'
  status: 'committed' | 'interrupted'
  turn_no: number
  created_at: number
  display: DisplayItem[]
  notes?: string[]
  stop?: string
  error?: string
  /** 修改正文得到的版本：原回合的节点 ID */
  edited_from?: string
  /** 同一父节点下的各版本（重新生成、修改得到）；ids 只在有多个版本时给出 */
  versions: { index: number; count: number; ids?: string[] }
  /** 整理节点：写成交接摘要的回合区间与保留原文的起点 */
  summary_from?: number
  summary_to?: number
  keep_from_turn?: number
}

/** save.Timeline（GET /saves/{id}/timeline） */
export interface Timeline {
  save_id: string
  head_id: string
  nodes: TimelineNode[]
  has_more: boolean
  /** head 之后还有内容（回到了更早的回合）：继续游玩会开启新分支 */
  has_later: boolean
}

/** save.Segment：分支视图中的一段（一串没有分叉的节点） */
export interface BranchSegment {
  start_id: string
  end_id: string
  /** 这一段及其后续中最近创建的叶子：切换到这条分支时 head 去的地方 */
  leaf_id: string
  from_turn: number
  to_turn: number
  turns: number
  summary: string
  compactions?: number
  edits?: number
  /** 当前进度（head）在这一段或其后续中 */
  current: boolean
  head_turn?: number
  head_inside?: boolean
  latest_at: number
  children?: BranchSegment[]
}

/** save.Tree（GET /saves/{id}/tree） */
export interface BranchTree {
  save_id: string
  head_id: string
  root?: BranchSegment
}

/** save.MemoryFileView */
export interface MemoryFile {
  path: string
  size: number
  /** 最后修改它的回合（0 表示来自作品的初始模板） */
  turn: number
  summary?: string
  content?: string
}

/** save.MemoryView（GET /saves/{id}/memory） */
export interface MemoryView {
  node_id: string
  turn_no: number
  files: MemoryFile[]
  total: number
  limit: number
}

/** runtime.CompactStats：一次整理的结果 */
export interface CompactStats {
  reason: 'threshold' | 'limit' | 'overflow' | 'manual' | 'upgrade'
  mode: CompactMode
  summary_from: number
  summary_to: number
  keep_from_turn: number
  summary_chars: number
  memory_writes: number
  before: number
  after: number
  calls: number
  duration_ms: number
}

/** runtime.Presentation（GET /saves/{id}）：阅读页所需的全部呈现信息 */
export interface Presentation {
  save: Save
  running_turn_id?: string
  needs_opening: boolean
  /** 作品来自开发目录：不论开发者选项如何都显示开发者工具（D-31） */
  dev_work: boolean
  /** 正在整理上下文 */
  compacting: boolean
  /** 当前上下文规模与整理阈值（顶栏用量环） */
  context: { used: number; threshold: number }
  /** 当前进度线上的整理次数 */
  compactions: number
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
    theme: WorkTheme
    pace: Pace
  }
}

/** work.Theme：作品主题（作用域限于 .work-root 的 CSS + 排版数值） */
export interface WorkTheme {
  css?: string
  reading: ThemeReading
}

/** work.ThemeReading：主题中的排版数值；缺省表示沿用平台默认 */
export interface ThemeReading {
  /** serif | sans | 作品字体的 font-family 名 */
  font?: string
  heading_font?: string
  max_width?: string
  line_height?: number
  paragraph_spacing?: string
  indent?: string
  /** 有背景图（.work-backdrop） */
  backdrop?: boolean
}

/** media.Upload（POST /uploads）：玩家上传的图片（服务端已重新编码） */
export interface Upload {
  hash: string
  mime: string
  width: number
  height: number
  size: number
  url: string
}

/** runtime.AssetRef：作品素材（作品中的引用写 ID 或路径） */
export interface AssetRef {
  id: string
  path: string
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

/** state.Op：一个状态操作（PATCH /saves/{id}/state） */
export interface StateOp {
  op: 'set' | 'inc' | 'append' | 'remove' | 'delete'
  path: string
  value?: unknown
}

/** state.Change：update_state 的一个操作实际产生的变化 */
export interface StateChange {
  op: 'set' | 'inc' | 'append' | 'remove' | 'delete'
  path: string
  /** append / remove 的元素 */
  value?: unknown
  before: unknown
  after: unknown
}

/** state.Field：状态字段定义 */
export interface StateField {
  name: string
  type: 'boolean' | 'number' | 'string' | 'list' | 'map' | 'object'
  default: unknown
  desc?: string
  items?: StateField
  values?: StateField
  fields?: Record<string, StateField>
  order?: string[]
}

/** runtime.StateView（GET /saves/{id}/state）：head 处的状态与状态定义 */
export interface StateView {
  node_id: string
  turn_no: number
  state: Record<string, unknown>
  schema: { fields: Record<string, StateField>; order: string[] }
}

/** event.PanelRender */
export interface PanelRender {
  id: string
  html: string
  hash: string
}

// ---------------------------------------------------------------- 模型

export type Protocol = 'openai-responses' | 'anthropic' | 'gemini' | 'fake'

/** provider.ProviderView（密钥只给掩码） */
export interface ProviderView {
  id: string
  name: string
  protocol: Protocol
  base_url: string
  headers?: Record<string, string>
  key_masked: string
  /** 价格倍率（中转站分组倍率，D-48）：实际费用 = 模型单价 × 倍率 */
  price_multiplier: number
  /** 最近一次连接测试的结果；为空表示没测试过 */
  test_ok?: boolean
  tested_at?: number
  created_at: number
}

/** provider.ProviderInput：修改时省略的字段保持不变（api_key 为空表示不改密钥） */
export interface ProviderInput {
  name?: string
  protocol?: Protocol
  base_url?: string
  api_key?: string
  headers?: Record<string, string>
  price_multiplier?: number
}

/** llm.Price：每百万 token 的价格；缺项表示未知（费用无法估算） */
export interface Price {
  input?: number
  cache_read?: number
  cache_write?: number
  cache_write_long?: number
  output?: number
  currency?: string
}

/** llm.Caps */
export interface Caps {
  vision: boolean
  tools: boolean
  parallel_tools: boolean
  image_in_tool_result: boolean
  reasoning: 'effort' | 'budget' | 'level' | 'none'
  reasoning_map?: Partial<Record<Reasoning, string>>
  cache: 'breakpoints' | 'implicit' | 'none'
  image_max_edge: number
}

/** llm.Model（价格为模型单价，不含供应商倍率） */
export interface Model {
  id: string
  provider_id: string
  name: string
  display_name?: string
  context_window: number
  max_output: number
  caps: Caps
  price?: Price
}

/** provider.ModelInput：添加时缺省字段按内置预设填充 */
export interface ModelPreset {
  revision: string
  context_window: number
  max_output: number
  caps: Caps
  price?: Price
}

export interface ModelInput {
  provider_id?: string
  name?: string
  display_name?: string
  context_window?: number
  max_output?: number
  caps?: Caps
  price?: Price
  clear_price?: boolean
}

/** llm.RemoteModel */
export interface RemoteModel {
  id: string
  display_name?: string
}

/** provider.TestStep */
export interface TestStep {
  name: 'list_models' | 'text' | 'tool' | 'image'
  ok: boolean
  skipped?: boolean
  message: string
  detail?: string
  latency_ms?: number
}

/** provider.TestReport */
export interface TestReport {
  ok: boolean
  model: string
  steps: TestStep[]
}

/** runtime.DebugSummary */
export interface DebugSummary {
  id: string
  turn_id: string
  turn_no: number
  model: string
  protocol: string
  purpose: string
  started: string
  duration_ms: number
  status: 'running' | 'complete' | 'error'
  stop: string
  error: string
  usage: Usage
  tool_count: number
  http_count: number
  revision: number
  truncated: boolean
}

export interface DebugStatus {
  enabled: boolean
  calls: DebugSummary[]
  dropped: number
}

export interface DebugToolCall {
  id: string
  name: string
  args?: unknown
}
export interface DebugPart {
  kind: string
  text?: string
  image?: { blob: string; mime: string }
  tool_call?: DebugToolCall
  tool_result?: { call_id: string; parts: DebugPart[]; is_error?: boolean }
}
export interface DebugSnapshot {
  state: Record<string, unknown>
  roots: string[]
  files: { path: string; size: number; summary: string; blob: string }[]
  error: string
}
export interface DebugCall extends DebugSummary {
  fragments?: { id: string; layer: string; section?: string; enabled: boolean; reason?: string; chars: number }[]
  layers?: Record<string, number>
  prefix_hash?: string
  prefix_changes?: string[]
  request?: {
    system: { text: string }[]
    messages: { role: string; parts: DebugPart[] }[]
    tools: { name: string; description: string; schema: unknown }[]
    max_output: number
    reasoning: string
    cache_key: string
    cache_ttl: string
  }
  steps: { kind: 'text' | 'reasoning' | 'tool'; at_ms: number; text: string; call?: DebugToolCall }[]
  tools: { id: string; name: string; args: unknown; result: string; is_error?: boolean; duration_ms: number }[]
  results: { call_id: string; parts: DebugPart[]; is_error?: boolean }[]
  http: {
    method: string
    url: string
    request_headers: Record<string, string[]>
    request_body: string
    status: number
    response_headers: Record<string, string[]>
    truncated: boolean
    response_body: string
    header_ms: number
    duration_ms: number
    error: string
    complete: boolean
  }[]
  before?: DebugSnapshot
  after?: DebugSnapshot
}

export interface DebugFileContent {
  path: string
  text: string
  from_char: number
  to_char: number
  total_chars: number
  image?: { blob: string; mime: string }
}

/** llm.Usage */
export interface Usage {
  input_uncached: number
  cache_read: number
  cache_write: number
  output: number
  reasoning: number
}

/** usage_log.call_kind：模型调用的用途 */
export type CallKind = 'narrate' | 'continue' | 'compact' | 'helper' | 'title' | 'probe'

/** usage.Stats：一组调用的累计用量 */
export interface UsageStats {
  usage: Usage
  /** 按货币分别累计，单位为该货币的百万分之一（D-34：不做汇率换算） */
  costs: Record<string, number>
  /** 未配置价格、无法估算费用的调用次数 */
  unpriced: number
  calls: number
}

/** usage.Summary（GET /usage）：合计，以及按调用用途、按模型的小计 */
export interface UsageSummary {
  total: UsageStats
  /** 只含出现过的用途 */
  by_kind: Partial<Record<CallKind, UsageStats>>
  /** 按调用次数从多到少；name 在模型已删除或不在模型表中时省略 */
  by_model: { model_id: string; name?: string; stats: UsageStats }[]
}
