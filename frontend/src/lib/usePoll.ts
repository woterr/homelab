import { useEffect, useRef, useState } from 'react'

export function usePoll(fn: () => Promise<void> | void, interval: number, deps: unknown[] = []) {
  const fnRef = useRef(fn)
  fnRef.current = fn
  useEffect(() => {
    let stopped = false
    let timer: ReturnType<typeof setTimeout>
    const tick = async () => {
      try {
        await fnRef.current()
      } catch {
        /* swallow background poll errors */
      }
      if (!stopped) timer = setTimeout(tick, interval)
    }
    void tick()
    return () => {
      stopped = true
      clearTimeout(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [interval, ...deps])
}

export function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs)
    return () => clearInterval(id)
  }, [intervalMs])
  return now
}