import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { Badge, Button, Field, Input, Modal, Mono, SkeletonRows } from '../../components/ui'
import { Tabs, useChecks } from '../../components/workflow'
import { fieldIssues, get, post } from '../../lib/api'
import { between, cidr as cidrIssue, ip as ipIssue, issues as check } from '../../lib/validation'

interface Located { subnetId: string; subnetName: string; cidr: string; routingDomainName: string }

/** Split, summarise and locate. Nothing here changes inventory. */
export function ToolsModal({ onClose, onOpenSubnet }: { onClose: () => void; onOpenSubnet: (id: string) => void }) {
  const [tab, setTab] = useState('split')
  const [cidr, setCidr] = useState('')
  const [prefix, setPrefix] = useState('')
  const [many, setMany] = useState('')
  const [address, setAddress] = useState('')
  const split = useMutation({ meta: { inlineError: true }, mutationFn: () => post<{ subnets: string[]; total: string; truncated: boolean }>('/tools/split', { cidr, newPrefix: Number(prefix) }) })
  const summarise = useMutation({ meta: { inlineError: true }, mutationFn: () => post<{ summary: string[] }>('/tools/summarise', { cidrs: many.split(/[\s,]+/).filter(Boolean) }) })
  const locate = useMutation({ meta: { inlineError: true }, mutationFn: () => get<Located[]>(`/tools/locate?address=${encodeURIComponent(address.trim())}`) })
  const splitChecks = useChecks(check({ cidr: cidrIssue(cidr), newPrefix: between(prefix, 0, 128, 'New prefix') }))
  const locateChecks = useChecks(check({ address: ipIssue(address) }))
  return (
    <Modal open onClose={onClose} size="lg" title="Subnet tools" subtitle="Calculations only. Nothing here changes inventory.">
      <Tabs tabs={[{ key: 'split', label: 'Split' }, { key: 'summarise', label: 'Summarise' }, { key: 'locate', label: 'Locate an address' }]} active={tab} onChange={setTab} />
      {tab === 'split' && (
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); splitChecks.submit(() => split.mutate()) }}>
          <div className="grid grid-cols-[1fr_8rem_auto] items-end gap-2">
            <Field label="CIDR" htmlFor="t-cidr" error={splitChecks.shown.cidr ?? fieldIssues(split.error).cidr}><Input id="t-cidr" className="font-mono" value={cidr} onChange={(e) => setCidr(e.target.value)} placeholder="10.0.0.0/22" /></Field>
            <Field label="New prefix" htmlFor="t-prefix" error={splitChecks.shown.newPrefix ?? fieldIssues(split.error).newPrefix}><Input id="t-prefix" type="number" value={prefix} onChange={(e) => setPrefix(e.target.value)} placeholder="24" /></Field>
            <Button type="submit" variant="primary" busy={split.isPending}>Split</Button>
          </div>
          {split.data && <><p className="text-[13px] text-ink-2">{split.data.total} subnets{split.data.truncated && `, showing the first ${split.data.subnets.length}`}.</p>
            <div className="flex max-h-56 flex-wrap gap-1 overflow-y-auto">{split.data.subnets.map((s) => <Badge key={s}><Mono>{s}</Mono></Badge>)}</div></>}
        </form>
      )}
      {tab === 'summarise' && (
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); summarise.mutate() }}>
          <Field label="CIDRs" htmlFor="t-many" hint="Separated by spaces, commas or new lines." error={fieldIssues(summarise.error).cidr ?? fieldIssues(summarise.error).cidrs}>
            <textarea id="t-many" rows={5} value={many} onChange={(e) => setMany(e.target.value)} spellCheck={false}
              className="w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 py-1.5 font-mono text-[12.5px] focus:border-accent focus:outline-none" />
          </Field>
          <Button type="submit" variant="primary" busy={summarise.isPending}>Summarise</Button>
          {summarise.data && <div className="flex flex-wrap gap-1">{summarise.data.summary.map((s) => <Badge key={s} tone="accent"><Mono>{s}</Mono></Badge>)}</div>}
        </form>
      )}
      {tab === 'locate' && (
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); locateChecks.submit(() => locate.mutate()) }}>
          <div className="grid grid-cols-[1fr_auto] items-end gap-2">
            <Field label="Address" htmlFor="t-addr" error={locateChecks.shown.address ?? fieldIssues(locate.error).address}><Input id="t-addr" className="font-mono" value={address} onChange={(e) => setAddress(e.target.value)} /></Field>
            <Button type="submit" variant="primary" busy={locate.isPending}>Locate</Button>
          </div>
          {locate.isPending && <SkeletonRows rows={2} />}
          {locate.data && (locate.data.length === 0 ? <p className="text-[13px] text-ink-3">No active subnet holds this address in any routing domain.</p> : (
            <ul>{locate.data.map((m) => (
              <li key={m.subnetId} className="flex items-center justify-between border-b border-line py-1.5 text-[13px] last:border-0">
                <span><Mono>{m.cidr}</Mono> {m.subnetName} <span className="text-ink-3">· {m.routingDomainName}</span></span>
                <Button variant="ghost" className="h-7" onClick={() => onOpenSubnet(m.subnetId)}>Open</Button>
              </li>
            ))}</ul>
          ))}
          <p className="text-[12px] text-ink-3">The same private address can sit in several routing domains, so each one is listed.</p>
        </form>
      )}
    </Modal>
  )
}
