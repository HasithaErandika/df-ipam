import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { allocationStates, deviceTypes, StateBadges } from '../../components/devices'
import { RefSelect, RoutingDomainSelect } from '../../components/SearchSelect'
import { Button, ErrorState, Field, FormError, Input, Modal, Mono, Select, SkeletonRows, useConfirmSave } from '../../components/ui'
import { Detail, Tabs } from '../../components/workflow'
import { useChecks } from '../../components/workflow'
import { ApiError, get, patchPlain, post } from '../../lib/api'
import { formatDateTime, humanAction } from '../../lib/format'
import { useCan, useMe } from '../../lib/session'
import type { EvidenceItem, HistoryEntry, IPRecord } from '../../lib/types'
import { choose, ip, issues as check, optionalMAC } from '../../lib/validation'
import { DevicePanel, heading } from './DevicePanel'

function EditRecord({ record, onClose }: { record: IPRecord; onClose: () => void }) {
  const queryClient = useQueryClient()
  const canAllocate = useCan('ip:allocate')
  const canReserve = useCan('ip:reserve')
  const canState = canAllocate || canReserve
  const [form, setForm] = useState({ hostname: record.hostname, fqdn: record.fqdn, mac: record.mac ?? '', notes: record.notes, deviceId: record.deviceId ?? '', allocationState: record.allocationState, reason: '' })
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Address saved.', inlineError: true },
    mutationFn: () => patchPlain(`/ip-addresses/${record.id}`, {
      hostname: form.hostname, fqdn: form.fqdn, mac: form.mac, notes: form.notes, deviceId: form.deviceId,
      ...(form.allocationState !== record.allocationState ? { allocationState: form.allocationState, reason: form.reason } : {}),
    }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['ip'] }); queryClient.invalidateQueries({ queryKey: ['subnets'] }); onClose() },
  })
  const held = record.allocationState === 'allocated' || record.allocationState === 'reserved'
  const checks = useChecks(check({
    mac: optionalMAC(form.mac),
    reason: held && form.allocationState !== record.allocationState && !form.reason.trim() ? 'Give a reason for releasing this address.' : undefined,
  }))
  const errors: Record<string, string> = {}
  if (save.error instanceof ApiError) for (const d of save.error.details) errors[d.field] = d.issue
  Object.assign(errors, checks.shown)
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm((f) => ({ ...f, [k]: e.target.value }))
  return (
    <Modal open onClose={onClose} title={`Edit ${record.address}`} subtitle="Changes are audited."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => confirmSave(record.address, () => save.mutate()))}>Save changes</Button></>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Hostname" htmlFor="ip-host"><Input id="ip-host" value={form.hostname} onChange={set('hostname')} /></Field>
        <Field label="FQDN" htmlFor="ip-fqdn"><Input id="ip-fqdn" value={form.fqdn} onChange={set('fqdn')} /></Field>
        <Field label="MAC" htmlFor="ip-mac" error={errors.mac}><Input id="ip-mac" className="font-mono" value={form.mac} onChange={set('mac')} /></Field>
        <Field label="Device" htmlFor="ip-device">
          <RefSelect<{ id: string; name: string; deviceType: string }> id="ip-device" value={form.deviceId} onChange={(v) => setForm((f) => ({ ...f, deviceId: v }))} path="/devices" emptyLabel="None"
            toOption={(d) => ({ value: d.id, label: d.name, detail: deviceTypes[d.deviceType]?.label ?? d.deviceType })} />
        </Field>
        <div className="col-span-2"><Field label="Notes" htmlFor="ip-notes"><Input id="ip-notes" value={form.notes} onChange={set('notes')} /></Field></div>
        {canState && (
          <>
            <Field label="Allocation state" htmlFor="ip-state" hint="Prefer the request workflow; this is a direct change.">
              <Select id="ip-state" value={form.allocationState} onChange={set('allocationState')}>
                {Object.entries(allocationStates).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
              </Select>
            </Field>
            {form.allocationState !== record.allocationState && (
              <Field label="Reason" htmlFor="ip-reason" error={errors.reason}><Input id="ip-reason" value={form.reason} onChange={set('reason')} /></Field>
            )}
          </>
        )}
      </div>
      {Object.keys(errors).length === 0 && <div className="mt-3"><FormError error={save.error} /></div>}
    </Modal>
  )
}

export function RecordModal({ id, onClose }: { id: string; onClose: () => void }) {
  const me = useMe()
  const seesAll = me.permissions.includes('inventory:view')
  const editsAll = me.permissions.includes('inventory:edit')
  const editsOwn = me.permissions.includes('inventory:edit_assigned')
  const [tab, setTab] = useState('device')
  const [editing, setEditing] = useState(false)
  const record = useQuery({ queryKey: ['ip', id], queryFn: () => get<IPRecord>(`/ip-addresses/${id}`) })
  const history = useQuery({ queryKey: ['ip', id, 'history'], enabled: tab === 'history', queryFn: () => get<HistoryEntry[]>(`/ip-addresses/${id}/history`) })
  const evidence = useQuery({ queryKey: ['ip', id, 'evidence'], enabled: tab === 'evidence', queryFn: () => get<EvidenceItem[]>(`/ip-addresses/${id}/evidence`) })
  const r = record.data
  const canEdit = editsAll || (editsOwn && r?.ownerId === me.id)
  return (
    <>
      <Modal open onClose={onClose} size="xl" title={r ? r.address : 'Address'}
        subtitle={r && <span className="flex flex-wrap items-center gap-2"><StateBadges allocation={r.allocationState} observation={r.observationState} /> {r.subnetName} <Mono>{r.subnetCidr}</Mono> · {r.routingDomainName}</span>}
        footer={r && canEdit ? <Button variant="primary" onClick={() => setEditing(true)}>Edit record</Button> : undefined}>
        {record.isPending && <SkeletonRows rows={6} />}
        {record.error && <ErrorState error={record.error} onRetry={record.refetch} />}
        {r && (
          <div className={seesAll ? 'grid gap-5 md:grid-cols-[1fr_1.2fr]' : ''}>
            <section>
              <h3 className={heading}>Record</h3>
              <dl>
                <Detail label="Hostname">{r.hostname || '—'}</Detail>
                <Detail label="FQDN">{r.fqdn || '—'}</Detail>
                {seesAll && <Detail label="MAC">{r.mac ? <Mono>{r.mac}</Mono> : '—'}</Detail>}
                <Detail label="Owner">{r.ownerEmail ?? '—'}</Detail>
                <Detail label="Application">{r.applicationName ?? '—'}</Detail>
                <Detail label="Business service">{r.businessServiceName ?? '—'}</Detail>
                <Detail label="Site">{r.siteName ?? '—'}</Detail>
                {seesAll && <Detail label="Confidence">{r.confidenceScore === null ? '—' : `${r.confidenceScore}%`}</Detail>}
                {r.confidenceExplanation && <Detail label="Why">{r.confidenceExplanation}</Detail>}
                <Detail label="First seen">{r.firstSeen ? formatDateTime(r.firstSeen) : '—'}</Detail>
                <Detail label="Last seen">{r.lastSeen ? formatDateTime(r.lastSeen) : '—'}</Detail>
                <Detail label="Updated">{formatDateTime(r.updatedAt)}</Detail>
              </dl>
              {r.notes && <p className="mt-2 text-[13px] text-ink-2">{r.notes}</p>}
            </section>
            {seesAll && <section>
              <Tabs tabs={[{ key: 'device', label: 'Device' }, { key: 'evidence', label: 'Evidence' }, { key: 'history', label: 'History' }]} active={tab} onChange={setTab} />
              {tab === 'device' && <DevicePanel id={id} />}
              {tab === 'evidence' && (evidence.isPending ? <SkeletonRows rows={3} /> : evidence.error ? <ErrorState error={evidence.error} /> :
                evidence.data!.length === 0 ? <p className="text-[13px] text-ink-3">No raw evidence yet. Evidence stays separate from the record above.</p> : (
                  <ul>{evidence.data!.map((e, i) => <li key={i} className="flex justify-between border-b border-line py-1 text-[13px] last:border-0"><span>{e.source}{e.hostname && ` · ${e.hostname}`}{e.mac && <> · <Mono>{e.mac}</Mono></>}</span><span className="text-ink-3">{formatDateTime(e.observedAt)}</span></li>)}</ul>
                ))}
              {tab === 'history' && (history.isPending ? <SkeletonRows rows={3} /> : history.error ? <ErrorState error={history.error} /> :
                history.data!.length === 0 ? <p className="text-[13px] text-ink-3">No changes recorded.</p> : (
                  <ul>{history.data!.map((h, i) => <li key={i} className="flex justify-between border-b border-line py-1 text-[13px] last:border-0"><span>{humanAction(h.action)}</span><span className="text-ink-3">{h.actorEmail ?? 'System'} · {formatDateTime(h.occurredAt)}</span></li>)}</ul>
                ))}
            </section>}
          </div>
        )}
      </Modal>
      {editing && r && <EditRecord record={r} onClose={() => setEditing(false)} />}
    </>
  )
}

export function NewRecord({ onClose, onCreated }: { onClose: () => void; onCreated: (id: string) => void }) {
  const queryClient = useQueryClient()
  const [routingDomainId, setRd] = useState('')
  const [address, setAddress] = useState('')
  const [hostname, setHostname] = useState('')
  const create = useMutation({ meta: { success: 'Address record created.', inlineError: true },
    mutationFn: () => post<IPRecord>('/ip-addresses', { routingDomainId, address, hostname }),
    onSuccess: (r) => { queryClient.invalidateQueries({ queryKey: ['ip'] }); onClose(); onCreated(r.id) },
  })
  const checks = useChecks(check({ routingDomainId: choose(routingDomainId, 'a routing domain'), address: ip(address) }))
  const errors: Record<string, string> = {}
  if (create.error instanceof ApiError) for (const d of create.error.details) errors[d.field] = d.issue
  Object.assign(errors, checks.shown)
  return (
    <Modal open onClose={onClose} title="New IP record" subtitle="It joins the most specific subnet holding the address. Records are only kept for addresses of interest."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={create.isPending} onClick={() => checks.submit(() => create.mutate())}>Create record</Button></>}>
      <div className="space-y-3.5">
        <Field label="Routing domain" htmlFor="nr-rd" error={errors.routingDomainId}>
          <RoutingDomainSelect id="nr-rd" value={routingDomainId} onChange={setRd} />
        </Field>
        <Field label="Address" htmlFor="nr-addr" error={errors.address}><Input id="nr-addr" className="font-mono" value={address} onChange={(e) => setAddress(e.target.value)} /></Field>
        <Field label="Hostname (optional)" htmlFor="nr-host"><Input id="nr-host" value={hostname} onChange={(e) => setHostname(e.target.value)} /></Field>
        {Object.keys(errors).length === 0 && <FormError error={create.error} />}
      </div>
    </Modal>
  )
}
