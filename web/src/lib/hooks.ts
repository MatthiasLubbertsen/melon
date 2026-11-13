import { useEffect, useRef, useState } from 'react'

/** Re-renders on an interval, so relative times and "now" stay honest. */
export function useNow(everyMs: number): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const tick = () => setNow(Date.now())
    const id = setInterval(tick, everyMs)
    document.addEventListener('visibilitychange', tick)
    return () => {
      clearInterval(id)
      document.removeEventListener('visibilitychange', tick)
    }
  }, [everyMs])
  return now
}

export function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

const THRESHOLD = 72

/**
 * Pull-to-refresh for touch screens. Returns how far the user has pulled
 * (0..1+) so the page can show an indicator; calls onRefresh on release.
 */
export function usePullToRefresh(onRefresh: () => Promise<unknown>, enabled = true) {
  const [pull, setPull] = useState(0)
  const [refreshing, setRefreshing] = useState(false)
  const state = useRef({ startY: 0, active: false, pull: 0, busy: false })
  const cb = useRef(onRefresh)
  cb.current = onRefresh

  useEffect(() => {
    if (!enabled) return
    const s = state.current
    const start = (e: TouchEvent) => {
      // Only at the very top, and never while a sheet or dialog is open.
      if (window.scrollY > 0 || s.busy || document.querySelector('[data-slot="drawer-popup"],[data-slot="dialog-content"]')) return
      s.startY = e.touches[0].clientY
      s.active = true
    }
    const move = (e: TouchEvent) => {
      if (!s.active) return
      const dy = e.touches[0].clientY - s.startY
      if (dy <= 0 || window.scrollY > 0) {
        s.pull = 0
        setPull(0)
        return
      }
      s.pull = Math.min(dy * 0.5, THRESHOLD * 1.5) / THRESHOLD
      setPull(s.pull)
    }
    const end = async () => {
      if (!s.active) return
      s.active = false
      const go = s.pull >= 1
      s.pull = 0
      setPull(0)
      if (!go) return
      s.busy = true
      setRefreshing(true)
      try {
        await cb.current()
      } finally {
        s.busy = false
        setRefreshing(false)
      }
    }
    window.addEventListener('touchstart', start, { passive: true })
    window.addEventListener('touchmove', move, { passive: true })
    window.addEventListener('touchend', end)
    window.addEventListener('touchcancel', end)
    return () => {
      window.removeEventListener('touchstart', start)
      window.removeEventListener('touchmove', move)
      window.removeEventListener('touchend', end)
      window.removeEventListener('touchcancel', end)
    }
  }, [enabled])

  return { pull, refreshing, threshold: THRESHOLD }
}
