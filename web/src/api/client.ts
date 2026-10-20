/**
 * REST 客户端：统一前缀、JSON 编解码、错误归一（ApiError）。
 *
 * 调试：开发环境下每个失败请求都会在控制台打印 method、path、状态码与 request_id，
 * 拿 request_id 去 <数据目录>/logs 中搜索即可看到服务端日志。
 */
import { ApiError, type ErrorCode } from './errors'

export const API_BASE = '/api/v1'

type Method = 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE'

async function request<T>(method: Method, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  let res: Response
  try {
    res = await fetch(API_BASE + path, {
      method,
      signal,
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch (err) {
    if ((err as Error).name === 'AbortError') throw err
    throw new ApiError('network', '无法连接到演义服务，请确认程序仍在运行。', 0, String(err))
  }

  if (res.status === 204) return undefined as T
  const text = await res.text()
  const data: unknown = text ? safeParse(text) : undefined

  if (!res.ok) {
    const e = (data as { error?: { code?: ErrorCode; message?: string; details?: unknown; request_id?: string } })?.error
    const err = new ApiError(
      e?.code ?? 'internal',
      e?.message ?? `请求失败（HTTP ${res.status}）`,
      res.status,
      e?.details,
      e?.request_id ?? res.headers.get('X-Request-ID') ?? undefined,
    )
    if (import.meta.env.DEV) {
      console.warn(`[api] ${method} ${path} → ${res.status} ${err.code}`, { requestId: err.requestId, details: err.details })
    }
    throw err
  }
  return data as T
}

function safeParse(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return { error: { code: 'internal', message: text.slice(0, 200) } }
  }
}

export const http = {
  get: <T>(path: string, signal?: AbortSignal) => request<T>('GET', path, undefined, signal),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  patch: <T>(path: string, body: unknown) => request<T>('PATCH', path, body),
  put: <T>(path: string, body: unknown) => request<T>('PUT', path, body),
  del: <T = void>(path: string) => request<T>('DELETE', path),
}
