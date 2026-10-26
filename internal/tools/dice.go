package tools

import (
	"context"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"

	"github.com/wahhheee/aint/internal/pkg/jsonx"
	"github.com/wahhheee/aint/internal/save"
)

// 掷骰（《02-技术方案》10.2）：表达式由若干项以 + / - 相连，每项是骰子 NdM（N 省略为 1，d% 即 d100）或整数。
// 结果给 AI 一行事实，并在正文流中显示骰子卡片（显示项 dice）。

// 掷骰表达式的上限：防止"1000d1000"之类的输入撑大结果与卡片。
const (
	maxDiceTerms = 8
	maxDiceCount = 50
	maxDiceSides = 1000
)

var reDiceTerm = regexp.MustCompile(`^(\d*)[dD](\d+|%)$`)

type diceTerm struct {
	sign  int // +1 / -1
	count int // 骰子个数；0 表示常数项
	sides int
	value int // 常数项的值
}

// parseDice 解析掷骰表达式。错误文案直接给 AI，写成可行的写法。
func parseDice(expr string) ([]diceTerm, string, error) {
	src := strings.ReplaceAll(strings.TrimSpace(expr), " ", "")
	if src == "" {
		return nil, "", fmt.Errorf("expr 为空，可以写成 \"2d6+1\"、\"d100\"")
	}
	var terms []diceTerm
	var canon strings.Builder
	sign, start := 1, 0
	flush := func(end int) error {
		tok := src[start:end]
		if tok == "" {
			return fmt.Errorf("表达式 %q 写法不对，可以写成 \"2d6+1\"、\"d100\"", expr)
		}
		t := diceTerm{sign: sign}
		if m := reDiceTerm.FindStringSubmatch(tok); m != nil {
			t.count = 1
			if m[1] != "" {
				t.count, _ = strconv.Atoi(m[1])
			}
			if m[2] == "%" {
				t.sides = 100
			} else {
				t.sides, _ = strconv.Atoi(m[2])
			}
			switch {
			case t.count < 1 || t.count > maxDiceCount:
				return fmt.Errorf("一次最多掷 %d 颗骰子，%q 可以拆成几次来掷", maxDiceCount, tok)
			case t.sides < 2 || t.sides > maxDiceSides:
				return fmt.Errorf("骰子的面数在 2–%d 之间，%q 不在范围内", maxDiceSides, tok)
			}
		} else {
			n, err := strconv.Atoi(tok)
			if err != nil || n > 100000 {
				return fmt.Errorf("%q 不是骰子也不是整数，可以写成 \"2d6+1\"、\"d100\"", tok)
			}
			t.value = n
		}
		if len(terms) > 0 || sign < 0 {
			if sign < 0 {
				canon.WriteByte('-')
			} else {
				canon.WriteByte('+')
			}
		}
		if t.count > 0 {
			fmt.Fprintf(&canon, "%dd%d", t.count, t.sides)
		} else {
			canon.WriteString(strconv.Itoa(t.value))
		}
		terms = append(terms, t)
		return nil
	}
	for i := range len(src) {
		if c := src[i]; c == '+' || c == '-' {
			if i == 0 && c == '-' {
				sign, start = -1, 1
				continue
			}
			if err := flush(i); err != nil {
				return nil, "", err
			}
			sign, start = 1, i+1
			if c == '-' {
				sign = -1
			}
		}
	}
	if err := flush(len(src)); err != nil {
		return nil, "", err
	}
	if len(terms) > maxDiceTerms {
		return nil, "", fmt.Errorf("表达式最多 %d 项，可以拆成几次来掷", maxDiceTerms)
	}
	if !hasDice(terms) {
		return nil, "", fmt.Errorf("表达式里没有骰子，可以写成 \"2d6+1\"、\"d100\"")
	}
	return terms, canon.String(), nil
}

func hasDice(terms []diceTerm) bool {
	for _, t := range terms {
		if t.count > 0 {
			return true
		}
	}
	return false
}

// rollDie 掷一颗 sides 面的骰子（1..sides）。测试中可替换为确定的序列。
var rollDie = func(sides int) int {
	return rand.IntN(sides) + 1 //nolint:gosec // G404：掷骰是游戏随机数，不需要密码学强度
}

type rollArgs struct {
	Expr   string `json:"expr"`
	Reason string `json:"reason"`
}

func handleRollDice(_ context.Context, env Env, raw jsonx.Value) Result {
	var a rollArgs
	if err := decodeArgs(raw, &a); err != nil {
		return NotExecuted("参数格式不对（%v），可以写成 {\"expr\": \"2d6+1\", \"reason\": \"……\"}。", err)
	}
	terms, canon, err := parseDice(a.Expr)
	if err != nil {
		return NotExecuted("%v。", err)
	}
	var rolls []int
	var parts []string
	total, bonus := 0, 0
	for _, t := range terms {
		if t.count == 0 {
			total += t.sign * t.value
			bonus += t.sign * t.value
			continue
		}
		faces := make([]string, t.count)
		for i := range t.count {
			r := rollDie(t.sides)
			rolls = append(rolls, r)
			total += t.sign * r
			faces[i] = strconv.Itoa(r)
		}
		part := fmt.Sprintf("%dd%d：%s", t.count, t.sides, strings.Join(faces, "、"))
		if t.sign < 0 {
			part = "减去 " + part
		}
		parts = append(parts, part)
	}
	if bonus != 0 {
		parts = append(parts, fmt.Sprintf("加值 %+d", bonus))
	}
	env.ShowCard(save.DisplayItem{T: "dice", Expr: canon, Rolls: rolls, Total: total, Text: strings.TrimSpace(a.Reason)})
	return Text("# roll_dice %s\n%s；合计 %d", canon, strings.Join(parts, "；"), total)
}
