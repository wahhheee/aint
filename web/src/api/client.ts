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
    throw networkError(err)
  }
  return parseResponse<T>(method, path, res.status, await res.text(), res.headers.get('X-Request-ID'))
}

/** 上传选项：进度回调（已发送 / 总字节数）与取消。 */
export interface UploadOptions {
  onProgress?: (loaded: number, total: number) => void
  signal?: AbortSignal
}

/**
 * 上传 multipart 表单。fetch 拿不到上传进度，这里用 XMLHttpRequest；响应与错误的处理与 request 相同。
 * 取消时以 AbortError 拒绝（与 fetch 一致）。
 */
function upload<T>(path: string, body: FormData, { onProgress, signal }: UploadOptions = {}): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException('上传已取消', 'AbortError'))
      return
    }
    const xhr = new XMLHttpRequest()
    xhr.open('POST', API_BASE + path)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress?.(e.loaded, e.total)
    }
    xhr.onload = () => {
      try {
        resolve(parseResponse<T>('POST', path, xhr.status, xhr.responseText, xhr.getResponseHeader('X-Request-ID')))
      } catch (err) {
        reject(err)
      }
    }
    xhr.onerror = () => reject(networkError('XMLHttpRequest error'))
    xhr.onabort = () => reject(new DOMException('上传已取消', 'AbortError'))
    signal?.addEventListener('abort', () => xhr.abort(), { once: true })
    xhr.send(body)
  })
}

function networkError(cause: unknown): ApiError {
  return new ApiError('network', '无法连接到 AINT 服务，请确认程序仍在运行。', 0, String(cause))
}

/** 解析响应：成功时返回数据，失败时抛出 ApiError（fetch 与 XMLHttpRequest 共用）。 */
function parseResponse<T>(method: Method, path: string, status: number, text: string, requestIdHeader: string | null): T {
  if (status === 204) return undefined as T
  const data: unknown = text ? safeParse(text) : undefined

  if (status < 200 || status >= 300) {
    if (status === 401 && !path.startsWith('/auth/')) window.dispatchEvent(new Event('aint:unauthorized'))
    const e = (data as { error?: { code?: ErrorCode; message?: string; details?: unknown; request_id?: string } })?.error
    const err = new ApiError(
      e?.code ?? 'internal',
      e?.message ?? `请求失败（HTTP ${status}）`,
      status,
      e?.details,
      e?.request_id ?? requestIdHeader ?? undefined,
    )
    if (import.meta.env.DEV) {
      console.warn(`[api] ${method} ${path} → ${status} ${err.code}`, { requestId: err.requestId, details: err.details })
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
  post: <T>(path: string, body?: unknown, signal?: AbortSignal) => request<T>('POST', path, body ?? {}, signal),
  patch: <T>(path: string, body: unknown) => request<T>('PATCH', path, body),
  put: <T>(path: string, body: unknown) => request<T>('PUT', path, body),
  del: <T = void>(path: string) => request<T>('DELETE', path),
  upload,
}
