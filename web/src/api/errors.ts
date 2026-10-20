/**
 * API 错误。后端统一返回 {error: {code, message, details, request_id}}（internal/server/respond.go）。
 * 错误码与 internal/pkg/errs 同步。
 */

export type ErrorCode =
  | 'invalid_argument'
  | 'not_found'
  | 'conflict'
  | 'unauthorized'
  | 'forbidden'
  | 'work_invalid'
  | 'model_missing'
  | 'not_implemented'
  | 'unavailable'
  | 'internal'
  | 'network' // 前端自有：请求没有到达服务器

export class ApiError extends Error {
  readonly code: ErrorCode
  readonly status: number
  readonly details?: unknown
  /** 服务端分配的请求 ID：反馈问题时提供它，可在日志中直接定位 */
  readonly requestId?: string

  constructor(code: ErrorCode, message: string, status: number, details?: unknown, requestId?: string) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.details = details
    this.requestId = requestId
  }
}

export function isApiError(err: unknown, code?: ErrorCode): err is ApiError {
  return err instanceof ApiError && (code === undefined || err.code === code)
}

/** 给界面展示用的错误文案（业务错误用服务端文案；未知错误给出通用说明）。 */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message
  if (err instanceof Error) return err.message
  return String(err)
}
