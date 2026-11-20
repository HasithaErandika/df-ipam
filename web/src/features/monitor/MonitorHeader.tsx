import { CheckCircle2, Maximize2, Minimize2, X } from 'lucide-react'
import { LogoMark } from '../../components/LogoMark'

export interface Problem { n: number; text: string }

/**
 * The monitor's top band: who it is, whether anything needs attention (each
 * problem named), and the time in Sri Lanka.
 */
export function MonitorHeader({ now, problems, loading, full, onFull, onClose }: {
  now: Date; problems: Problem[]; loading: boolean; full: boolean; onFull: () => void; onClose: () => void
}) {
  const total = problems.reduce((n, p) => n + p.n, 0)
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
        <div className="mx-auto flex min-w-0 flex-wrap items-center justify-center gap-2" role="status" aria-live="polite">
          {loading ? <span className="text-[13px] text-ink-3">Loading…</span>
            : total === 0
              ? <span className="flex items-center gap-2 rounded-full bg-ok-soft px-2.5 py-1 text-[12px] font-medium text-ok"><CheckCircle2 size={13} aria-hidden />All systems normal</span>
              : <>
                <span className="flex items-center gap-2 rounded-full bg-danger-soft px-2.5 py-1 text-[12px] font-semibold text-danger">
                  <span className="relative flex size-2"><span className="absolute inline-flex size-full animate-ping rounded-full bg-danger opacity-60" /><span className="relative inline-flex size-2 rounded-full bg-danger" /></span>
                  {total} need{total === 1 ? 's' : ''} attention
                </span>
                {problems.filter((p) => p.n > 0).map((p) => <span key={p.text} className="rounded-full border border-line bg-surface-2 px-2 py-0.5 text-[11.5px] text-ink-2"><strong className="text-ink tabular-nums">{p.n}</strong> {p.text}</span>)}
              </>}
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
      <div className="h-0.5 bg-gradient-to-r from-[#1f4fae] via-accent to-[#8fb3f7]" aria-hidden />
    </header>
  )
}
