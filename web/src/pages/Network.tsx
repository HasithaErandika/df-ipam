import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { Badge, Button, Card, Modal, EmptyState, ErrorState, Field, FormError, Input, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { list, listAll, post } from '../lib/api'
import { domainKinds, formatCount } from '../lib/format'
import { useClampPage, usePaging, type Paging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { Organisation, RoutingDomain } from '../lib/types'

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
  const submit = (e: FormEvent) => { e.preventDefault(); create.mutate() }
  return (
    <Modal open={open} onClose={onClose} title="New organisation"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="new-org" busy={create.isPending}>Create organisation</Button></>}>
      <form id="new-org" onSubmit={submit} className="space-y-4">
        <Field label="Name" htmlFor="org-name"><Input id="org-name" required autoFocus value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Code" htmlFor="org-code" hint="A short identifier, such as DF."><Input id="org-code" required value={code} onChange={(e) => setCode(e.target.value)} /></Field>
        <p className="text-[13px] text-ink-3">A Default routing domain is created with it.</p>
        <FormError error={create.error} />
      </form>
    </Modal>
  )
}

function NewDomain({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const organisations = useQuery({ queryKey: ['organisations', 'all'], queryFn: () => listAll<Organisation>('/organisations') })
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
  const submit = (e: FormEvent) => { e.preventDefault(); create.mutate() }
  return (
    <Modal open={open} onClose={onClose} title="New routing domain"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="new-rd" busy={create.isPending}>Create routing domain</Button></>}>
      <form id="new-rd" onSubmit={submit} className="space-y-4">
        <Field label="Organisation" htmlFor="rd-org">
          <Select id="rd-org" required value={organisationId} onChange={(e) => setOrganisationId(e.target.value)}>
            <option value="">Select</option>
            {organisations.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </Select>
        </Field>
        <Field label="Name" htmlFor="rd-name"><Input id="rd-name" required value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Type" htmlFor="rd-kind">
          <Select id="rd-kind" value={kind} onChange={(e) => setKind(e.target.value)}>
            {Object.entries(domainKinds).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </Field>
        <Field label="Route distinguisher (optional)" htmlFor="rd-rd"><Input id="rd-rd" className="font-mono" value={routeDistinguisher} onChange={(e) => setRd(e.target.value)} /></Field>
        <label className="flex items-center gap-2 text-ink">
          <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={isIsolated} onChange={(e) => setIsolated(e.target.checked)} />
          Isolated routing domain
        </label>
        <FormError error={create.error} />
      </form>
    </Modal>
  )
}

export function Network() {
  const canCreate = useCan('inventory:create')
  const orgPaging = usePaging('org', 25)
  const domainPaging = usePaging('domain', 25)
  const organisations = useOrganisations(orgPaging)
  const domains = useDomains(domainPaging)
  useClampPage(orgPaging, organisations.data?.meta)
  useClampPage(domainPaging, domains.data?.meta)
  const [adding, setAdding] = useState<'org' | 'domain' | null>(null)
  const hasOrgs = (organisations.data?.data.length ?? 0) > 0

  return (
    <>
      <PageHeader title="Network Management" description="Organisations and the routing domains that give each address its context." />
      <div className="grid gap-4 xl:grid-cols-3">
        <Card title="Organisations" action={canCreate && <Button variant="ghost" className="h-7 px-2" onClick={() => setAdding('org')}><Plus size={15} aria-hidden /> Add</Button>}>
          {organisations.isPending && <SkeletonRows rows={3} />}
          {organisations.error && <ErrorState error={organisations.error} onRetry={organisations.refetch} />}
          {organisations.data?.data.length === 0 && <EmptyState title="No organisations yet" body="Start here: subnets belong to an organisation's routing domain." />}
          {hasOrgs && (
            <ul>
              {organisations.data?.data.map((o) => (
                <li key={o.id} className="flex items-center justify-between border-b border-line px-4 py-2.5 last:border-0">
                  <span className="font-medium text-ink">{o.name}</span><Badge>{o.code}</Badge>
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
                <thead><tr><Th>Name</Th><Th>Organisation</Th><Th>Type</Th><Th>RD</Th><Th className="text-right">Subnets</Th></tr></thead>
                <tbody>
                  {domains.data.data.map((d) => (
                    <tr key={d.id}>
                      <Td className="font-medium text-ink">{d.name} {d.isIsolated && <Badge tone="warn">Isolated</Badge>}</Td>
                      <Td className="text-ink-2">{d.organisationName}</Td>
                      <Td className="text-ink-2">{domainKinds[d.kind]}</Td>
                      <Td className="text-ink-2">{d.routeDistinguisher ? <Mono>{d.routeDistinguisher}</Mono> : '—'}</Td>
                      <Td className="text-right text-ink-2 tabular-nums">{formatCount(d.subnetCount)}</Td>
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
    </>
  )
}
