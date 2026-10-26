package work

import (
	"fmt"
	"strconv"
	"strings"
)

// EngineVersion 是程序的作品能力版本（D-32）：作品可用的能力集合（标签、字段、工具语义……）。
//
// 只在作品可用的能力增加时递增次版本（例如新增 <act> 标签 → 1.1），出现不兼容变化时递增主版本；
// 与程序的发布版本无关——程序修 bug、改界面不需要改它。作品在 work.yaml 的 engine 中声明所需范围。
const EngineVersion = "2.0"

// engineSatisfies 判断 version 是否满足范围表达式。
//
// 支持的写法（semver 范围的常用子集）：
//
//	">=1.0 <2.0"   空格分隔的比较式同时满足
//	"^1.1"         等价于 ">=1.1 <2.0"
//	"1.0"、"=1.0"  精确匹配
//	">=1.0 || ^3"  任一组满足
func engineSatisfies(rangeExpr, version string) (bool, error) {
	v, err := parseVer(version)
	if err != nil {
		return false, err
	}
	for _, group := range strings.Split(rangeExpr, "||") {
		ok := true
		fields := strings.Fields(group)
		if len(fields) == 0 {
			return false, fmt.Errorf("范围为空")
		}
		for _, c := range fields {
			sat, err := compare(c, v)
			if err != nil {
				return false, err
			}
			ok = ok && sat
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

type ver [3]int

func parseVer(s string) (ver, error) {
	var v ver
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) == 0 || len(parts) > 3 {
		return v, fmt.Errorf("版本号 %q 不合法", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, fmt.Errorf("版本号 %q 不合法", s)
		}
		v[i] = n
	}
	return v, nil
}

func (a ver) cmp(b ver) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func compare(c string, v ver) (bool, error) {
	if rest, ok := strings.CutPrefix(c, "^"); ok {
		lo, err := parseVer(rest)
		if err != nil {
			return false, err
		}
		hi := ver{lo[0] + 1, 0, 0}
		return v.cmp(lo) >= 0 && v.cmp(hi) < 0, nil
	}
	for _, op := range []string{">=", "<=", ">", "<", "="} {
		if rest, ok := strings.CutPrefix(c, op); ok {
			t, err := parseVer(rest)
			if err != nil {
				return false, err
			}
			d := v.cmp(t)
			switch op {
			case ">=":
				return d >= 0, nil
			case "<=":
				return d <= 0, nil
			case ">":
				return d > 0, nil
			case "<":
				return d < 0, nil
			default:
				return d == 0, nil
			}
		}
	}
	t, err := parseVer(c)
	if err != nil {
		return false, err
	}
	return v.cmp(t) == 0, nil
}
