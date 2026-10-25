package save

import (
	"context"
	"database/sql"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/wahhheee/yanyi/internal/pkg/errs"
	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

var paceRange = regexp.MustCompile(`^[1-9][0-9]{0,4}-[1-9][0-9]{0,4}$`)

// Validate 检查存档参数；零值的预算表示沿用全局设置。
func (s Settings) Validate() error {
	if s.NarratorModel == "" {
		return errs.Invalid("请选择主笔模型")
	}
	if !slices.Contains([]string{"short", "medium", "long"}, s.Pace) && !paceRange.MatchString(s.Pace) {
		return errs.Invalid("篇幅设置不正确")
	}
	if paceRange.MatchString(s.Pace) {
		parts := strings.Split(s.Pace, "-")
		low, _ := strconv.Atoi(parts[0])
		high, _ := strconv.Atoi(parts[1])
		if low > high {
			return errs.Invalid("篇幅下限不能大于上限")
		}
	}
	if !slices.Contains([]string{"off", "low", "medium", "high"}, s.Reasoning) {
		return errs.Invalid("推理强度不正确")
	}
	if !slices.Contains([]string{"", "short", "long"}, s.CacheTTL) {
		return errs.Invalid("缓存时长不正确")
	}
	if !slices.Contains([]string{"", "inline", "separate"}, s.CompactMode) {
		return errs.Invalid("整理方式不正确")
	}
	if s.ContextBudget < 0 || s.ContextBudget > 2_000_000 || s.KeepRecent < 0 || s.KeepRecent > 2_000_000 {
		return errs.Invalid("上下文参数超出范围")
	}
	if s.MaxCalls < 0 || s.MaxCalls > 50 || s.MaxToolCalls < 0 || s.MaxToolCalls > 200 || s.MaxOutput < 0 || s.MaxOutput > 128_000 {
		return errs.Invalid("回合预算超出范围")
	}
	return nil
}

// UpdateSettings 保存经过校验的完整设置；调用方持有存档写者槽位。
func (s *Service) UpdateSettings(ctx context.Context, id string, value Settings) error {
	if err := value.Validate(); err != nil {
		return err
	}
	raw, err := jsonx.Marshal(value)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx *sql.Tx) error { return updateSettings(ctx, tx, id, string(raw)) })
}
