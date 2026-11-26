export function formatCount(n: number): string {
  return n.toLocaleString('en-GB')
}

/** How long ago, in words: "just now", "12 min ago", "3 days ago". */
export function formatAgo(iso: string, now = Date.now()): string {
  const minutes = Math.floor((now - new Date(iso).getTime()) / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} h ago`
  const days = Math.floor(hours / 24)
  return `${days} day${days === 1 ? '' : 's'} ago`
}

/** How long until a moment, in words: "in 4 min", "in 2 h". */
export function formatIn(iso: string, now = Date.now()): string {
  const minutes = Math.ceil((new Date(iso).getTime() - now) / 60000)
  if (minutes <= 0) return 'now'
  if (minutes < 60) return `in ${minutes} min`
  return `in ${Math.round(minutes / 60)} h`
}

/** The building's own time zone. Every time is shown in it, whatever the viewer's computer says. */
export const timeZone = 'Asia/Colombo'

/** A moment as 15 Jan 2027, 14:05, in Sri Lanka time. */
export function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString('en-GB', { dateStyle: 'medium', timeStyle: 'short', timeZone })
}

/** Short date for date-only values such as YYYY-MM-DD, read as calendar dates. */
export function formatDate(value: string): string {
  const [y, m, d] = value.slice(0, 10).split('-').map(Number)
  return new Date(Date.UTC(y, m - 1, d)).toLocaleDateString('en-GB', { dateStyle: 'medium', timeZone: 'UTC' })
}

/** Today as YYYY-MM-DD by the person's own calendar, plus a number of days. */
export function isoDate(addDays = 0): string {
  const d = new Date()
  d.setDate(d.getDate() + addDays)
  return d.toLocaleDateString('en-CA')
}

/** Whole days from now until an instant; negative once it has passed. */
export function daysUntil(iso: string, now = Date.now()): number {
  return Math.ceil((new Date(iso).getTime() - now) / 86_400_000)
}

/**
 * A reservation runs to the end of its chosen day in UTC, stored as the midnight
 * that follows it. These give back the chosen day.
 */
export function endOfDayDate(iso: string): string {
  return new Date(Date.parse(iso) - 1000).toISOString().slice(0, 10)
}

/** Calendar days to a reservation's last day: 0 on the day itself, 1 the day before. */
export function daysLeft(iso: string, now = Date.now()): number {
  const today = new Date(now).toISOString().slice(0, 10)
  return Math.round((Date.parse(endOfDayDate(iso)) - Date.parse(today)) / 86_400_000)
}

export function formatEndOfDay(iso: string): string {
  return formatDate(endOfDayDate(iso))
}

export type Level = 'normal' | 'warning' | 'critical'

export function level(pct: number, warn: number, critical: number): Level {
  if (pct >= critical) return 'critical'
  return pct >= warn ? 'warning' : 'normal'
}

/** Page numbers to show, with gaps marked by null: 1 … 4 5 6 … 20. */
export function pageWindow(page: number, totalPages: number): (number | null)[] {
  const wanted = new Set([1, totalPages, page - 1, page, page + 1].filter((p) => p >= 1 && p <= totalPages))
  const sorted = [...wanted].sort((a, b) => a - b)
  const out: (number | null)[] = []
  sorted.forEach((p, i) => {
    if (i > 0) {
      const gap = p - sorted[i - 1]
      if (gap === 2) out.push(p - 1)
      else if (gap > 2) out.push(null)
    }
    out.push(p)
  })
  return out
}

/** "Floor 4 · VLAN 40" style segment captions. */
export function segmentCaption(s: { floor: number | null; vlan: number | null; network: string }): string {
  return [s.floor !== null ? `Floor ${s.floor}` : null, s.vlan !== null ? `VLAN ${s.vlan}` : null, s.network].filter(Boolean).join(' · ')
}

/** A person's name from a work email: hasitha.wickramasinghe@… gives "Hasitha Wickramasinghe". */
export function nameFromEmail(email: string): string {
  const parts = email.split('@')[0].split('.').filter(Boolean)
  return parts.map((p) => p.charAt(0).toUpperCase() + p.slice(1).toLowerCase()).join(' ') || email
}

/** Initials for an avatar: the first letter of the first two names (HW), or the first two letters of a single name. */
export function initialsFromEmail(email: string): string {
  const parts = email.split('@')[0].split('.').filter(Boolean)
  const letters = parts.length > 1 ? (parts[0][0] ?? '') + (parts[1][0] ?? '') : (parts[0] ?? '').slice(0, 2)
  return letters.toUpperCase() || '?'
}
