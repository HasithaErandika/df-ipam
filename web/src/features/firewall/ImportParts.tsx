import { CheckCircle2, DownloadCloud, RefreshCw, X, XCircle } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button, Card, FormError, Notice } from '../../components/ui'
import { cx } from '../../components/ui/cx'
import { formatAgo } from '../../lib/format'
import { notConnected } from './LiveView'

const steps = ['Fetch what the firewall has', 'Choose what to bring in', 'Confirm, and it is saved']

/** Before anything is fetched: what this does, in three steps, and the one button. */
export function StartPanel({ busy, error, onFetch }: { busy: boolean; error: unknown; onFetch: () => void }) {
  return (
    <Card>
      <div className="flex flex-col items-center px-6 py-12 text-center">
        <span aria-hidden className="mb-3 grid size-11 place-items-center rounded-lg bg-accent-soft text-accent"><DownloadCloud size={22} /></span>
        <h2 className="text-[15px] font-semibold text-ink">Import from the FortiGate</h2>
        <p className="mt-1 max-w-lg text-[13px] text-ink-2">Networks, devices, servers and network links the firewall knows, compared with DF-IPAM. The firewall is only read; nothing is saved until you confirm.</p>
        <ol className="mt-5 flex flex-wrap justify-center gap-2">
          {steps.map((s, i) => (
            <li key={s} className="flex items-center gap-2 rounded-full border border-line bg-surface-2 py-1 pr-3 pl-1 text-[12.5px] text-ink-2">
              <span className="grid size-5 place-items-center rounded-full bg-surface text-[11px] font-semibold text-ink">{i + 1}</span>{s}
            </li>
          ))}
        </ol>
        <Button variant="primary" className="mt-6" busy={busy} onClick={onFetch}><RefreshCw size={14} aria-hidden />Fetch from firewall</Button>
        {error ? <div className="mt-4 w-full max-w-lg text-left"><FetchError error={error} /></div> : null}
      </div>
    </Card>
  )
}

export function FetchError({ error }: { error: unknown }) {
  return notConnected(error)
    ? <Notice tone="warn">No firewall is connected. An administrator connects it in System &gt; FortiGate.</Notice>
    : <FormError error={error} />
}

/** After a fetch: which firewall, when, and fetching again. */
export function SourceBar({ firewall, at, busy, onFetch, problems }: { firewall: string; at: string; busy: boolean; onFetch: () => void; problems: string[] }) {
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-[13px]">
        <span className="inline-flex items-center gap-2 text-ink-2">
          <span aria-hidden className="size-2 rounded-full bg-ok" />Read from <strong className="font-medium text-ink">{firewall}</strong> {formatAgo(at)}
        </span>
        <Button variant="ghost" busy={busy} onClick={onFetch}><RefreshCw size={13} aria-hidden />Fetch again</Button>
      </div>
      {problems.length > 0 && <Notice tone="warn">Not everything could be read: {problems.join(' · ')}.</Notice>}
    </div>
  )
}

export interface KindTab { value: string; text: string; count: number | null }

/** One tab per kind of record, each with how many items wait for a decision. */
export function KindTabs({ value, onChange, tabs }: { value: string; onChange: (v: string) => void; tabs: KindTab[] }) {
  return (
    <div role="tablist" aria-label="What to import" className="flex gap-1 overflow-x-auto border-b border-line">
      {tabs.map((t) => (
        <button key={t.value} type="button" role="tab" aria-selected={value === t.value} onClick={() => onChange(t.value)}
          className={cx('-mb-px inline-flex h-9 shrink-0 cursor-pointer items-center gap-2 border-b-2 px-3 text-[13px] font-medium whitespace-nowrap',
            value === t.value ? 'border-accent text-ink' : 'border-transparent text-ink-3 hover:text-ink')}>
          {t.text}
          {t.count !== null && <span className={cx('min-w-5 rounded-full px-1.5 text-[11px] leading-[18px] tabular-nums',
            t.count > 0 ? 'bg-accent-soft text-accent' : 'bg-surface-2 text-ink-3')}>{t.count}</span>}
        </button>
      ))}
    </div>
  )
}

/** Above a list: show only what needs a decision, or everything; and choose all shown. */
export function ListTools({ all, open, onlyOpen, setOnlyOpen, allChosen, chooseAll, children }: {
  all: number; open: number; onlyOpen: boolean; setOnlyOpen: (v: boolean) => void; allChosen: boolean; chooseAll: (on: boolean) => void; children?: ReactNode
}) {
  const opt = (on: boolean, text: string, n: number) => (
    <button type="button" role="radio" aria-checked={onlyOpen === on} onClick={() => setOnlyOpen(on)}
      className={cx('h-[26px] cursor-pointer rounded-[4px] px-2.5 text-[12.5px] font-medium', onlyOpen === on ? 'bg-surface text-ink shadow-[0_1px_2px_rgba(11,18,32,.12)]' : 'text-ink-3 hover:text-ink')}>
      {text} <span className="tabular-nums text-ink-3">{n}</span>
    </button>
  )
  return (
    <div className="flex flex-wrap items-center gap-3 border-b border-line px-4 py-2.5">
      <label className="flex cursor-pointer items-center gap-2 text-[13px] text-ink-2">
        <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={allChosen} disabled={open === 0} onChange={(e) => chooseAll(e.target.checked)} />Choose all
      </label>
      {children}
      <div role="radiogroup" aria-label="Show" className="ml-auto inline-flex h-8 items-center rounded-[5px] border border-line-strong bg-surface-2 p-0.5">
        {opt(true, 'To review', open)}{opt(false, 'All', all)}
      </div>
    </div>
  )
}

/** The one place to save, stuck to the bottom of the screen while something is chosen. */
export function SaveBar({ count, noun, note, busy, disabled, onClear, onSave }: {
  count: number; noun: string; note?: ReactNode; busy: boolean; disabled?: boolean; onClear: () => void; onSave: () => void
}) {
  if (count === 0) return null
  return (
    <div className="sticky bottom-0 z-[2] -mx-px flex flex-wrap items-center gap-3 rounded-b-md border-t border-line bg-surface px-4 py-3 shadow-[0_-8px_24px_-16px_rgba(11,18,32,.3)]">
      <span className="text-[13px] text-ink"><strong className="font-semibold tabular-nums">{count}</strong> {noun} chosen</span>
      <button type="button" className="cursor-pointer text-[12.5px] text-ink-3 hover:text-ink hover:underline" onClick={onClear}>Clear</button>
      {note && <span className="text-[12.5px] text-warn">{note}</span>}
      <Button variant="primary" className="ml-auto" busy={busy} disabled={disabled} onClick={onSave}>Save {count}…</Button>
    </div>
  )
}

export interface SavedItem { key: string; label: ReactNode; ok: boolean; error?: string }

/** What the last save did, item by item, until it is dismissed. */
export function SavedResults({ items, onClose }: { items: SavedItem[]; onClose: () => void }) {
  const failed = items.filter((i) => !i.ok).length
  return (
    <Card title={failed ? `${items.length - failed} saved, ${failed} failed` : `${items.length} saved`}
      action={<button type="button" aria-label="Dismiss" className="grid size-7 cursor-pointer place-items-center rounded-md text-ink-3 hover:bg-surface-2 hover:text-ink" onClick={onClose}><X size={15} /></button>}>
      <ul className="max-h-56 divide-y divide-line overflow-y-auto">
        {items.map((r) => (
          <li key={r.key} className="flex flex-wrap items-center gap-2 px-4 py-2 text-[13px]">
            {r.ok ? <CheckCircle2 size={15} className="text-ok" aria-hidden /> : <XCircle size={15} className="text-danger" aria-hidden />}
            <span className="text-ink">{r.label}</span>{r.error && <span className="text-danger">· {r.error}</span>}
          </li>
        ))}
      </ul>
    </Card>
  )
}
