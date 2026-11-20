import { useEffect, useRef, useState } from 'react'

const key = 'dfipam.lastActivity'
const reasonKey = 'dfipam.signOutReason'
const warnFor = 60_000
const events = ['pointerdown', 'keydown', 'wheel', 'touchstart', 'mousemove'] as const

function read(): number {
  try {
    return Number(localStorage.getItem(key)) || Date.now()
  } catch {
    return Date.now()
  }
}

function write(at: number) {
  try {
    localStorage.setItem(key, String(at))
  } catch {
    // Private windows can refuse storage; this tab still tracks its own activity.
  }
}

/** Why the last sign-out happened, read once by the sign-in page. */
export function takeSignOutReason(): string | null {
  try {
    const r = sessionStorage.getItem(reasonKey)
    sessionStorage.removeItem(reasonKey)
    return r
  } catch {
    return null
  }
}

function setReason(minutes: number) {
  try {
    sessionStorage.setItem(reasonKey, `You were signed out after ${minutes} minutes without activity.`)
  } catch {
    // Without storage the sign-in page simply shows no reason.
  }
}

/**
 * Signs the person out after a period without activity. Activity in any tab counts,
 * so working in one tab doesn't sign out another. A warning shows for the last
 * minute; secondsLeft is null until then. While paused (the wall-screen monitor,
 * which nobody touches) the person counts as active; the server's 10-hour
 * session limit still ends the session.
 */
export function useIdleTimeout(minutes: number, onTimeout: () => void, paused = false) {
  const [secondsLeft, setSecondsLeft] = useState<number | null>(null)
  const last = useRef(Date.now())
  const hold = useRef(paused)
  hold.current = paused
  const limit = minutes * 60_000

  useEffect(() => {
    last.current = Date.now()
    write(last.current)
    let throttle = 0
    const touch = () => {
      const now = Date.now()
      if (now - throttle < 5_000) return // writing on every mouse move would be wasteful
      throttle = now
      last.current = now
      write(now)
    }
    for (const e of events) window.addEventListener(e, touch, { passive: true })
    const timer = window.setInterval(() => {
      if (hold.current) {
        last.current = Date.now()
        setSecondsLeft(null)
        return
      }
      const idle = Date.now() - Math.max(last.current, read())
      if (idle >= limit) {
        setReason(minutes)
        onTimeout()
        return
      }
      setSecondsLeft(limit - idle <= warnFor ? Math.ceil((limit - idle) / 1000) : null)
    }, 1_000)
    return () => {
      for (const e of events) window.removeEventListener(e, touch)
      window.clearInterval(timer)
    }
    // onTimeout is stable for the session; restarting the timer on each render would reset it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [limit])

  const stay = () => {
    last.current = Date.now()
    write(last.current)
    setSecondsLeft(null)
  }
  return { secondsLeft, stay }
}
