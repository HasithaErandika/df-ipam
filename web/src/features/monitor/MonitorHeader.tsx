import { AlertTriangle, CheckCircle2, Maximize2, Minimize2, X } from 'lucide-react'
import { LogoMark } from '../../components/LogoMark'

export interface Problem { n: number; text: string }

/**
 * The monitor's top band: who it is, whether anything needs attention (each
 * problem named), and the time in Sri Lanka.
 */
export function MonitorHeader({ now, problems, loading, full, onFull, onClose }: {
  now: Date; problems: Problem[]; loading: boolean; full: boolean; onFull: () => void; onClose: () => void
}) {
  const opts = { timeZone: 'Asia/Colombo' } as const
  const hm = now.toLocaleTimeString('en-GB', { ...opts, hour: '2-digit', minute: '2-digit' })
  const sec = now.toLocaleTimeString('en-GB', { ...opts, second: '2-digit' }).padStart(2, '0')
  const day = now.toLocaleDateString('en-GB', { ...opts, weekday: 'short', day: 'numeric', month: 'short' })
  const iconBtn = 'grid size-7 cursor-pointer place-items-center rounded-lg text-ink-3 transition-colors hover:bg-surface-2 hover:text-ink'
  return (
    <header className="overflow-hidden rounded-xl border border-line bg-surface">
      <div className="flex flex-wrap items-center gap-3 px-4 py-2">
        <div className="flex items-center gap-3">
          <LogoMark size={26} />
          <div className="text-[13.5px] leading-none">
            <span className="font-semibold text-ink">DF-IPAM</span> <span className="text-ink-3">· Network monitor</span>
          </div>
        </div>
        <div className="mx-auto flex min-w-0 justify-center" role="status" aria-live="polite">
          {loading ? <span className="text-[13px] text-ink-3">Loading…</span> : <Status problems={problems} />}
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-baseline gap-2 text-[12px] text-ink-3">
            <span>{day}</span>
            <span className="text-[16px] leading-none font-semibold text-ink tabular-nums">{hm}<span className="text-[11px] font-medium text-ink-3">:{sec}</span></span>
          </div>
          <div className="flex gap-0.5 border-l border-line pl-2">
            <button type="button" className={iconBtn} onClick={onFull} title={full ? 'Exit full screen' : 'Full screen'} aria-label={full ? 'Exit full screen' : 'Full screen'}>
              {full ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
            </button>
            <button type="button" className={iconBtn} onClick={onClose} title="Close the monitor" aria-label="Close the monitor"><X size={15} /></button>
          </div>
        </div>
      </div>
      <div className="h-0.5 bg-gradient-to-r from-brand-deep via-accent to-brand-light" aria-hidden />
    </header>
  )
}

/**
 * The overall state in one strip: a quiet "all normal", or a labelled band that
 * names each problem with its count, worst first.
 */
function Status({ problems }: { problems: Problem[] }) {
  const open = problems.filter((p) => p.n > 0)
  if (open.length === 0) {
    return (
      <span className="inline-flex items-center gap-2 rounded-lg border border-ok/25 bg-ok-soft/60 px-3 py-1.5 text-[12.5px] font-medium text-ok">
        <CheckCircle2 size={14} aria-hidden />All systems normal
      </span>
    )
  }
  return (
    <div className="flex min-w-0 items-stretch overflow-hidden rounded-lg border border-danger/25 bg-surface text-[12.5px]">
      <span className="flex items-center gap-1.5 border-l-[3px] border-danger bg-danger-soft px-3 font-semibold text-danger">
        <AlertTriangle size={14} aria-hidden />Needs attention
      </span>
      <ul className="flex min-w-0 flex-wrap items-center divide-x divide-line">
        {open.map((p) => (
          <li key={p.text} className="flex items-baseline gap-1.5 px-3 py-1.5">
            <span className="text-[15px] leading-none font-semibold text-danger tabular-nums">{p.n}</span>
            <span className="text-ink-2">{p.text}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
