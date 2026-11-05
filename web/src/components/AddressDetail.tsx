import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { ApiError, get, list, listAll, patch, post } from '../lib/api'
import { environments, formatDateTime, humanAction } from '../lib/format'
import { optionLabel, useOptions } from '../lib/hooks'
import { useCan } from '../lib/session'
import type { Change, DnsRecord, Evidence, IpAddress, RoutingDomain } from '../lib/types'
import { allocationStates, StateBadges } from './inventory'
import { Button, ErrorState, Field, FormError, Input, Modal, Mono, Select, SkeletonRows } from './ui'

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-4 border-b border-line py-1.5 last:border-0">
      <dt className="text-ink-2">{label}</dt>
      <dd className="text-right text-ink">{children}</dd>
    </div>
  )
}

const heading = 'mb-1 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase'
const tabs = { details: 'Details', edit: 'Edit', history: 'History', evidence: 'Evidence' } as const
type Tab = keyof typeof tabs

function invalidate(qc: ReturnType<typeof useQueryClient>) {
  qc.invalidateQueries({ queryKey: ['ip-addresses'] })
  qc.invalidateQueries({ queryKey: ['subnets'] })
  qc.invalidateQueries({ queryKey: ['summary'] })
}

function EditForm({ a, onDone }: { a: IpAddress; onDone: () => void }) {
  const qc = useQueryClient()
  const mayAllocate = useCan('ip:allocate')
  const mayReserve = useCan('ip:reserve')
  const canAllocate = mayAllocate || mayReserve
  const owners = useOptions('/users/lookup')
  const services = useOptions('/business-services')
  const apps = useOptions('/applications')
  const devices = useOptions('/devices')
  const [v, setV] = useState({
    hostname: a.hostname, fqdn: a.fqdn, mac: a.mac ?? '', notes: a.notes, ownerId: a.ownerId ?? '',
    businessServiceId: a.businessServiceId ?? '', applicationId: a.applicationId ?? '', deviceId: a.deviceId ?? '',
    allocationState: a.allocationState, reason: '',
  })
  const set = (k: keyof typeof v) => (e: { target: { value: string } }) => setV({ ...v, [k]: e.target.value })
  const releasing = v.allocationState !== a.allocationState && (a.allocationState === 'allocated' || a.allocationState === 'reserved')
  const save = useMutation({
    mutationFn: () => {
      const { allocationState, reason, ...rest } = v
      const body: Record<string, string> = { ...rest }
      if (allocationState !== a.allocationState) Object.assign(body, { allocationState, reason })
      return patch<IpAddress>(`/ip-addresses/${a.id}`, body)
    },
    onSuccess: () => { invalidate(qc); onDone() },
  })
  const issue = (field: string) => save.error instanceof ApiError ? save.error.details.find((d) => d.field === field)?.issue : undefined
  const submit = (e: FormEvent) => { e.preventDefault(); save.mutate() }
  return (
    <form onSubmit={submit} className="space-y-3.5" noValidate>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Hostname" htmlFor="ip-hostname"><Input id="ip-hostname" value={v.hostname} onChange={set('hostname')} /></Field>
        <Field label="FQDN" htmlFor="ip-fqdn"><Input id="ip-fqdn" value={v.fqdn} onChange={set('fqdn')} /></Field>
        <Field label="MAC address" htmlFor="ip-mac" error={issue('mac')}><Input id="ip-mac" className="font-mono" value={v.mac} onChange={set('mac')} /></Field>
        <Field label="Owner" htmlFor="ip-owner">
          <Select id="ip-owner" value={v.ownerId} onChange={set('ownerId')}><option value="">None</option>{owners.data?.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}</Select>
        </Field>
        <Field label="Business service" htmlFor="ip-service">
          <Select id="ip-service" value={v.businessServiceId} onChange={set('businessServiceId')}><option value="">None</option>{services.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select>
        </Field>
        <Field label="Application" htmlFor="ip-app">
          <Select id="ip-app" value={v.applicationId} onChange={set('applicationId')}><option value="">None</option>{apps.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select>
        </Field>
        <Field label="Device" htmlFor="ip-device">
          <Select id="ip-device" value={v.deviceId} onChange={set('deviceId')}><option value="">None</option>{devices.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select>
        </Field>
        <Field label="Allocation state" htmlFor="ip-state" hint={canAllocate ? undefined : 'Changing allocation needs allocate or reserve permission.'}>
          <Select id="ip-state" value={v.allocationState} disabled={!canAllocate} onChange={set('allocationState')}>
            {Object.entries(allocationStates).map(([k, label]) => <option key={k} value={k}>{label}</option>)}
          </Select>
        </Field>
      </div>
      {releasing && (
        <Field label="Reason for release" htmlFor="ip-reason" error={issue('reason')} hint="Kept in the audit history.">
          <Input id="ip-reason" required value={v.reason} onChange={set('reason')} />
        </Field>
      )}
      <Field label="Notes" htmlFor="ip-notes"><Input id="ip-notes" value={v.notes} onChange={set('notes')} /></Field>
      {!(save.error instanceof ApiError && save.error.details.length > 0) && <FormError error={save.error} />}
      <div className="flex justify-end gap-2"><Button onClick={onDone}>Cancel</Button><Button variant="primary" type="submit" busy={save.isPending}>Save changes</Button></div>
    </form>
  )
}

/** One IP record: its details, edit form, change history and raw evidence. */
export function AddressDetail({ id, onClose }: { id: string | null; onClose: () => void }) {
  const canEdit = useCan('inventory:edit')
  const [tab, setTab] = useState<Tab>('details')
  useEffect(() => setTab('details'), [id])
  const record = useQuery({ queryKey: ['ip-addresses', 'one', id], enabled: id !== null, queryFn: () => get<IpAddress>(`/ip-addresses/${id}`) })
  const history = useQuery({ queryKey: ['ip-addresses', 'history', id], enabled: id !== null && tab === 'history', queryFn: () => get<Change[]>(`/ip-addresses/${id}/history`) })
  const evidence = useQuery({ queryKey: ['ip-addresses', 'evidence', id], enabled: id !== null && tab === 'evidence', queryFn: () => get<Evidence[]>(`/ip-addresses/${id}/evidence`) })
  const dns = useQuery({ queryKey: ['dns', id], enabled: id !== null && tab === 'details', queryFn: () => list<DnsRecord>(`/dns-records?ipAddressId=${id}&pageSize=50`) })
  if (id === null) return null
  const a = record.data
  return (
    <Modal open onClose={onClose} size="xl" title={a?.address ?? 'IP record'}
      subtitle={a ? <span>{a.subnetName} · <Mono>{a.subnetCidr}</Mono> · {a.routingDomainName}</span> : undefined}>
      {record.isPending && <SkeletonRows rows={6} />}
      {record.error && <ErrorState error={record.error} onRetry={record.refetch} />}
      {a && (
        <>
          <div role="tablist" className="mb-4 flex gap-1 border-b border-line">
            {(Object.keys(tabs) as Tab[]).filter((t) => t !== 'edit' || canEdit).map((t) => (
              <button key={t} role="tab" type="button" aria-selected={tab === t} onClick={() => setTab(t)}
                className={`-mb-px cursor-pointer border-b-2 px-3 py-1.5 text-[13px] ${tab === t ? 'border-accent font-medium text-ink' : 'border-transparent text-ink-2 hover:text-ink'}`}>{tabs[t]}</button>
            ))}
          </div>
          {tab === 'details' && (
            <div className="grid gap-5 md:grid-cols-2">
              <section>
                <h3 className={heading}>State</h3>
                <div className="mb-2"><StateBadges allocation={a.allocationState} observation={a.observationState} /></div>
                <dl>
                  <Row label="Hostname">{a.hostname || 'Not set'}</Row>
                  <Row label="FQDN">{a.fqdn || 'Not set'}</Row>
                  <Row label="MAC address">{a.mac ? <Mono>{a.mac}</Mono> : 'Not set'}</Row>
                  <Row label="Owner">{a.ownerEmail ?? 'Not set'}</Row>
                  <Row label="Business service">{a.businessServiceName ?? 'Not set'}</Row>
                  <Row label="Application">{a.applicationName ?? 'Not set'}</Row>
                  <Row label="Device">{a.deviceName ? `${a.deviceName}${a.deviceType ? ` (${a.deviceType.replaceAll('_', ' ')})` : ''}` : 'Not set'}</Row>
                  <Row label="Updated">{formatDateTime(a.updatedAt)}</Row>
                </dl>
                {a.notes && <p className="mt-3 text-ink-2">{a.notes}</p>}
              </section>
              <section>
                <h3 className={heading}>Context</h3>
                <dl>
                  <Row label="Routing domain">{a.routingDomainName}</Row>
                  <Row label="Subnet">{a.subnetName}</Row>
                  <Row label="Site">{a.siteName ?? 'Not set'}</Row>
                  <Row label="Environment">{environments[a.environment] ?? a.environment}</Row>
                </dl>
                <h3 className={`${heading} mt-4`}>Observation</h3>
                <dl>
                  <Row label="Confidence">{a.confidenceScore === null ? 'Unknown' : `${a.confidenceScore}%`}</Row>
                  <Row label="Source">{a.discoverySource ?? 'None yet'}</Row>
                  <Row label="First seen">{a.firstSeen ? formatDateTime(a.firstSeen) : 'Never'}</Row>
                  <Row label="Last seen">{a.lastSeen ? formatDateTime(a.lastSeen) : 'Never'}</Row>
                </dl>
                {a.confidenceExplanation && <p className="mt-2 text-[13px] text-ink-3">{a.confidenceExplanation}</p>}
                <h3 className={`${heading} mt-4`}>DNS records</h3>
                {dns.error && <ErrorState error={dns.error} />}
                {dns.data?.data.length === 0 && <p className="text-[13px] text-ink-3">No DNS records from read-only sources.</p>}
                <ul>{dns.data?.data.map((r) => <li key={r.id} className="border-b border-line py-1 text-[13px] last:border-0"><Mono>{r.recordType}</Mono> {r.name} → <Mono>{r.value}</Mono></li>)}</ul>
              </section>
            </div>
          )}
          {tab === 'edit' && <EditForm a={a} onDone={() => setTab('details')} />}
          {tab === 'history' && (
            <>
              {history.isPending && <SkeletonRows rows={4} />}
              {history.error && <ErrorState error={history.error} onRetry={history.refetch} />}
              {history.data?.length === 0 && <p className="text-ink-3">No changes recorded.</p>}
              <ol>
                {history.data?.map((h, i) => (
                  <li key={i} className="flex justify-between gap-4 border-b border-line py-2 last:border-0">
                    <span className="text-ink">{humanAction(h.action)}<span className="text-ink-3"> by {h.actorEmail ?? 'system'}</span></span>
                    <span className="text-[13px] whitespace-nowrap text-ink-2">{formatDateTime(h.occurredAt)}</span>
                  </li>
                ))}
              </ol>
            </>
          )}
          {tab === 'evidence' && (
            <>
              <p className="mb-2 text-[13px] text-ink-3">Raw observations, kept apart from the record. Discovery never changes allocation.</p>
              {evidence.isPending && <SkeletonRows rows={4} />}
              {evidence.error && <ErrorState error={evidence.error} onRetry={evidence.refetch} />}
              {evidence.data?.length === 0 && <p className="text-ink-3">No evidence yet. Discovery arrives in Phase 4.</p>}
              <ul>
                {evidence.data?.map((e, i) => (
                  <li key={i} className="border-b border-line py-2 last:border-0">
                    <div className="flex justify-between gap-3"><span className="font-medium text-ink">{e.source}</span><span className="text-[13px] text-ink-2">{formatDateTime(e.observedAt)}</span></div>
                    <div className="text-[13px] text-ink-2">{[e.hostname, e.mac].filter(Boolean).join(' · ') || 'No hostname or MAC'}</div>
                  </li>
                ))}
              </ul>
            </>
          )}
        </>
      )}
    </Modal>
  )
}

/** Creates a record. The API places it in the most specific subnet. */
export function NewAddress({ open, onClose, routingDomainId = '', address = '', onCreated }:
  { open: boolean; onClose: () => void; routingDomainId?: string; address?: string; onCreated?: (a: IpAddress) => void }) {
  const qc = useQueryClient()
  const mayAllocate = useCan('ip:allocate')
  const mayReserve = useCan('ip:reserve')
  const canAllocate = mayAllocate || mayReserve
  const domains = useQuery({ queryKey: ['routing-domains', 'all'], enabled: open, queryFn: () => listAll<RoutingDomain>('/routing-domains') })
  const [v, setV] = useState({ routingDomainId, address, hostname: '', mac: '', allocationState: 'available_candidate', notes: '' })
  useEffect(() => { if (open) setV((p) => ({ ...p, routingDomainId, address })) }, [open, routingDomainId, address])
  const set = (k: keyof typeof v) => (e: { target: { value: string } }) => setV({ ...v, [k]: e.target.value })
  const create = useMutation({
    mutationFn: () => post<IpAddress>('/ip-addresses', { ...v, mac: v.mac || undefined, allocationState: canAllocate ? v.allocationState : undefined }),
    onSuccess: (a) => {
      invalidate(qc)
      setV({ routingDomainId, address: '', hostname: '', mac: '', allocationState: 'available_candidate', notes: '' })
      create.reset()
      onClose()
      onCreated?.(a)
    },
  })
  const issue = (field: string) => create.error instanceof ApiError ? create.error.details.find((d) => d.field === field)?.issue : undefined
  const submit = (e: FormEvent) => { e.preventDefault(); create.mutate() }
  return (
    <Modal open={open} onClose={onClose} title="New IP record" size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="new-ip" busy={create.isPending}>Create record</Button></>}>
      <form id="new-ip" noValidate onSubmit={submit} className="grid gap-3 sm:grid-cols-2">
        <Field label="Routing domain" htmlFor="nip-rd" error={issue('routingDomainId')}>
          <Select id="nip-rd" value={v.routingDomainId} onChange={set('routingDomainId')}>
            <option value="">Select</option>
            {domains.data?.map((d) => <option key={d.id} value={d.id}>{d.organisationName} / {d.name}</option>)}
          </Select>
        </Field>
        <Field label="Address" htmlFor="nip-address" error={issue('address')} hint="Placed in the most specific active subnet.">
          <Input id="nip-address" className="font-mono" value={v.address} onChange={set('address')} />
        </Field>
        <Field label="Hostname (optional)" htmlFor="nip-host"><Input id="nip-host" value={v.hostname} onChange={set('hostname')} /></Field>
        <Field label="MAC address (optional)" htmlFor="nip-mac" error={issue('mac')}><Input id="nip-mac" className="font-mono" value={v.mac} onChange={set('mac')} /></Field>
        {canAllocate && (
          <Field label="Allocation state" htmlFor="nip-state">
            <Select id="nip-state" value={v.allocationState} onChange={set('allocationState')}>
              {Object.entries(allocationStates).filter(([k]) => k !== 'retired').map(([k, label]) => <option key={k} value={k}>{label}</option>)}
            </Select>
          </Field>
        )}
        <Field label="Notes (optional)" htmlFor="nip-notes"><Input id="nip-notes" value={v.notes} onChange={set('notes')} /></Field>
        <div className="sm:col-span-2">{!(create.error instanceof ApiError && create.error.details.length > 0) && <FormError error={create.error} />}</div>
      </form>
    </Modal>
  )
}
