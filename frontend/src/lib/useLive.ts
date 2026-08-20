import { useEffect, useRef } from 'react'
import { LiveClient, liveUrl, type WsFrame } from '../lib/ws'
import { useConnectionStore } from '../state/useConnectionStore'
import { useMetricsStore } from '../state/useMetricsStore'
import { useEventsStore } from '../state/useEventsStore'

export function useLive(series: string[] = []) {
  const setConnected = useConnectionStore((s) => s.setConnected)
  const applyLive = useMetricsStore((s) => s.applyLive)
  const pushEvent = useEventsStore((s) => s.push)
  const seriesRef = useRef(series)
  seriesRef.current = series

  useEffect(() => {
    const client = new LiveClient(liveUrl())
    client.connect()

    // Poll the socket state so the "live/offline" indicator is honest even
    // while the client is backing off between reconnect attempts.
    const poll = setInterval(() => {
      const ws = (client as unknown as { ws: WebSocket | null }).ws
      const open = ws?.readyState === WebSocket.OPEN
      setConnected(open)
      if (open) client.subscribe(seriesRef.current)
    }, 1500)

    const off = client.on((frame: WsFrame) => {
      if (frame.t === 'event' && frame.event) {
        pushEvent(frame.event)
      } else if (frame.t === 'tick' && frame.tick) {
        applyLive(frame.tick.data)
      }
    })

    return () => {
      clearInterval(poll)
      off()
      client.destroy()
      setConnected(false)
    }
  }, [applyLive, pushEvent, setConnected])
}