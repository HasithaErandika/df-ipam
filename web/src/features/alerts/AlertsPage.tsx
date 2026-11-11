import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Button, Card, EmptyState, ErrorState, PageHeader, Select, SkeletonRows, Td, Th } from '../../components/ui'
import { get, post, put } from '../../lib/api'
import { useCan } from '../../lib/session'

type AlertRow={id:string;type:string;severity:string;status:string;message:string;ip?:string;mac?:string;openedAt:string}
type Preference={kind:string;email:boolean}

export function AlertsPage(){
 const [status,setStatus]=useState('open');const client=useQueryClient();const canManage=useCan('alerts:manage')
 const query=useQuery({queryKey:['/alerts',status],queryFn:()=>get<AlertRow[]>(`/alerts?status=${status}`)})
 const preferences=useQuery({queryKey:['/notification-preferences'],queryFn:()=>get<Preference[]>('/notification-preferences')})
 const ack=useMutation({mutationFn:(id:string)=>post(`/alerts/${id}/acknowledge`),onSuccess:()=>client.invalidateQueries({queryKey:['/alerts']})})
 const savePreference=useMutation({mutationFn:(value:Preference)=>put('/notification-preferences',value),onSuccess:()=>client.invalidateQueries({queryKey:['/notification-preferences']})})
 return <>
  <PageHeader title="Alerts" description="Detected network and server issues." actions={<Select aria-label="Alert status" value={status} onChange={e=>setStatus(e.target.value)} className="w-36"><option value="open">Open</option><option value="acknowledged">Acknowledged</option><option value="closed">Closed</option><option value="all">All</option></Select>}/>
  <Card title="Alert history">{query.isLoading?<SkeletonRows/>:query.isError?<ErrorState error={query.error} onRetry={()=>void query.refetch()}/>:!query.data?.length?<EmptyState title="No alerts" body="Alerts will appear when scans find an issue."/>:<div className="overflow-x-auto"><table className="w-full"><thead><tr>{['Severity','Alert','Address','Status','Opened','Action'].map(x=><Th key={x}>{x}</Th>)}</tr></thead><tbody>{query.data.map(a=><tr key={a.id}><Td><span className={a.severity==='critical'?'text-danger font-semibold':a.severity==='high'?'text-warn font-semibold':''}>{a.severity}</span></Td><Td>{a.message}</Td><Td><code>{a.ip||a.mac||'—'}</code></Td><Td>{a.status}</Td><Td>{new Date(a.openedAt).toLocaleString()}</Td><Td>{canManage&&a.status==='open'?<Button busy={ack.isPending} onClick={()=>ack.mutate(a.id)}>Acknowledge</Button>:'—'}</Td></tr>)}</tbody></table></div>}</Card>
  <Card title="Email notifications" className="mt-4">{preferences.isLoading?<SkeletonRows rows={2}/>:preferences.isError?<ErrorState error={preferences.error} onRetry={()=>void preferences.refetch()}/>:<div className="divide-y divide-line px-4">{preferences.data?.map(p=><label key={p.kind} className="flex cursor-pointer items-center justify-between py-3 text-sm"><span className="capitalize text-ink">{p.kind.replaceAll('_',' ')}</span><input type="checkbox" checked={p.email} disabled={savePreference.isPending} onChange={e=>savePreference.mutate({kind:p.kind,email:e.target.checked})}/></label>)}</div>}</Card>
 </>
}
