// Package errs 定义贯穿全项目的业务错误。
//
// 约定：
//   - 服务层返回 *errs.Error 表达"可预期的业务错误"（参数不对、找不到、存档正忙……），
//     Message 是面向玩家的简体中文，Details 放便于排查的结构化信息；
//   - 其他 error 一律视为内部错误：HTTP 层只返回通用文案，完整错误链写日志；
//   - 用 Wrap 保留底层错误（errors.Is/As 可穿透），日志里能看到根因。
//
// HTTP 状态码的映射在 server 包（server/respond.go），本包不依赖 net/http。
package errs

import (
	"errors"
	"fmt"
)

// Code 是稳定的错误码，前端据此决定展示方式；新增错误码时同步 web/src/api/errors.ts。
type Code string

const (
	CodeRateLimited     Code = "rate_limited"
	CodeInvalidArgument Code = "invalid_argument" // 请求参数不合法
	CodeNotFound        Code = "not_found"        // 资源不存在
	CodeConflict        Code = "conflict"         // 状态冲突，如存档正忙
	CodeUnauthorized    Code = "unauthorized"     // 需要登录（设置了访问密码时）
	CodeForbidden       Code = "forbidden"        // 请求被拒绝（如 Host/Origin 校验失败）
	CodeWorkInvalid     Code = "work_invalid"     // 作品校验未通过
	CodeModelMissing    Code = "model_missing"    // 未配置模型
	CodeNotImplemented  Code = "not_implemented"  // 功能尚未实现（脚手架阶段大量使用）
	CodeUnavailable     Code = "unavailable"      // 暂时不可用（如正在整理上下文）
	CodeInternal        Code = "internal"         // 内部错误
)

// Error 是业务错误。
type Error struct {
	Code    Code
	Message string // 面向玩家的说明
	Details any    // 便于排查的附加信息（会返回给前端，勿放敏感数据）
	Err     error  // 底层原因（只写日志，不返回给前端）
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Err }

// WithDetails 附加排查信息并返回自身，便于链式书写。
func (e *Error) WithDetails(d any) *Error {
	e.Details = d
	return e
}

// New 创建业务错误。
func New(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap 以业务错误包装底层错误。err 为 nil 时返回 nil。
func Wrap(err error, code Code, format string, args ...any) *Error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Err: err}
}

// 常用构造函数。

func Invalid(format string, args ...any) *Error  { return New(CodeInvalidArgument, format, args...) }
func NotFound(format string, args ...any) *Error { return New(CodeNotFound, format, args...) }
func Conflict(format string, args ...any) *Error { return New(CodeConflict, format, args...) }

// NotImplemented 标记尚未实现的功能。what 写清楚是哪项功能，便于在日志与界面中定位。
func NotImplemented(what string) *Error {
	return New(CodeNotImplemented, "功能尚未实现：%s", what)
}

// As 取出错误链中的 *Error。
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// CodeOf 返回错误码；非业务错误返回 CodeInternal。
func CodeOf(err error) Code {
	if e, ok := As(err); ok {
		return e.Code
	}
	return CodeInternal
}

// Is 判断错误链中是否有指定错误码的业务错误。
func Is(err error, code Code) bool {
	e, ok := As(err)
	return ok && e.Code == code
}
