import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useState } from 'react'
import { type Row } from '../../components/catalog/types'
import { RefSelect } from '../../components/SearchSelect'
import { Button, ErrorState, Field, FormError, Input, Modal, Mono, Select, SkeletonRows, Td, Th, useConfirmSave } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import { get, patchPlain, post } from '../../lib/api'
import { domainKinds } from '../../lib/format'
import type { Organisation, RoutingDomain } from '../../lib/types'
import { choose, issues as check, required } from '../../lib/validation'

export function NewOrganisation({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [code, setCode] = useState('')
  const create = useMutation({ meta: { success: 'Organisation created.', inlineError: true },
    mutationFn: () => post('/organisations', { name, code }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['organisations'] })
      queryClient.invalidateQueries({ queryKey: ['routing-domains'] })
      setName(''); setCode('')
      onClose()
    },
  })
  const checks = useChecks(check({ name: required(name, 'Name'), code: required(code, 'Code') }), open)
  const submit = (e: FormEvent) => { e.preventDefault(); checks.submit(() => create.mutate()) }
  return (
    <Modal open={open} onClose={onClose} title="New organisation"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="new-org" busy={create.isPending}>Create organisation</Button></>}>
      <form id="new-org" noValidate onSubmit={submit} className="space-y-4">
        <Field label="Name" htmlFor="org-name" error={checks.shown.name}><Input id="org-name" autoFocus value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Code" htmlFor="org-code" error={checks.shown.code} hint="A short identifier, such as DF."><Input id="org-code" value={code} onChange={(e) => setCode(e.target.value)} /></Field>
        <p className="text-[13px] text-ink-3">A Default routing domain is created with it.</p>
        <FormError error={create.error} />
      </form>
    </Modal>
  )
}

export function NewDomain({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [organisationId, setOrganisationId] = useState('')
  const [name, setName] = useState('')
  const [kind, setKind] = useState('vrf')
  const [routeDistinguisher, setRd] = useState('')
  const [isIsolated, setIsolated] = useState(false)
  const create = useMutation({ meta: { success: 'Routing domain created.', inlineError: true },
    mutationFn: () => post('/routing-domains', { organisationId, name, kind, routeDistinguisher: routeDistinguisher || null, isIsolated }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['routing-domains'] })
      setName(''); setRd(''); setIsolated(false)
      onClose()
    },
  })
  const checks = useChecks(check({
    organisationId: choose(organisationId, 'an organisation'), name: required(name, 'Name'),
    routeDistinguisher: routeDistinguisher.trim() && !/^[\w.]+:\d+$/.test(routeDistinguisher.trim()) ? 'Use a form such as 65000:100.' : undefined,
  }), open)
  const submit = (e: FormEvent) => { e.preventDefault(); checks.submit(() => create.mutate()) }
  return (
    <Modal open={open} onClose={onClose} title="New routing domain"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="new-rd" busy={create.isPending}>Create routing domain</Button></>}>
      <form id="new-rd" noValidate onSubmit={submit} className="space-y-4">
        <Field label="Organisation" htmlFor="rd-org" error={checks.shown.organisationId}>
          <RefSelect<Organisation> id="rd-org" value={organisationId} onChange={setOrganisationId} path="/organisations" toOption={(o) => ({ value: o.id, label: o.name })} />
        </Field>
        <Field label="Name" htmlFor="rd-name" error={checks.shown.name}><Input id="rd-name" value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Type" htmlFor="rd-kind">
          <Select id="rd-kind" value={kind} onChange={(e) => setKind(e.target.value)}>
            {Object.entries(domainKinds).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </Field>
        <Field label="Route distinguisher (optional)" htmlFor="rd-rd" error={checks.shown.routeDistinguisher}><Input id="rd-rd" className="font-mono" value={routeDistinguisher} onChange={(e) => setRd(e.target.value)} /></Field>
        <label className="flex items-center gap-2 text-ink">
          <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={isIsolated} onChange={(e) => setIsolated(e.target.checked)} />
          Isolated routing domain
        </label>
        <FormError error={create.error} />
      </form>
    </Modal>
  )
}

interface BlockSpace { cidr: string; totalAddresses: string; freeAddresses: string; utilisationPct: string; largestFreeCidr?: string; freeRanges: { first: string; last: string; size: string; cidrs: string[] }[] }

export function BlockSpaceModal({ block, onClose }: { block: Row; onClose: () => void }) {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['block-space', block.id], queryFn: () => get<BlockSpace>(`/network-blocks/${block.id}/free-space`) })
  return (
    <Modal open onClose={onClose} title={`Free space in ${block.name}`} subtitle={<Mono>{block.cidr}</Mono>} size="lg">
      {isPending && <SkeletonRows rows={4} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && (
        <>
          <p className="text-[13px] text-ink-2">{data.utilisationPct}% used by subnets. {data.largestFreeCidr && <>Largest free prefix: <Mono>{data.largestFreeCidr}</Mono>.</>}</p>
          {data.freeRanges.length === 0 ? <p className="mt-3 text-[13px] text-ink-3">No free space left.</p> : (
            <table className="mt-3 w-full border-collapse">
              <thead><tr><Th>Range</Th><Th>As CIDRs</Th></tr></thead>
              <tbody>{data.freeRanges.map((r) => (
                <tr key={r.first}><Td><Mono>{r.first} – {r.last}</Mono></Td><Td className="text-ink-2"><Mono>{r.cidrs.join(', ')}</Mono></Td></tr>
              ))}</tbody>
            </table>
          )}
        </>
      )}
    </Modal>
  )
}

export function EditOrganisation({ org, onClose }: { org: Organisation; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(org.name)
  const [code, setCode] = useState(org.code)
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Organisation saved.', inlineError: true },
    mutationFn: () => patchPlain(`/organisations/${org.id}`, { name, code }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['organisations'] }); queryClient.invalidateQueries({ queryKey: ['routing-domains'] }); onClose() },
  })
  return (
    <Modal open onClose={onClose} title="Edit organisation"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => confirmSave(org.name, () => save.mutate())}>Save changes</Button></>}>
      <div className="space-y-4">
        <Field label="Name" htmlFor="eo-name"><Input id="eo-name" value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Code" htmlFor="eo-code"><Input id="eo-code" value={code} onChange={(e) => setCode(e.target.value)} /></Field>
        <FormError error={save.error} />
      </div>
    </Modal>
  )
}

export function EditDomain({ domain, onClose }: { domain: RoutingDomain; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(domain.name)
  const [rd, setRd] = useState(domain.routeDistinguisher ?? '')
  const [isolated, setIsolated] = useState(domain.isIsolated)
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Routing domain saved.', inlineError: true },
    mutationFn: () => patchPlain(`/routing-domains/${domain.id}`, { name, routeDistinguisher: rd, isIsolated: isolated }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['routing-domains'] }); onClose() },
  })
  return (
    <Modal open onClose={onClose} title="Edit routing domain" subtitle={`${domain.organisationName} · ${domainKinds[domain.kind]}`}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => confirmSave(domain.name, () => save.mutate())}>Save changes</Button></>}>
      <div className="space-y-4">
        <Field label="Name" htmlFor="ed-name"><Input id="ed-name" value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Route distinguisher (optional)" htmlFor="ed-rd"><Input id="ed-rd" className="font-mono" value={rd} onChange={(e) => setRd(e.target.value)} /></Field>
        <label className="flex items-center gap-2 text-ink"><input type="checkbox" checked={isolated} onChange={(e) => setIsolated(e.target.checked)} /> Isolated routing domain</label>
        <p className="text-[12.5px] text-ink-3">The organisation and type stay fixed, because subnets and provider-reserved ranges depend on them.</p>
        <FormError error={save.error} />
      </div>
    </Modal>
  )
}
