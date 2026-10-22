package tools

import (
	"strings"

	"github.com/wahhheee/yanyi/internal/pkg/jsonx"
)

// decodeArgs 解码工具参数。部分模型会把数组或对象类型的参数写成 JSON 字符串（如 "choices": "[{…}]"），
// 直接解码失败时，把这类字符串参数还原为数组 / 对象后再解码一次；仍然失败时返回第一次的错误。
func decodeArgs(raw jsonx.Value, v any) error {
	err := jsonx.Unmarshal(raw, v)
	if err == nil {
		return nil
	}
	var m map[string]any
	if jsonx.Unmarshal(raw, &m) != nil {
		return err
	}
	changed := false
	for k, val := range m {
		s, ok := val.(string)
		if !ok {
			continue
		}
		if t := strings.TrimSpace(s); strings.HasPrefix(t, "[") || strings.HasPrefix(t, "{") {
			var inner any
			if jsonx.Unmarshal([]byte(t), &inner) == nil {
				m[k], changed = inner, true
			}
		}
	}
	if !changed {
		return err
	}
	fixed, merr := jsonx.Marshal(m)
	if merr != nil || jsonx.Unmarshal(fixed, v) != nil {
		return err
	}
	return nil
}
