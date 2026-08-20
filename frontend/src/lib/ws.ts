// ws.ts — resilient WebSocket client for live ticks and events.

import type { Event } from './api'

export interface WsSample {
  s: string
  v: number
}

export interface WsTick {
  ts: number
  data: WsSample[]
}

export interface WsFrame {
  t: 'tick' | 'event'
  ts?: number
  tick?: WsTick
  event?: Event
}

export interface WsSub {
  type: 'sub'
  series: string[]
}

type Handler = (frame: WsFrame) => void

export class LiveClient {
  private ws: WebSocket | null = null
  private handlers = new Set<Handler>()
  private sub: string[] = []
  private retry = 0
  private maxRetry = 30_000
  private closed = false
  private timer: ReturnType<typeof setTimeout> | null = null
  private reconnectDelay = 800

  constructor(private url: string) {}

  connect(): void {
    this.closed = false
    this.open()
  }

  private open(): void {
    try {
      this.ws = new WebSocket(this.url)
    } catch {
      this.schedule()
      return
    }
    this.ws.onopen = () => {
      this.retry = 0
      this.reconnectDelay = 800
      if (this.sub.length > 0) this.send({ type: 'sub', series: this.sub })
    }
    this.ws.onmessage = (ev) => {
      try {
        const frame = JSON.parse(String(ev.data)) as WsFrame
        for (const h of this.handlers) h(frame)
      } catch {
        /* ignore malformed frames */
      }
    }
    this.ws.onclose = () => this.schedule()
    this.ws.onerror = () => this.ws?.close()
  }

  private schedule(): void {
    if (this.closed) return
    if (this.timer) clearTimeout(this.timer)
    this.timer = setTimeout(() => this.open(), this.reconnectDelay)
    this.reconnectDelay = Math.min(this.reconnectDelay * 1.6, this.maxRetry)
    this.retry++
  }

  subscribe(series: string[]): void {
    this.sub = series
    if (this.ws?.readyState === WebSocket.OPEN) this.send({ type: 'sub', series })
  }

  private send(msg: WsSub): void {
    this.ws?.send(JSON.stringify(msg))
  }

  on(handler: Handler): () => void {
    this.handlers.add(handler)
    return () => this.handlers.delete(handler)
  }

  destroy(): void {
    this.closed = true
    if (this.timer) clearTimeout(this.timer)
    this.ws?.close()
    this.handlers.clear()
  }
}

export function liveUrl(): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${location.host}/api/v1/ws`
}
