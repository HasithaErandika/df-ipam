import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router'
import { CatalogManager, entities, type Row } from '../components/catalog'
import { Badge, Button, Card, ConfirmDialog, Modal, EmptyState, ErrorState, Field, FormError, Input, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { Tabs } from '../components/workflow'
import { del, get, list, patchPlain, post } from '../lib/api'
import { domainKinds, formatCount } from '../lib/format'
import { useClampPage, usePaging, type Paging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { Organisation, RoutingDomain } from '../lib/types'
import { RefSelect } from '../components/SearchSelect'
import { useChecks } from '../components/workflow'
import { choose, issues as check, required } from '../lib/validation'

const useOrganisations = (p: Paging) => useQuery({
  queryKey: ['organisations', p.page, p.pageSize], placeholderData: keepPreviousData,
  queryFn: () => list<Organisation>(`/organisations?page=${p.page}&pageSize=${p.pageSize}`),
})
const useDomains = (p: Paging) => useQuery({
  queryKey: ['routing-domains', p.page, p.pageSize], placeholderData: keepPreviousData,
  queryFn: () => list<RoutingDomain>(`/routing-domains?page=${p.page}&pageSize=${p.pageSize}`),
})

function NewOrganisation({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [code, setCode] = useState('')
  const create = useMutation({
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

function NewDomain({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [organisationId, setOrganisationId] = useState('')
  const [name, setName] = useState('')
  const [kind, setKind] = useState('vrf')
  const [routeDistinguisher, setRd] = useState('')
  const [isIsolated, setIsolated] = useState(false)
  const create = useMutation({
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

function BlockSpaceModal({ block, onClose }: { block: Row; onClose: () => void }) {
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

function EditOrganisation({ org, onClose }: { org: Organisation; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(org.name)
  const [code, setCode] = useState(org.code)
  const save = useMutation({
    mutationFn: () => patchPlain(`/organisations/${org.id}`, { name, code }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['organisations'] }); queryClient.invalidateQueries({ queryKey: ['routing-domains'] }); onClose() },
  })
  return (
    <Modal open onClose={onClose} title="Edit organisation"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => save.mutate()}>Save changes</Button></>}>
      <div className="space-y-4">
        <Field label="Name" htmlFor="eo-name"><Input id="eo-name" value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Code" htmlFor="eo-code"><Input id="eo-code" value={code} onChange={(e) => setCode(e.target.value)} /></Field>
        <FormError error={save.error} />
      </div>
    </Modal>
  )
}

function EditDomain({ domain, onClose }: { domain: RoutingDomain; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(domain.name)
  const [rd, setRd] = useState(domain.routeDistinguisher ?? '')
  const [isolated, setIsolated] = useState(domain.isIsolated)
  const save = useMutation({
    mutationFn: () => patchPlain(`/routing-domains/${domain.id}`, { name, routeDistinguisher: rd, isIsolated: isolated }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['routing-domains'] }); onClose() },
  })
  return (
    <Modal open onClose={onClose} title="Edit routing domain" subtitle={`${domain.organisationName} · ${domainKinds[domain.kind]}`}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => save.mutate()}>Save changes</Button></>}>
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

function Hierarchy() {
  const queryClient = useQueryClient()
  const canCreate = useCan('inventory:create')
  const canEdit = useCan('inventory:edit')
  const canDelete = useCan('inventory:delete')
  const orgPaging = usePaging('org', 25)
  const domainPaging = usePaging('domain', 25)
  const organisations = useOrganisations(orgPaging)
  const domains = useDomains(domainPaging)
  useClampPage(orgPaging, organisations.data?.meta)
  useClampPage(domainPaging, domains.data?.meta)
  const [adding, setAdding] = useState<'org' | 'domain' | null>(null)
  const [editOrg, setEditOrg] = useState<Organisation | null>(null)
  const [editDomain, setEditDomain] = useState<RoutingDomain | null>(null)
  const [removing, setRemoving] = useState<{ path: string; label: string } | null>(null)
  const remove = useMutation({
    mutationFn: (path: string) => del(path),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['organisations'] }); queryClient.invalidateQueries({ queryKey: ['routing-domains'] }); setRemoving(null) },
  })
  const hasOrgs = (organisations.data?.data.length ?? 0) > 0
  const icon = 'h-7 px-1.5'
  return (
    <>
      <div className="grid gap-4 xl:grid-cols-3">
        <Card title="Organisations" action={canCreate && <Button variant="ghost" className="h-7 px-2" onClick={() => setAdding('org')}><Plus size={15} aria-hidden /> Add</Button>}>
          {organisations.isPending && <SkeletonRows rows={3} />}
          {organisations.error && <ErrorState error={organisations.error} onRetry={organisations.refetch} />}
          {organisations.data?.data.length === 0 && <EmptyState title="No organisations yet" body="Start here: subnets belong to an organisation's routing domain." />}
          {hasOrgs && (
            <ul>
              {organisations.data?.data.map((o) => (
                <li key={o.id} className="flex items-center justify-between gap-2 border-b border-line px-4 py-2 last:border-0">
                  <span className="font-medium text-ink">{o.name}</span>
                  <span className="flex items-center gap-1">
                    <Badge>{o.code}</Badge>
                    {canEdit && <Button variant="ghost" className={icon} aria-label={`Edit ${o.name}`} onClick={() => setEditOrg(o)}><Pencil size={14} /></Button>}
                    {canDelete && <Button variant="ghost" className={icon} aria-label={`Delete ${o.name}`} onClick={() => { remove.reset(); setRemoving({ path: `/organisations/${o.id}`, label: o.name }) }}><Trash2 size={14} /></Button>}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {organisations.data && hasOrgs && <Pagination meta={organisations.data.meta} onPage={orgPaging.setPage} />}
        </Card>
        <Card title="Routing domains" className="xl:col-span-2" action={canCreate && hasOrgs && <Button variant="ghost" className="h-7 px-2" onClick={() => setAdding('domain')}><Plus size={15} aria-hidden /> Add</Button>}>
          {domains.isPending && <SkeletonRows rows={3} />}
          {domains.error && <ErrorState error={domains.error} onRetry={domains.refetch} />}
          {domains.data?.data.length === 0 && <EmptyState title="No routing domains yet" body="Each organisation gets a Default routing domain." />}
          {domains.data && domains.data.data.length > 0 && (
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead><tr><Th>Name</Th><Th>Organisation</Th><Th>Type</Th><Th>RD</Th><Th className="text-right">Subnets</Th>{(canEdit || canDelete) && <Th />}</tr></thead>
                <tbody>
                  {domains.data.data.map((d) => (
                    <tr key={d.id}>
                      <Td className="font-medium text-ink">{d.name} {d.isIsolated && <Badge tone="warn">Isolated</Badge>}</Td>
                      <Td className="text-ink-2">{d.organisationName}</Td>
                      <Td className="text-ink-2">{domainKinds[d.kind]}</Td>
                      <Td className="text-ink-2">{d.routeDistinguisher ? <Mono>{d.routeDistinguisher}</Mono> : '—'}</Td>
                      <Td className="text-right text-ink-2 tabular-nums">{formatCount(d.subnetCount)}</Td>
                      {(canEdit || canDelete) && (
                        <Td className="text-right whitespace-nowrap">
                          {canEdit && <Button variant="ghost" className={icon} aria-label={`Edit ${d.name}`} onClick={() => setEditDomain(d)}><Pencil size={14} /></Button>}
                          {canDelete && <Button variant="ghost" className={icon} aria-label={`Delete ${d.name}`} onClick={() => { remove.reset(); setRemoving({ path: `/routing-domains/${d.id}`, label: d.name }) }}><Trash2 size={14} /></Button>}
                        </Td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {domains.data && domains.data.data.length > 0 && <Pagination meta={domains.data.meta} onPage={domainPaging.setPage} onPageSize={domainPaging.setPageSize} />}
        </Card>
      </div>
      <NewOrganisation open={adding === 'org'} onClose={() => setAdding(null)} />
      <NewDomain open={adding === 'domain'} onClose={() => setAdding(null)} />
      {editOrg && <EditOrganisation org={editOrg} onClose={() => setEditOrg(null)} />}
      {editDomain && <EditDomain domain={editDomain} onClose={() => setEditDomain(null)} />}
      <ConfirmDialog open={removing !== null} title={`Delete ${removing?.label}?`}
        body="Only records nothing else uses can be deleted. Archive subnets or remove related records first."
        confirmLabel="Delete" busy={remove.isPending} error={remove.error} onConfirm={() => removing && remove.mutate(removing.path)} onCancel={() => setRemoving(null)} />
    </>
  )
}

export function Network() {
  const [params, setParams] = useSearchParams()
  const [space, setSpace] = useState<Row | null>(null)
  const defs = entities.map((e) => e.key === 'blocks'
    ? { ...e, rowAction: (r: Row) => <Button variant="ghost" className="h-7" onClick={() => setSpace(r)}>Free space</Button> }
    : e)
  const tabs = [{ key: 'hierarchy', label: 'Organisations and routing' }, ...defs.map((e) => ({ key: e.key, label: e.label }))]
  const tab = tabs.some((t) => t.key === params.get('tab')) ? params.get('tab')! : 'hierarchy'
  const def = defs.find((e) => e.key === tab)
  return (
    <>
      <PageHeader title="Network Management" description="The hierarchy that gives each address its context: organisations, routing domains, sites, zones, VDOMs, VLANs, blocks and owners." />
      <Tabs tabs={tabs} active={tab} onChange={(key) => setParams({ tab: key })} />
      {tab === 'hierarchy' ? <Hierarchy /> : def && <CatalogManager key={def.key} def={def} />}
      {space && <BlockSpaceModal block={space} onClose={() => setSpace(null)} />}
    </>
  )
}
