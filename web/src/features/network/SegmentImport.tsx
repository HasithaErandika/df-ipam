import { useMutation } from '@tanstack/react-query'
import { Check, FileText, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { Badge, Button, Field, FormError, Modal, Notice, Select, Switch, Textarea } from '../../components/ui'
import { cx } from '../../components/ui/cx'
import { post } from '../../lib/api'
import { inventoryKeys, useInvalidate } from '../../lib/queries'

interface Row { line: number; name: string; network: string; gateway: string; vlan: number | null; firewall: string; down: boolean; add: boolean; reason: string }
interface Result { dryRun: boolean; added: number; skipped: number; rows: Row[] }

const kinds = [
  { value: 'server', text: 'Server networks (static, swept)' },
  { value: 'cloud', text: 'Cloud networks (static, never swept)' },
  { value: 'dhcp', text: 'DHCP networks (set each pool afterwards)' },
  { value: 'vpn', text: 'VPN client pools (never swept)' },
]

const th = 'sticky top-0 z-[1] bg-surface-2 px-2.5 py-1.5 text-left text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase'

function Rows({ rows, leftOut, onToggle, showSkipped }: { rows: Row[]; leftOut: Set<number>; onToggle?: (line: number) => void; showSkipped: boolean }) {
  const shown = rows.filter((r) => showSkipped || r.add)
  if (shown.length === 0) return <p className="py-6 text-center text-[13px] text-ink-3">Nothing to show.</p>
  return (
    <div className="max-h-[50vh] overflow-auto rounded-md border border-line">
      <table className="w-full text-[12.5px]">
        <thead><tr>{onToggle && <th className={cx(th, 'w-8')} />}<th className={th}>Line</th><th className={th}>Name</th><th className={th}>Network</th><th className={th}>Gateway</th><th className={th}>VLAN</th><th className={th}>Result</th></tr></thead>
        <tbody>{shown.map((r) => {
          const out = leftOut.has(r.line)
          return (
            <tr key={r.line} className={cx('border-t border-line', (!r.add || out) && 'text-ink-3')}>
              {onToggle && (
                <td className="px-2.5 py-1.5">{r.add && (
                  <button type="button" role="checkbox" aria-checked={!out} aria-label={`Add ${r.name}`} onClick={() => onToggle(r.line)}
                    className={cx('grid size-4 cursor-pointer place-items-center rounded-[3px] border', out ? 'border-line-strong bg-surface' : 'border-accent bg-accent text-white')}>
                    {!out && <Check size={12} strokeWidth={3} aria-hidden />}
                  </button>
                )}</td>
              )}
              <td className="px-2.5 py-1.5 tabular-nums">{r.line}</td>
              <td className="px-2.5 py-1.5"><span className={r.add && !out ? 'font-medium text-ink' : undefined}>{r.name}</span>{r.firewall && <div className="text-[11px] text-ink-3">{r.firewall}</div>}</td>
              <td className="px-2.5 py-1.5 font-mono">{r.network || '—'}</td>
              <td className="px-2.5 py-1.5 font-mono">{r.gateway || '—'}</td>
              <td className="px-2.5 py-1.5 tabular-nums">{r.vlan ?? '—'}</td>
              <td className="px-2.5 py-1.5">
                {r.add && !out ? <Badge tone="ok">Add</Badge> : <span className="text-[12px]">{out ? 'Left out' : r.reason}</span>}
                {r.down && <span className="ml-1.5"><Badge tone="warn">Interface down</Badge></span>}
              </td>
            </tr>
          )
        })}</tbody>
      </table>
    </div>
  )
}

/**
 * Adds segments from text: FortiGate "show system interface" output, or lines
 * with a CIDR and a name. Review shows every network and what happens to it;
 * nothing is saved until Add is pressed.
 */
export function SegmentImport() {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const [kind, setKind] = useState('server')
  const [leftOut, setLeftOut] = useState<Set<number>>(new Set())
  const [showSkipped, setShowSkipped] = useState(true)
  const file = useRef<HTMLInputElement>(null)
  const refresh = useInvalidate(...inventoryKeys)
  const review = useMutation({ mutationFn: () => post<Result>('/segments/import?dryRun=true', { text, kind }), meta: { inlineError: true }, onSuccess: () => setLeftOut(new Set()) })
  const save = useMutation({
    mutationFn: () => post<Result>('/segments/import', { text, kind, skip: [...leftOut] }),
    meta: { inlineError: true, success: (_: void, r: Result) => `${r.added} segment${r.added === 1 ? '' : 's'} added.` },
    onSuccess: () => void refresh(),
  })
  const close = () => { setOpen(false); setText(''); setLeftOut(new Set()); review.reset(); save.reset() }
  const toggle = (line: number) => setLeftOut((s) => { const n = new Set(s); if (n.has(line)) n.delete(line); else n.add(line); return n })
  const plan = review.data
  const toAdd = plan ? plan.rows.filter((r) => r.add && !leftOut.has(r.line)).length : 0
  const done = save.data

  let footer
  if (done) footer = <Button variant="primary" onClick={close}>Done</Button>
  else if (plan) {
    footer = <><Button onClick={() => review.reset()}>Back</Button>
      <Button variant="primary" disabled={toAdd === 0} busy={save.isPending} onClick={() => save.mutate()}>Add {toAdd} {toAdd === 1 ? 'segment' : 'segments'}</Button></>
  } else {
    footer = <><Button onClick={close}>Cancel</Button><Button variant="primary" disabled={!text.trim()} busy={review.isPending} onClick={() => review.mutate()}>Review</Button></>
  }

  return (
    <>
      <Button onClick={() => setOpen(true)}><Upload size={14} aria-hidden />Import from text</Button>
      <Modal open={open} onClose={close} size="xl" title="Import segments from text" footer={footer}
        subtitle={done ? 'Imported.' : plan ? 'Step 2 of 2: check each network, untick any to leave out, then add.' : 'Step 1 of 2: paste the text or choose a file.'}>
        {done ? (
          <div className="space-y-3">
            <Notice tone={done.added ? 'ok' : 'warn'}><strong>{done.added}</strong> added, <strong>{done.skipped}</strong> skipped. Each one is in the audit log.</Notice>
            <Rows rows={done.rows} leftOut={new Set()} showSkipped />
          </div>
        ) : plan ? (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-3">
              <Notice tone={toAdd ? 'ok' : 'warn'}><strong>{toAdd}</strong> will be added as {kinds.find((k) => k.value === kind)?.text.split(' (')[0].toLowerCase()}, <strong>{plan.rows.length - toAdd}</strong> skipped. Nothing is saved yet.</Notice>
              <Switch id="si-skipped" checked={showSkipped} onChange={setShowSkipped} label="Show skipped" />
            </div>
            <Rows rows={plan.rows} leftOut={leftOut} onToggle={toggle} showSkipped={showSkipped} />
            <FormError error={save.error} />
          </div>
        ) : (
          <div className="space-y-4">
            <p className="text-ink-2">
              Paste FortiGate <code className="font-mono">show system interface</code> output, or one network per line such as <code className="font-mono">Floor 3 192.168.3.0/24</code>.
              An interface address becomes the segment's gateway. Tunnels, loopbacks and single addresses are skipped, and so is anything that overlaps an existing segment.
            </p>
            <Field label="Text" htmlFor="si-text">
              <Textarea id="si-text" rows={12} className="font-mono text-[12px]" value={text} onChange={(e) => setText(e.target.value)}
                placeholder={'FW-01 $ show system interface\nSub_vlan_3   static   0.0.0.0 0.0.0.0  172.20.14.14 255.255.255.248  up   disable   vlan  disable'} />
            </Field>
            <div className="flex flex-wrap items-end gap-3">
              <Button onClick={() => file.current?.click()}><FileText size={14} aria-hidden />Choose a .txt file</Button>
              <input ref={file} type="file" className="sr-only" accept=".txt,.log,.conf,.cfg,text/plain"
                onChange={async (e) => { const f = e.target.files?.[0]; if (f) setText(await f.text()); e.target.value = '' }} />
              <div className="min-w-64 flex-1">
                <Field label="Add them as" htmlFor="si-kind"><Select id="si-kind" value={kind} onChange={(e) => setKind(e.target.value)}>{kinds.map((k) => <option key={k.value} value={k.value}>{k.text}</option>)}</Select></Field>
              </div>
            </div>
            <FormError error={review.error} />
          </div>
        )}
      </Modal>
    </>
  )
}
