import { useMutation } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Badge, Button, Field, FormError, Input, Modal, Mono } from '../../components/ui'
import { get, post } from '../../lib/api'

const tabs = { split: 'Split', summarise: 'Summarise', locate: 'Locate an address' } as const
type Tab = keyof typeof tabs

interface Located { subnetId: string; subnetName: string; cidr: string; routingDomainName: string }

function Split() {
  const [cidr, setCidr] = useState('')
  const [prefix, setPrefix] = useState('')
  const run = useMutation({ mutationFn: () => post<{ subnets: string[]; total: string; truncated: boolean }>('/tools/split', { cidr, newPrefix: Number(prefix) }) })
  const submit = (e: FormEvent) => { e.preventDefault(); run.mutate() }
  return (
    <form onSubmit={submit} className="space-y-3">
      <div className="grid grid-cols-[1fr_8rem_auto] items-end gap-2">
        <Field label="CIDR" htmlFor="split-cidr"><Input id="split-cidr" className="font-mono" placeholder="10.0.0.0/22" required value={cidr} onChange={(e) => setCidr(e.target.value)} /></Field>
        <Field label="New prefix" htmlFor="split-prefix"><Input id="split-prefix" inputMode="numeric" placeholder="24" required value={prefix} onChange={(e) => setPrefix(e.target.value)} /></Field>
        <Button type="submit" variant="primary" busy={run.isPending}>Split</Button>
      </div>
      <FormError error={run.error} />
      {run.data && (
        <>
          <p className="text-[13px] text-ink-2">{BigInt(run.data.total).toLocaleString('en-GB')} subnets{run.data.truncated ? `, showing the first ${run.data.subnets.length}` : ''}.</p>
          <ul className="flex max-h-64 flex-wrap gap-1 overflow-y-auto">{run.data.subnets.map((s) => <li key={s}><Badge><span className="font-mono">{s}</span></Badge></li>)}</ul>
        </>
      )}
    </form>
  )
}

function Summarise() {
  const [text, setText] = useState('')
  const cidrs = text.split(/[\s,]+/).filter(Boolean)
  const run = useMutation({ mutationFn: () => post<{ summary: string[] }>('/tools/summarise', { cidrs }) })
  const submit = (e: FormEvent) => { e.preventDefault(); run.mutate() }
  return (
    <form onSubmit={submit} className="space-y-3">
      <Field label="CIDRs" htmlFor="sum-cidrs" hint="One per line, or separated by commas or spaces.">
        <textarea id="sum-cidrs" rows={6} required value={text} onChange={(e) => setText(e.target.value)}
          className="w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 py-2 font-mono text-[13px] text-ink focus:border-accent focus:bg-surface focus:outline-none" />
      </Field>
      <Button type="submit" variant="primary" busy={run.isPending}>Summarise {cidrs.length > 0 ? cidrs.length : ''}</Button>
      <FormError error={run.error} />
      {run.data && (
        <div>
          <p className="mb-1 text-[13px] text-ink-2">Covered by {run.data.summary.length} prefix{run.data.summary.length === 1 ? '' : 'es'}:</p>
          <ul className="flex flex-wrap gap-1">{run.data.summary.map((s) => <li key={s}><Badge tone="accent"><span className="font-mono">{s}</span></Badge></li>)}</ul>
        </div>
      )}
    </form>
  )
}

function Locate({ onOpen }: { onOpen: (subnetId: string) => void }) {
  const [address, setAddress] = useState('')
  const run = useMutation({ mutationFn: () => get<Located[]>(`/tools/locate?address=${encodeURIComponent(address.trim())}`) })
  const submit = (e: FormEvent) => { e.preventDefault(); run.mutate() }
  return (
    <form onSubmit={submit} className="space-y-3">
      <div className="grid grid-cols-[1fr_auto] items-end gap-2">
        <Field label="Address" htmlFor="locate-address" hint="Finds the most specific subnet in every routing domain."><Input id="locate-address" className="font-mono" required value={address} onChange={(e) => setAddress(e.target.value)} /></Field>
        <Button type="submit" variant="primary" busy={run.isPending}>Locate</Button>
      </div>
      <FormError error={run.error} />
      {run.data?.length === 0 && <p className="text-ink-3">No active subnet holds this address.</p>}
      <ul>
        {run.data?.map((m) => (
          <li key={m.subnetId} className="flex justify-between gap-3 border-b border-line py-1.5 last:border-0">
            <button type="button" className="cursor-pointer text-left font-medium text-ink hover:text-accent" onClick={() => onOpen(m.subnetId)}>{m.subnetName} <Mono>{m.cidr}</Mono></button>
            <span className="text-ink-2">{m.routingDomainName}</span>
          </li>
        ))}
      </ul>
    </form>
  )
}

/** Split, summarise and locate. They only calculate: nothing is saved. */
export function Tools({ onClose, onOpen }: { onClose: () => void; onOpen: (subnetId: string) => void }) {
  const [tab, setTab] = useState<Tab>('split')
  return (
    <Modal open onClose={onClose} size="lg" title="Subnet tools" subtitle="Calculations only. Nothing is saved.">
      <div role="tablist" className="mb-4 flex gap-1 border-b border-line">
        {(Object.keys(tabs) as Tab[]).map((t) => (
          <button key={t} role="tab" type="button" aria-selected={tab === t} onClick={() => setTab(t)}
            className={`-mb-px cursor-pointer border-b-2 px-3 py-1.5 text-[13px] ${tab === t ? 'border-accent font-medium text-ink' : 'border-transparent text-ink-2 hover:text-ink'}`}>{tabs[t]}</button>
        ))}
      </div>
      {tab === 'split' && <Split />}
      {tab === 'summarise' && <Summarise />}
      {tab === 'locate' && <Locate onOpen={(id) => { onClose(); onOpen(id) }} />}
    </Modal>
  )
}
