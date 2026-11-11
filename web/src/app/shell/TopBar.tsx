import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, Check, IdCard, LogOut, UserRound } from 'lucide-react'
import { useState } from 'react'
import { get, post } from '../../lib/api'
import { signOut } from '../../lib/auth'
import { ProfileModal } from './ProfileModal'

const menuItem = 'flex h-8 cursor-pointer items-center gap-2 rounded-[4px] px-2.5 text-[13px] text-ink-2 outline-none data-[highlighted]:bg-bg data-[highlighted]:text-ink'

export function UserMenu() {
  const queryClient = useQueryClient()
  const [details, setDetails] = useState(false)
  return (
    <>
      {/* Not modal: a modal menu would fight the details dialog for focus as it closes. */}
      <DropdownMenu.Root modal={false}>
        <DropdownMenu.Trigger aria-label="Account menu" title="Account"
          className="grid size-8 cursor-pointer place-items-center rounded-full bg-accent-soft text-accent hover:bg-accent hover:text-white data-[state=open]:bg-accent data-[state=open]:text-white">
          <UserRound size={17} strokeWidth={1.75} aria-hidden />
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content align="end" sideOffset={6}
            className="z-30 w-44 rounded-md border border-line-strong/70 bg-surface p-1 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]">
            <DropdownMenu.Item onSelect={() => setDetails(true)} className={menuItem}>
              <IdCard size={14} aria-hidden /> Details
            </DropdownMenu.Item>
            <DropdownMenu.Item onSelect={async () => { await signOut(); queryClient.clear() }} className={menuItem}>
              <LogOut size={14} aria-hidden /> Sign out
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      <ProfileModal open={details} onClose={() => setDetails(false)} />
      <NotificationBell />
    </>
  )
}

type Notice={id:string;kind:string;message:string;link:string;isRead:boolean;createdAt:string}
function NotificationBell(){
  const client=useQueryClient();const [open,setOpen]=useState(false)
  const query=useQuery({queryKey:['/notifications'],queryFn:()=>get<{items:Notice[];unreadCount:number}>('/notifications'),refetchInterval:30000})
  const mark=useMutation({mutationFn:(id:string)=>post(`/notifications/${id}/read`),onSuccess:()=>client.invalidateQueries({queryKey:['/notifications']})})
  const all=useMutation({mutationFn:()=>post('/notifications/read-all'),onSuccess:()=>client.invalidateQueries({queryKey:['/notifications']})})
  return <DropdownMenu.Root open={open} onOpenChange={setOpen} modal={false}><DropdownMenu.Trigger aria-label="Notifications" title="Notifications" className="relative grid size-8 cursor-pointer place-items-center rounded-full text-ink-2 hover:bg-surface-2"><Bell size={17}/>{Boolean(query.data?.unreadCount)&&<span className="absolute right-0.5 top-0.5 size-2 rounded-full bg-danger"/>}</DropdownMenu.Trigger><DropdownMenu.Portal><DropdownMenu.Content align="end" sideOffset={8} className="z-30 w-[min(24rem,calc(100vw-2rem))] rounded-md border border-line-strong bg-surface p-2 shadow-xl"><div className="flex items-center justify-between px-2 py-1"><span className="text-sm font-semibold text-ink">Notifications {query.data?.unreadCount?`(${query.data.unreadCount})`:''}</span>{Boolean(query.data?.unreadCount)&&<button type="button" className="text-xs text-accent hover:underline" onClick={()=>all.mutate()}>Mark all read</button>}</div>{query.isLoading?<p className="p-3 text-sm text-ink-3">Loading…</p>:query.isError?<p className="p-3 text-sm text-danger">Notifications could not be loaded.</p>:!query.data?.items.length?<p className="p-3 text-sm text-ink-3">No notifications yet.</p>:<div className="max-h-80 overflow-y-auto">{query.data.items.map(n=><div key={n.id} className={`flex gap-2 border-t border-line px-2 py-2 ${n.isRead?'':'bg-accent-soft/40'}`}><div className="min-w-0 flex-1"><p className="text-xs text-ink">{n.message}</p><time className="text-[10px] text-ink-3">{new Date(n.createdAt).toLocaleString()}</time></div>{!n.isRead&&<button type="button" title="Mark read" onClick={()=>mark.mutate(n.id)} className="text-ink-3 hover:text-accent"><Check size={15}/></button>}</div>)}</div>}</DropdownMenu.Content></DropdownMenu.Portal></DropdownMenu.Root>
}
