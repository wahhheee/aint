// Package settings 是全局设置（阅读偏好、外观、模型角色、默认运行参数……），存于 settings 表。
//
// 存储方式：整份设置序列化为一个 JSON 文档（key = "app"）。读取时先取默认值，再把库中的值覆盖上去——
// 新版本增加的设置项自动获得默认值，无需迁移。前端以"部分更新"（PATCH）的方式修改。
// 访问密码等安全相关设置不在这里（见 access 包与 secrets.json）。
package settings

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
	"github.com/wahhheee/yanyi/internal/store"
)

// Settings 是全局设置。修改字段时同步 web/src/api/types.ts。
type Settings struct {
	Listen       string     `json:"listen"`         // 监听地址，重启生效。
	Theme        string     `json:"theme"`          // system | light | dark
	UseWorkTheme bool       `json:"use_work_theme"` // 关闭则使用平台主题（样式标签仍生效）
	Reading      Reading    `json:"reading"`
	Audio        Audio      `json:"audio"`
	Models       ModelRoles `json:"models"`
	Play         Play       `json:"play"`
	Context      Context    `json:"context"`
	Onboarded    bool       `json:"onboarded"` // 是否已看过欢迎页
	// Developer 是"开发者选项"（SET-07，D-31）：显示完整状态与状态编辑；游玩调试使用独立开关。
	// 默认关闭；从开发目录链接的作品不论此开关如何都显示（Presentation.DevWork）。
	Developer bool `json:"developer"`
}

// Reading 是阅读设置（SET-02）。
type Reading struct {
	TimedChoices     bool    `json:"timed_choices"`
	FontFamily       string  `json:"font_family"` // work | serif | sans
	FontSize         int     `json:"font_size"`   // 14–24 px
	LineHeight       float64 `json:"line_height"` // 1.5–2.4
	ParagraphSpacing float64 `json:"paragraph_spacing"`
	MaxWidth         float64 `json:"max_width"` // 32–52 em
	Indent           string  `json:"indent"`    // work | on | off
	Typewriter       string  `json:"typewriter"`
	ReduceMotion     string  `json:"reduce_motion"`   // system | on | off
	StageIntensity   float64 `json:"stage_intensity"` // 舞台背景强度 0–1
	ToolHints        bool    `json:"tool_hints"`      // 显示"翻阅了 N 份资料"
}

// Audio 是音频设置（SET-04，P1）。
type Audio struct {
	Master float64 `json:"master"`
	BGM    float64 `json:"bgm"`
	SFX    float64 `json:"sfx"`
	Muted  bool    `json:"muted"`
}

// ModelRoles 是模型角色（PRV-06）：值为 models.id。
type ModelRoles struct {
	Narrator string `json:"narrator"`
	Light    string `json:"light"`
}

// Play 是新存档的默认运行参数与游玩相关开关。
type Play struct {
	Pace        string `json:"pace"`          // short | medium | long
	Reasoning   string `json:"reasoning"`     // off | low | medium | high（作品推荐值优先）
	CacheTTL    string `json:"cache_ttl"`     // short | long：缓存保留时长（llm.CacheShort / CacheLong）
	TitleOnExit string `json:"title_on_exit"` // auto | ask | off
	Helpers     bool   `json:"helpers"`
}

// Context 是上下文整理的默认参数（《04-交互与界面设计》9.4）；存档设置可以单独覆盖（save.Settings）。
type Context struct {
	MaxCalls     int    `json:"max_calls"` // 0 沿用运行时默认上限
	MaxToolCalls int    `json:"max_tool_calls"`
	MaxOutput    int    `json:"max_output"`
	Budget       int    `json:"budget"`      // 工作上下文预算（token）：上下文达到它（且不超过模型窗口 − 预留）时整理
	KeepRecent   int    `json:"keep_recent"` // 整理后原样保留的最近回合（token）
	Mode         string `json:"mode"`        // inline（主笔在原上下文中整理，复用缓存）| separate（轻量模型单独整理）
}

// Defaults 返回默认设置（《04-交互与界面设计》10.2、9.4）。
func Defaults() Settings {
	return Settings{
		Listen:       "127.0.0.1",
		Theme:        "system",
		UseWorkTheme: true,
		Reading: Reading{
			TimedChoices: true, FontFamily: "work", FontSize: 18, LineHeight: 1.9, ParagraphSpacing: 0.9, MaxWidth: 40,
			Indent: "work", Typewriter: "normal", ReduceMotion: "system", StageIntensity: 0.6, ToolHints: true,
		},
		Audio:   Audio{Master: 0.6, BGM: 1, SFX: 1},
		Play:    Play{Pace: "medium", Reasoning: "low", CacheTTL: "short", TitleOnExit: "off"},
		Context: Context{Budget: 128_000, KeepRecent: 12_000, Mode: "inline"},
		Models:  ModelRoles{},
	}
}

const key = "app"

// Service 是设置服务。
type Service struct {
	db *store.DB
}

// NewService 创建设置服务。
func NewService(db *store.DB) *Service { return &Service{db: db} }

// Get 返回当前设置（默认值 + 已保存的值）。
func (s *Service) Get(ctx context.Context) (Settings, error) {
	return s.get(ctx, s.db.Read())
}

func (s *Service) get(ctx context.Context, q store.Querier) (Settings, error) {
	cur := Defaults()
	var raw string
	err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return cur, nil
	}
	if err != nil {
		return cur, err
	}
	if err := jsonx.Unmarshal([]byte(raw), &cur); err != nil {
		return Defaults(), errs.Wrap(err, errs.CodeInternal, "设置数据已损坏，已回退为默认值")
	}
	return cur, nil
}

// Patch 部分更新设置：patch 中出现的字段覆盖当前值（嵌套对象逐字段合并）。
func (s *Service) Patch(ctx context.Context, patch []byte) (Settings, error) {
	var out Settings
	err := s.db.Tx(ctx, func(tx *sql.Tx) error {
		cur, err := s.get(ctx, tx)
		if err != nil {
			return err
		}
		if err := jsonx.UnmarshalStrict(bytes.NewReader(patch), &cur); err != nil {
			return errs.Invalid("设置格式不正确：%v", err)
		}
		if err := cur.Validate(); err != nil {
			return err
		}
		b, err := jsonx.Marshal(cur)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, string(b)); err != nil {
			return err
		}
		out = cur
		return nil
	})
	return out, err
}
