import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { del, list, listAll, patch, post } from '../lib/api'
import { domainKinds, formatCount } from '../lib/format'
import { useClampPage, usePaging, type Paging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { Organisation, RoutingDomain } from '../lib/types'
import { CatalogTab } from './network/catalog'
import { catalogs } from './network/definitions'

const useOrganisations = (p: Paging) => useQuery({
  queryKey: ['organisations', p.page, p.pageSize], placeholderData: keepPreviousData,
  queryFn: () => list<Organisation>(`/organisations?page=${p.page}&pageSize=${p.pageSize}`),
})
const useDomains = (p: Paging) => useQuery({
  queryKey: ['routing-domains', p.page, p.pageSize], placeholderData: keepPreviousData,
  queryFn: () => list<RoutingDomain>(`/routing-domains?page=${p.page}&pageSize=${p.pageSize}`),
})

function invalidateHierarchy(qc: ReturnType<typeof useQueryClient>) {
  qc.invalidateQueries({ queryKey: ['organisations'] })
  qc.invalidateQueries({ queryKey: ['routing-domains'] })
  qc.invalidateQueries({ queryKey: ['options'] })
}

function OrganisationForm({ org, onClose }: { org: Organisation | null; onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState(org?.name ?? '')
  const [code, setCode] = useState(org?.code ?? '')
  const save = useMutation({
    mutationFn: () => org ? patch(`/organisations/${org.id}`, { name, code }) : post('/organisations', { name, code }),
    onSuccess: () => { invalidateHierarchy(qc); onClose() },
  })
  const submit = (e: FormEvent) => { e.preventDefault(); save.mutate() }
  return (
    <Modal open onClose={onClose} title={org ? 'Edit organisation' : 'New organisation'}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="org-form" busy={save.isPending}>{org ? 'Save changes' : 'Create organisation'}</Button></>}>
      <form id="org-form" onSubmit={submit} className="space-y-4">
        <Field label="Name" htmlFor="org-name"><Input id="org-name" required autoFocus value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Code" htmlFor="org-code" hint="A short identifier, such as DF."><Input id="org-code" required value={code} onChange={(e) => setCode(e.target.value)} /></Field>
        {!org && <p className="text-[13px] text-ink-3">A Default routing domain is created with it.</p>}
        <FormError error={save.error} />
      </form>
    </Modal>
  )
}

function DomainForm({ domain, onClose }: { domain: RoutingDomain | null; onClose: () => void }) {
  const qc = useQueryClient()
  const organisations = useQuery({ queryKey: ['organisations', 'all'], queryFn: () => listAll<Organisation>('/organisations') })
  const [organisationId, setOrganisationId] = useState(domain?.organisationId ?? '')
  const [name, setName] = useState(domain?.name ?? '')
  const [kind, setKind] = useState(domain?.kind ?? 'vrf')
  const [routeDistinguisher, setRd] = useState(domain?.routeDistinguisher ?? '')
  const [isIsolated, setIsolated] = useState(domain?.isIsolated ?? false)
  const save = useMutation({
    mutationFn: () => domain
      ? patch(`/routing-domains/${domain.id}`, { name, kind, routeDistinguisher, isIsolated })
      : post('/routing-domains', { organisationId, name, kind, routeDistinguisher: routeDistinguisher || null, isIsolated }),
    onSuccess: () => { invalidateHierarchy(qc); onClose() },
  })
  const submit = (e: FormEvent) => { e.preventDefault(); save.mutate() }
  return (
    <Modal open onClose={onClose} title={domain ? 'Edit routing domain' : 'New routing domain'}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="rd-form" busy={save.isPending}>{domain ? 'Save changes' : 'Create routing domain'}</Button></>}>
      <form id="rd-form" onSubmit={submit} className="space-y-4">
        <Field label="Organisation" htmlFor="rd-org" hint={domain ? "A routing domain can't move to another organisation." : undefined}>
          <Select id="rd-org" required disabled={domain !== null} value={organisationId} onChange={(e) => setOrganisationId(e.target.value)}>
            <option value="">Select</option>
            {organisations.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </Select>
        </Field>
        <Field label="Name" htmlFor="rd-name"><Input id="rd-name" required value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Type" htmlFor="rd-kind" hint="AWS VPC and Azure VNet subnets get the provider's five reserved addresses automatically.">
          <Select id="rd-kind" value={kind} onChange={(e) => setKind(e.target.value)}>
            {Object.entries(domainKinds).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </Field>
        <Field label="Route distinguisher (optional)" htmlFor="rd-rd"><Input id="rd-rd" className="font-mono" value={routeDistinguisher} onChange={(e) => setRd(e.target.value)} /></Field>
        <label className="flex items-center gap-2 text-ink">
          <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={isIsolated} onChange={(e) => setIsolated(e.target.checked)} />
          Isolated routing domain
        </label>
        <FormError error={save.error} />
      </form>
    </Modal>
  )
}

type Deleting = { kind: 'org'; row: Organisation } | { kind: 'domain'; row: RoutingDomain }

function Hierarchy() {
  const qc = useQueryClient()
  const canCreate = useCan('inventory:create')
  const canEdit = useCan('inventory:edit')
  const canDelete = useCan('inventory:delete')
  const orgPaging = usePaging('org', 25)
  const domainPaging = usePaging('domain', 25)
  const organisations = useOrganisations(orgPaging)
  const domains = useDomains(domainPaging)
  useClampPage(orgPaging, organisations.data?.meta)
  useClampPage(domainPaging, domains.data?.meta)
  const [orgForm, setOrgForm] = useState<Organisation | 'new' | null>(null)
  const [domainForm, setDomainForm] = useState<RoutingDomain | 'new' | null>(null)
  const [deleting, setDeleting] = useState<Deleting | null>(null)
  const remove = useMutation({
    mutationFn: (d: Deleting) => del(d.kind === 'org' ? `/organisations/${d.row.id}` : `/routing-domains/${d.row.id}`),
    onSuccess: () => { invalidateHierarchy(qc); setDeleting(null) },
  })
  const hasOrgs = (organisations.data?.data.length ?? 0) > 0
  const iconButton = 'h-7 px-1.5'

  return (
    <div className="grid gap-4 xl:grid-cols-3">
      <Card title="Organisations" action={canCreate && <Button variant="ghost" className="h-7 px-2" onClick={() => setOrgForm('new')}><Plus size={15} aria-hidden /> Add</Button>}>
        {organisations.isPending && <SkeletonRows rows={3} />}
        {organisations.error && <ErrorState error={organisations.error} onRetry={organisations.refetch} />}
        {organisations.data?.data.length === 0 && <EmptyState title="No organisations yet" body="Start here: subnets belong to an organisation's routing domain." />}
        {hasOrgs && (
          <ul>
            {organisations.data?.data.map((o) => (
              <li key={o.id} className="flex items-center justify-between gap-2 border-b border-line px-4 py-2 last:border-0">
                <span className="min-w-0 truncate font-medium text-ink">{o.name}</span>
                <span className="flex items-center gap-1">
                  <Badge>{o.code}</Badge>
                  {canEdit && <Button variant="ghost" className={iconButton} aria-label={`Edit ${o.name}`} onClick={() => setOrgForm(o)}><Pencil size={14} /></Button>}
                  {canDelete && <Button variant="ghost" className={`${iconButton} hover:text-danger`} aria-label={`Delete ${o.name}`} onClick={() => { remove.reset(); setDeleting({ kind: 'org', row: o }) }}><Trash2 size={14} /></Button>}
                </span>
              </li>
            ))}
          </ul>
        )}
        {organisations.data && hasOrgs && <Pagination meta={organisations.data.meta} onPage={orgPaging.setPage} />}
      </Card>
      <Card title="Routing domains" className="xl:col-span-2" action={canCreate && hasOrgs && <Button variant="ghost" className="h-7 px-2" onClick={() => setDomainForm('new')}><Plus size={15} aria-hidden /> Add</Button>}>
        {domains.isPending && <SkeletonRows rows={3} />}
        {domains.error && <ErrorState error={domains.error} onRetry={domains.refetch} />}
        {domains.data?.data.length === 0 && <EmptyState title="No routing domains yet" body="Each organisation gets a Default routing domain." />}
        {domains.data && domains.data.data.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr><Th>Name</Th><Th>Organisation</Th><Th>Type</Th><Th>RD</Th><Th className="text-right">Subnets</Th><Th className="w-px"><span className="sr-only">Actions</span></Th></tr></thead>
              <tbody>
                {domains.data.data.map((d) => (
                  <tr key={d.id}>
                    <Td className="font-medium text-ink">{d.name} {d.isIsolated && <Badge tone="warn">Isolated</Badge>}</Td>
                    <Td className="text-ink-2">{d.organisationName}</Td>
                    <Td className="text-ink-2">{domainKinds[d.kind]}</Td>
                    <Td className="text-ink-2">{d.routeDistinguisher ? <Mono>{d.routeDistinguisher}</Mono> : '—'}</Td>
                    <Td className="text-right tabular-nums"><Link className="text-ink-2 hover:text-accent" to={`/subnets?routingDomainId=${d.id}`}>{formatCount(d.subnetCount)}</Link></Td>
                    <Td className="whitespace-nowrap text-right">
                      {canEdit && <Button variant="ghost" className={iconButton} aria-label={`Edit ${d.name}`} onClick={() => setDomainForm(d)}><Pencil size={14} /></Button>}
                      {canDelete && <Button variant="ghost" className={`${iconButton} hover:text-danger`} aria-label={`Delete ${d.name}`} onClick={() => { remove.reset(); setDeleting({ kind: 'domain', row: d }) }}><Trash2 size={14} /></Button>}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {domains.data && domains.data.data.length > 0 && <Pagination meta={domains.data.meta} onPage={domainPaging.setPage} onPageSize={domainPaging.setPageSize} />}
      </Card>
      {orgForm !== null && <OrganisationForm org={orgForm === 'new' ? null : orgForm} onClose={() => setOrgForm(null)} />}
      {domainForm !== null && <DomainForm domain={domainForm === 'new' ? null : domainForm} onClose={() => setDomainForm(null)} />}
      <ConfirmDialog open={deleting !== null}
        title={deleting?.kind === 'org' ? 'Delete this organisation?' : 'Delete this routing domain?'}
        body={deleting?.kind === 'org'
          ? `${deleting.row.name} and its unused routing domains are removed. Anything still linked to it blocks the delete.`
          : `${deleting?.row.name ?? ''} is removed. A routing domain with subnets, archived ones included, can't be deleted.`}
        confirmLabel="Delete" busy={remove.isPending} error={remove.error}
        onConfirm={() => deleting && remove.mutate(deleting)} onCancel={() => setDeleting(null)} />
    </div>
  )
}

const tabs = [{ tab: 'hierarchy', label: 'Organisations and routing domains' }, ...catalogs.map((c) => ({ tab: c.tab, label: c.label }))]

export function Network() {
  const [params, setParams] = useSearchParams()
  const current = params.get('tab') ?? 'hierarchy'
  const def = catalogs.find((c) => c.tab === current)
  return (
    <>
      <PageHeader title="Network Management" description="The hierarchy and reference data that subnets and addresses map to." />
      <div role="tablist" aria-label="Network records" className="mb-4 flex flex-wrap gap-1 border-b border-line">
        {tabs.map((t) => (
          <button key={t.tab} role="tab" type="button" aria-selected={current === t.tab}
            onClick={() => setParams(t.tab === 'hierarchy' ? {} : { tab: t.tab })}
            className={`-mb-px cursor-pointer border-b-2 px-3 py-1.5 text-[13px] whitespace-nowrap ${current === t.tab ? 'border-accent font-medium text-ink' : 'border-transparent text-ink-2 hover:text-ink'}`}>
            {t.label}
          </button>
        ))}
      </div>
      {def ? <CatalogTab key={def.tab} def={def} /> : <Hierarchy />}
    </>
  )
}
