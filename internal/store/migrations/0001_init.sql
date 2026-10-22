-- 0001：初始结构（对应《02-技术方案》4.2）
-- 约定：主键为 UUID v7 文本（works.id 除外，它是 work.yaml 中的作品 ID）；
--       时间列为 Unix 毫秒；JSON 列与 JSON blob 均为确定性序列化。

-- 内容寻址存储。小对象内联，大对象落文件（<数据目录>/blobs/ab/cd/<hex>）。
CREATE TABLE blobs (
  hash       TEXT PRIMARY KEY,           -- 'sha256:<hex>'
  size       INTEGER NOT NULL,
  mime       TEXT,
  inline     BLOB,                       -- ≤64 KiB 时内联；否则 NULL
  created_at INTEGER NOT NULL
);

CREATE TABLE works (
  id                TEXT PRIMARY KEY,    -- work.yaml 中的 id
  title             TEXT NOT NULL,
  latest_version_id TEXT,
  dev_path          TEXT,                -- 开发目录：链接的本地目录（WRK-10）
  created_at        INTEGER NOT NULL,
  updated_at        INTEGER NOT NULL
);

CREATE TABLE work_versions (
  id          TEXT PRIMARY KEY,
  work_id     TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
  version     TEXT NOT NULL,             -- 作者声明的 semver
  tree_hash   TEXT NOT NULL,             -- 文件树哈希 = 版本身份
  is_dev      INTEGER NOT NULL DEFAULT 0,
  source      TEXT NOT NULL,             -- JSON：{type: zip|dir|github|url|dev|builtin, ...}
  manifest    TEXT NOT NULL,             -- 解析后的 work.yaml（JSON）
  report      TEXT,                      -- 校验报告（JSON）
  imported_at INTEGER NOT NULL
);
-- 文件树哈希即版本身份；开发目录的版本是例外：每部作品至多一个，原地更新，不参与去重（D-37）。
CREATE UNIQUE INDEX idx_work_versions_hash ON work_versions(work_id, tree_hash) WHERE is_dev = 0;
CREATE UNIQUE INDEX idx_work_versions_dev  ON work_versions(work_id) WHERE is_dev = 1;

CREATE TABLE work_files (
  version_id TEXT NOT NULL REFERENCES work_versions(id) ON DELETE CASCADE,
  path       TEXT NOT NULL,              -- 相对作品根，'/' 分隔，NFC 规范化
  blob_hash  TEXT NOT NULL REFERENCES blobs(hash),
  size       INTEGER NOT NULL,
  PRIMARY KEY (version_id, path)
);

CREATE TABLE saves (
  id              TEXT PRIMARY KEY,
  work_id         TEXT NOT NULL REFERENCES works(id),
  work_version_id TEXT NOT NULL REFERENCES work_versions(id),
  title           TEXT,                  -- 为空时界面显示"未命名存档 · 最近游玩时间"
  title_source    TEXT,                  -- generated | manual
  title_node      TEXT,                  -- 生成标题时的 head 节点
  forked_from     TEXT,                  -- JSON：{save_id, node_id, turn_no}（P1 复制为新存档）
  head_node_id    TEXT,
  settings        TEXT NOT NULL,         -- JSON：模型角色、篇幅、推理、助手开关、上下文策略覆盖
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL,
  last_played_at  INTEGER
);
CREATE INDEX idx_saves_work ON saves(work_id, last_played_at DESC);
CREATE INDEX idx_saves_recent ON saves(last_played_at DESC);

CREATE TABLE nodes (
  id              TEXT PRIMARY KEY,
  save_id         TEXT NOT NULL REFERENCES saves(id) ON DELETE CASCADE,
  parent_id       TEXT REFERENCES nodes(id),
  depth           INTEGER NOT NULL,      -- 根为 0
  kind            TEXT NOT NULL,         -- root | turn | compaction | edit
  status          TEXT NOT NULL,         -- committed | interrupted
  turn_no         INTEGER,               -- 进度线上的回合序号（turn 节点）
  input           TEXT,                  -- JSON：TurnInput（turn）/ EditInfo（edit）
  messages        TEXT,                  -- blob hash：本节点产生的 LLM 消息
  display         TEXT,                  -- blob hash：显示项列表
  epoch_block     TEXT,                  -- blob hash：阶段块（root/compaction）
  keep_from       TEXT,                  -- compaction：保留原文的第一个 turn 节点 id
  state_hash      TEXT NOT NULL,         -- 节点结束时的状态快照 blob
  memory_hash     TEXT NOT NULL,         -- 节点结束时的记忆清单 blob
  work_version_id TEXT NOT NULL,         -- 本节点生效的作品版本
  notes           TEXT,                  -- JSON：需附加到下一条玩家消息的系统注记
  usage           TEXT,                  -- JSON：本节点用量汇总
  meta            TEXT,                  -- JSON：模型、停止原因、前缀哈希、耗时、错误、edited_from 等
  created_at      INTEGER NOT NULL
);
CREATE INDEX idx_nodes_children ON nodes(save_id, parent_id, created_at);

-- 原文全文索引（trigram 适合中文子串；不足 3 字的查询走扫描）。
-- 已在 Windows/amd64 上验证 modernc.org/sqlite 支持 FTS5 + trigram（核对清单第 17 项，其余平台待 CI 确认）。
CREATE VIRTUAL TABLE transcript_fts USING fts5(
  text, node_id UNINDEXED, save_id UNINDEXED, tokenize = 'trigram'
);

-- 玩家上传的图片（已重新编码，内容在 blobs）。回合输入的附件只能引用登记在这里的图片。
CREATE TABLE uploads (
  hash       TEXT PRIMARY KEY REFERENCES blobs(hash),
  mime       TEXT NOT NULL,
  width      INTEGER NOT NULL,
  height     INTEGER NOT NULL,
  size       INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE providers (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  protocol   TEXT NOT NULL,              -- openai-responses | anthropic | gemini
  base_url   TEXT NOT NULL,
  headers    TEXT,                       -- JSON：额外请求头（不含密钥）
  options    TEXT,                       -- JSON：协议级选项
  secret_ref TEXT NOT NULL,              -- secrets.json 的键，或 env:NAME
  price_multiplier REAL NOT NULL DEFAULT 1, -- 价格倍率（中转站分组倍率，D-48）：实际费用 = 模型单价 × 倍率
  test_ok    INTEGER,                    -- 最近一次连接测试是否通过（NULL = 未测试）
  tested_at  INTEGER,
  created_at INTEGER NOT NULL
);

CREATE TABLE models (
  id             TEXT PRIMARY KEY,
  provider_id    TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  model          TEXT NOT NULL,          -- 供应商侧模型 ID
  display_name   TEXT,
  context_window INTEGER NOT NULL,
  max_output     INTEGER NOT NULL,
  caps           TEXT NOT NULL,          -- JSON 能力矩阵
  price          TEXT,                   -- JSON：每百万 token 价格；缺省表示未知
  defaults       TEXT                    -- JSON：推理强度、温度、图片最大边长等
);

CREATE TABLE usage_log (
  id             INTEGER PRIMARY KEY,
  save_id        TEXT,
  node_id        TEXT,
  call_kind      TEXT NOT NULL,          -- narrate | continue | compact | helper | title | probe
  model_id       TEXT NOT NULL,
  input_uncached INTEGER,
  cache_read     INTEGER,
  cache_write    INTEGER,
  output         INTEGER,
  reasoning      INTEGER,
  cost_micros    INTEGER,                -- NULL = 无法估算；单位为 currency 的百万分之一
  currency       TEXT,                   -- 计费货币（来自模型价格，D-34）
  latency_ms     INTEGER,
  stop           TEXT,
  created_at     INTEGER NOT NULL
);
CREATE INDEX idx_usage_save ON usage_log(save_id, created_at);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL                    -- JSON
);
