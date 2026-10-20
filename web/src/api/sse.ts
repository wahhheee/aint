/**
 * SSE 连接管理（《04-交互与界面设计》17.2）。
 *
 *   - 浏览器的 EventSource 在网络中断时会自动重连并携带 Last-Event-ID，服务端据此补发；
 *   - 连接被彻底关闭时（如服务重启、返回非 200），这里接手：指数退避重连，用 ?last_event_id= 续传；
 *   - 服务端发出 resync（无法补发）时通知调用方重新拉取时间线。
 *
 * 调试：开发环境下在控制台执行 `localStorage.debugSSE = 1` 可打印每一条事件。
 */
import { API_BASE } from './client'
import { EVENT_TYPES, type AnyEvent } from './events'

export type StreamStatus = 'connecting' | 'open' | 'reconnecting'

export interface StreamHandlers {
  onEvent: (ev: AnyEvent) => void
  onResync?: () => void
  onStatus?: (status: StreamStatus) => void
}

const MAX_BACKOFF = 15_000

/** 打开事件流，返回关闭函数。path 形如 `/saves/{id}/events`。 */
export function openEventStream(path: string, handlers: StreamHandlers): () => void {
  let es: EventSource | null = null
  let lastId = 0
  let attempt = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  let closed = false
  const debug = import.meta.env.DEV && typeof localStorage !== 'undefined' && localStorage.getItem('debugSSE') === '1'

  const connect = () => {
    if (closed) return
    handlers.onStatus?.(attempt === 0 ? 'connecting' : 'reconnecting')
    const url = API_BASE + path + (lastId > 0 ? `?last_event_id=${lastId}` : '')
    es = new EventSource(url)

    es.onopen = () => {
      attempt = 0
      handlers.onStatus?.('open')
    }
    es.onerror = () => {
      // CONNECTING：浏览器正在自动重连；CLOSED：需要我们自己重连。
      if (es?.readyState === EventSource.CLOSED) scheduleReconnect()
      else handlers.onStatus?.('reconnecting')
    }
    for (const type of EVENT_TYPES) {
      es.addEventListener(type, (raw) => {
        const msg = raw as MessageEvent<string>
        const id = Number(msg.lastEventId) || 0
        if (id > 0) lastId = id
        let data: unknown
        try {
          data = JSON.parse(msg.data)
        } catch {
          console.error('[sse] 无法解析事件', type, msg.data)
          return
        }
        if (debug) console.debug('[sse]', id, type, data)
        handlers.onEvent({ type, id, data } as AnyEvent)
      })
    }
    es.addEventListener('resync', () => {
      if (debug) console.debug('[sse] resync')
      lastId = 0
      handlers.onResync?.()
    })
  }

  const scheduleReconnect = () => {
    es?.close()
    es = null
    if (closed) return
    attempt++
    const delay = Math.min(MAX_BACKOFF, 500 * 2 ** attempt) * (0.8 + Math.random() * 0.4)
    handlers.onStatus?.('reconnecting')
    timer = setTimeout(connect, delay)
  }

  connect()
  return () => {
    closed = true
    clearTimeout(timer)
    es?.close()
  }
}
