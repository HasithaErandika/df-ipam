import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import type { NavItem } from '../../app/nav'
import { Button, Card, EmptyState, ErrorState, Field, Input, PageHeader, Select, SkeletonRows, Td, Th } from '../../components/ui'
import { download, get, post, upload } from '../../lib/api'

type Segment = { id: string; name: string; kind: string; floor?: number; vlan?: number; network: string; gateway?: string; dhcpStart?: string; dhcpEnd?: string; description: string }
type Device = { id: string; mac: string; name: string; owner: string; department: string; deviceType: string; status: string }
type IP = { id: string; ip: string; segmentId: string; status: string; ipType: string; assignedName: string; owner: string; monitor: boolean; health: string }

export function InventoryPage({ item }: { item: NavItem }) {
  const client = useQueryClient()
  const isDevices = item.to === '/devices'
  const isIP = item.to === '/servers'
  const path = isDevices ? '/devices' : isIP ? '/ip-addresses' : '/segments'
  const query = useQuery({ queryKey: [path], queryFn: () => get<(Device | Segment | IP)[]>(path) })
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [network, setNetwork] = useState('')
  const [kind, setKind] = useState('dhcp')
  const [mac, setMac] = useState('')
  const [ip, setIP] = useState('')
  const [segmentId, setSegmentId] = useState('')
  const mutation = useMutation({
    mutationFn: async () => {
      if (isDevices) return post('/devices', { mac, name, deviceType: 'other', status: 'approved' })
      if (isIP) return post('/ip-addresses', { segmentId, ip, status: 'assigned', ipType: 'server', monitor: true })
      return post('/segments', { name, kind, network, description: '' })
    },
    onSuccess: () => { setError(''); setName(''); setNetwork(''); setMac(''); setIP(''); void client.invalidateQueries({ queryKey: [path] }) },
    onError: (e: unknown) => setError(e instanceof Error ? e.message : 'Could not save this record.'),
  })
  const decision = useMutation({
    mutationFn: ({ id, status }: { id: string; status: 'approved' | 'rejected' }) => post(`/devices/${id}/decision`, { status }),
    onSuccess: () => void client.invalidateQueries({ queryKey: [path] }),
  })
  const importCSV = useMutation({ mutationFn: (file: File) => upload<{ imported: number }>('/devices/import.csv', file), onSuccess: () => void client.invalidateQueries({ queryKey: [path] }) })
  const segments = useQuery({ queryKey: ['/segments'], queryFn: () => get<Segment[]>('/segments'), enabled: isIP })
  const rows = query.data ?? []
  return <>
    <PageHeader title={item.label} description={item.summary} />
    {isDevices ? <Card title="Add known device" className="mb-4"><div className="grid gap-3 p-4 sm:grid-cols-[1fr_1fr_auto]">
      <Field label="MAC address" htmlFor="device-mac"><Input id="device-mac" value={mac} onChange={e => setMac(e.target.value)} placeholder="00:11:22:33:44:55" /></Field>
      <Field label="Device name" htmlFor="device-name"><Input id="device-name" value={name} onChange={e => setName(e.target.value)} /></Field>
      <Button className="self-end" variant="primary" busy={mutation.isPending} onClick={() => mutation.mutate()}>Add device</Button>
    </div></Card> : isIP ? <Card title="Add server IP" className="mb-4"><div className="grid gap-3 p-4 sm:grid-cols-[1fr_1fr_auto]">
      <Field label="Server segment" htmlFor="server-segment"><Select id="server-segment" value={segmentId} onChange={e => setSegmentId(e.target.value)}><option value="">Choose a segment</option>{(segments.data ?? []).filter(s=>s.kind==='server').map(s=><option key={s.id} value={s.id}>{s.name} · {s.network}</option>)}</Select></Field>
      <Field label="IP address" htmlFor="server-ip"><Input id="server-ip" value={ip} onChange={e=>setIP(e.target.value)} /></Field>
      <Button className="self-end" variant="primary" busy={mutation.isPending} onClick={()=>mutation.mutate()}>Register IP</Button>
    </div></Card> : <Card title="Add segment" className="mb-4"><div className="grid gap-3 p-4 sm:grid-cols-[1fr_1fr_1fr_auto]">
      <Field label="Name" htmlFor="seg-name"><Input id="seg-name" value={name} onChange={e=>setName(e.target.value)} placeholder="Floor 1" /></Field>
      <Field label="Type" htmlFor="seg-kind"><Select id="seg-kind" value={kind} onChange={e=>setKind(e.target.value)}><option value="dhcp">Floor (DHCP)</option><option value="server">Server</option></Select></Field>
      <Field label="Network" htmlFor="seg-network"><Input id="seg-network" value={network} onChange={e=>setNetwork(e.target.value)} placeholder="192.168.1.0/24" /></Field>
      <Button className="self-end" variant="primary" busy={mutation.isPending} onClick={()=>mutation.mutate()}>Add segment</Button>
    </div></Card>}
    {error && <p role="alert" className="mb-3 text-sm text-danger">{error}</p>}
    <Card title={isDevices ? 'Known devices' : isIP ? 'IP register' : 'Configured segments'} action={isDevices ? <div className="flex items-center gap-2"><label className="cursor-pointer rounded border border-line-strong px-3 py-1.5 text-xs">Import CSV<input className="sr-only" type="file" accept=".csv,text/csv" onChange={e=>{const file=e.target.files?.[0];if(file)importCSV.mutate(file)}} /></label><Button onClick={() => void download('/devices/export.csv','devices.csv')}>Export CSV</Button></div> : undefined}>
      {query.isLoading ? <SkeletonRows /> : query.isError ? <ErrorState error={query.error} onRetry={() => void query.refetch()} /> : rows.length === 0 ? <EmptyState title="Nothing here yet" body="Add the first record using the form above." /> : <div className="overflow-x-auto"><table className="w-full"><thead><tr>{(isDevices ? ['MAC address','Name','Owner','Department','Status','Decision'] : isIP ? ['IP address','Segment','Status','Type','Monitor','Health'] : ['Name','Type','Floor','VLAN','Network','DHCP pool']).map(x=><Th key={x}>{x}</Th>)}</tr></thead><tbody>{rows.map((row:any)=><tr key={row.id}>{isDevices ? <><Td><code>{row.mac}</code></Td><Td>{row.name||'—'}</Td><Td>{row.owner||'—'}</Td><Td>{row.department||'—'}</Td><Td>{row.status}</Td><Td>{row.status==='pending'?<div className="flex gap-1"><Button busy={decision.isPending} onClick={()=>decision.mutate({id:row.id,status:'approved'})}>Approve</Button><Button variant="danger" busy={decision.isPending} onClick={()=>decision.mutate({id:row.id,status:'rejected'})}>Reject</Button></div>:'—'}</Td></> : isIP ? <><Td><code>{row.ip}</code></Td><Td>{segments.data?.find(s=>s.id===row.segmentId)?.name||'—'}</Td><Td>{row.status}</Td><Td>{row.ipType}</Td><Td>{row.monitor?'On':'Off'}</Td><Td>{row.health}</Td></> : <><Td>{row.name}</Td><Td>{row.kind}</Td><Td>{row.floor??'—'}</Td><Td>{row.vlan??'—'}</Td><Td><code>{row.network}</code></Td><Td>{row.dhcpStart&&row.dhcpEnd?`${row.dhcpStart} – ${row.dhcpEnd}`:'—'}</Td></>}</tr>)}</tbody></table></div>}
    </Card>
  </>
}
