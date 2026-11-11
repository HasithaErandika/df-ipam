import { useQuery } from '@tanstack/react-query'
import { AlertTriangle } from 'lucide-react'
import { Badge, Card, EmptyState, ErrorState, Meter, PageHeader, SkeletonRows } from '../../components/ui'
import { get } from '../../lib/api'

type Floor={id:string;name:string;floor?:number;vlan?:number;network:string;assigned:number;seen:number;poolUsed:number;poolSize:number}
type Dashboard={floors:Floor[];alerts:Record<string,number>;serversDown:{environment:string;colour:string;down:number}[]}
export function DashboardPage(){
 const query=useQuery({queryKey:['/dashboard'],queryFn:()=>get<Dashboard>('/dashboard'),refetchInterval:30000})
 const data=query.data
 return <><PageHeader title="Dashboard" description="Network inventory and monitoring at a glance."/>{query.isLoading?<SkeletonRows rows={7}/>:query.isError?<ErrorState error={query.error} onRetry={()=>void query.refetch()}/>:!data?<EmptyState title="Dashboard is unavailable"/>:<div className="space-y-4">
 <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">{['critical','high','medium','low'].map(level=><Card key={level}><div className="flex items-center justify-between p-4"><div><p className="text-xs uppercase text-ink-3">{level} alerts</p><p className="mt-1 text-2xl font-semibold text-ink">{data.alerts[level]??0}</p></div><AlertTriangle size={20} className={level==='critical'?'text-danger':'text-warn'} aria-hidden/></div></Card>)}</div>
 <Card title="Floor networks"><div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-3">{data.floors.length?data.floors.map(f=>{const pct=f.poolSize?Math.min(100,f.poolUsed*100/f.poolSize):0;return <div key={f.id} className="rounded-md border border-line p-3"><div className="flex items-start justify-between"><div><h3 className="font-medium text-ink">{f.name}</h3><p className="font-mono text-xs text-ink-3">{f.network}</p></div><Badge>{f.vlan?`VLAN ${f.vlan}`:'DHCP'}</Badge></div><div className="mt-3"><Meter pct={pct} level={pct>90?'critical':pct>75?'warning':'normal'} label={`${f.name} DHCP pool usage`}/></div><p className="mt-2 text-xs text-ink-3">{f.poolUsed} used in pool · {f.seen} seen · {f.assigned} registered</p></div>}):<EmptyState title="No floor segments" body="Add DHCP floor segments in Settings to see usage."/>}</div></Card>
 <Card title="Servers down by environment"><div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-4">{data.serversDown.length?data.serversDown.map(s=><div key={s.environment} className="flex items-center justify-between rounded border border-line p-3"><span className="flex items-center gap-2"><span className="size-2 rounded-full" style={{backgroundColor:s.colour}}/>{s.environment}</span><span className={s.down?'font-semibold text-danger':'text-ink-3'}>{s.down} down</span></div>):<EmptyState title="No server segments configured"/>}</div></Card>
 </div>}</>
}
