import { useQuery } from '@tanstack/react-query'
import { cx } from '../../components/ui/cx'
import { get } from '../../lib/api'

interface Summary { total: number; needs: number; unknown: number; alerting: number; known: number; new: number }

type Chip = { kind: string; label: string; key: keyof Summary; tone: string; hint: string }

const chips: Chip[] = [
  { kind: '', label: 'All', key: 'total', tone: 'text-ink', hint: 'Every address in this window' },
  { kind: 'needs', label: 'Needs action', key: 'needs', tone: 'text-warn', hint: 'An open alert, or a device nobody has decided on' },
  { kind: 'alerting', label: 'Open alerts', key: 'alerting', tone: 'text-danger', hint: 'Addresses with an open alert' },
  { kind: 'unknown', label: 'Undecided devices', key: 'unknown', tone: 'text-warn', hint: 'MACs waiting to be approved or rejected' },
  { kind: 'known', label: 'Known', key: 'known', tone: 'text-ok', hint: 'Registered, and the device is approved' },
  { kind: 'new', label: 'New in the last hour', key: 'new', tone: 'text-accent', hint: 'First discovered within the last hour' },
]

/**
 * Counts for the chosen segment and time window. Each chip sets the Finding filter,
 * so the table below shows just those rows. A zero stays quiet.
 */
export function DiscoverySummary({ segmentId, seen, kind, onKind }: { segmentId: string; seen: string; kind: string; onKind: (k: string) => void }) {
  const params = new URLSearchParams({ seen, ...(segmentId ? { segmentId } : {}) })
  const path = `/discovery/summary?${params}`
  const q = useQuery({ queryKey: [path], queryFn: () => get<Summary>(path), refetchInterval: 30_000, placeholderData: (prev) => prev })
  if (q.isError) return null // the table below shows the error and a retry
  return (
    <div className="grid grid-cols-6 gap-2 border-b border-line p-3 max-xl:grid-cols-3 max-sm:grid-cols-2" aria-busy={q.isPending}>
      {chips.map((c) => {
        const value = q.data?.[c.key]
        const active = kind === c.kind || (c.kind === '' && !chips.some((x) => x.kind === kind))
        return (
          <button key={c.key} type="button" title={c.hint} aria-pressed={active} onClick={() => onKind(c.kind)}
            className={cx('cursor-pointer rounded-md border px-3 py-2 text-left transition-colors',
              active ? 'border-accent bg-accent-soft/50' : 'border-line bg-surface hover:border-accent/50 hover:bg-accent-soft/30')}>
            <div className="truncate text-[12px] font-medium text-ink-3">{c.label}</div>
            <div className={cx('mt-0.5 text-[20px] leading-tight font-semibold tabular-nums', value ? c.tone : 'text-ink-3')}>
              {value ?? <span className="inline-block h-5 w-6 animate-pulse rounded bg-surface-2" />}
            </div>
          </button>
        )
      })}
    </div>
  )
}
