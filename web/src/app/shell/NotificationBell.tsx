import * as Popover from '@radix-ui/react-dropdown-menu'
import { useMutation, useQuery } from '@tanstack/react-query'
import { AlertTriangle, Bell, CheckCheck, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { Badge, ErrorState, SkeletonRows } from '../../components/ui'
import { get, list, post } from '../../lib/api'
import { formatAgo } from '../../lib/format'
import { label, notificationKinds } from '../../lib/labels'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { Notice } from '../../lib/types'

/**
 * The bell checks the unread count once a minute, and only loads the messages
 * when opened. Browsers pause the check in background tabs.
 */
export function NotificationBell() {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const refresh = useInvalidate('/notifications')
  const count = useQuery({ queryKey: ['/notifications/unread-count'], queryFn: () => get<{ count: number }>('/notifications/unread-count'), refetchInterval: 60_000 })
  const items = useQuery({ queryKey: ['/notifications', 'bell'], queryFn: () => list<Notice>('/notifications?pageSize=15'), enabled: open })
  const read = useMutation({ mutationFn: (id: string) => post(`/notifications/${id}/read`), onSuccess: refresh, meta: { success: 'Notification marked read.' } })
  const readAll = useMutation({ mutationFn: () => post('/notifications/read-all'), onSuccess: refresh, meta: { success: 'All notifications marked read.' } })
  const unread = count.data?.count ?? 0
  const canView = useCan('inventory:view')
  const alerts = useQuery({ queryKey: ['/alerts/summary'], queryFn: () => get<{ open: number; critical: number; high: number }>('/alerts/summary'), enabled: canView, refetchInterval: 60_000 })
  const openAlerts = alerts.data?.open ?? 0
  const urgent = (alerts.data?.critical ?? 0) + (alerts.data?.high ?? 0)

  const openItem = (n: Notice) => {
    if (!n.isRead) read.mutate(n.id)
    setOpen(false)
    if (n.link) navigate(n.link)
  }

  return (
    <Popover.Root open={open} onOpenChange={setOpen} modal={false}>
      <Popover.Trigger aria-label={unread ? `Notifications, ${unread} unread` : 'Notifications'} title="Notifications"
        className="relative grid size-8 cursor-pointer place-items-center rounded-full text-ink-2 hover:bg-surface-2 hover:text-ink data-[state=open]:bg-surface-2">
        <Bell size={17} strokeWidth={1.75} aria-hidden />
        {unread === 0 && openAlerts > 0 && <span aria-hidden className={`absolute top-1 right-1 size-2 rounded-full ${urgent ? 'bg-danger' : 'bg-warn'}`} />}
        {unread > 0 && (
          <span aria-hidden className="absolute -top-0.5 -right-0.5 grid h-4 min-w-4 place-items-center rounded-full bg-danger px-1 text-[10px] font-semibold text-white tabular-nums">
            {unread > 99 ? '99+' : unread}
          </span>
        )}
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content align="end" sideOffset={8} className="z-30 w-[min(26rem,calc(100vw-2rem))] overflow-hidden rounded-md border border-line-strong/70 bg-surface shadow-popover">
          <div className="flex items-center justify-between border-b border-line px-3.5 py-2.5">
            <span className="text-[13px] font-semibold text-ink">Notifications</span>
            {unread > 0 && (
              <button type="button" disabled={readAll.isPending} onClick={() => readAll.mutate()} className="inline-flex cursor-pointer items-center gap-1 text-[12.5px] font-medium text-accent hover:underline disabled:opacity-50">
                <CheckCheck size={14} aria-hidden /> Mark all read
              </button>
            )}
          </div>
          {openAlerts > 0 && (
            <Popover.Item onSelect={() => { setOpen(false); navigate('/alerts') }}
              className={`flex cursor-pointer items-center gap-2.5 border-b border-line px-3.5 py-2.5 outline-none data-[highlighted]:brightness-[.98] ${urgent ? 'bg-danger-soft' : 'bg-warn-soft'}`}>
              <AlertTriangle size={16} className={urgent ? 'text-danger' : 'text-warn'} aria-hidden />
              <span className="min-w-0 flex-1 text-[13px] text-ink"><strong>{openAlerts}</strong> open alert{openAlerts === 1 ? '' : 's'}{urgent ? `, ${urgent} High or Critical` : ''}</span>
              <ChevronRight size={15} className="text-ink-3" aria-hidden />
            </Popover.Item>
          )}
          <div className="max-h-[22rem] overflow-y-auto">
            {items.isPending ? <SkeletonRows rows={4} />
              : items.isError ? <ErrorState error={items.error} onRetry={() => void items.refetch()} />
              : items.data.data.length === 0 ? <p className="px-4 py-10 text-center text-[13px] text-ink-3">No notifications yet. New alerts, request decisions, servers back up and expiring reservations appear here.</p>
              : items.data.data.map((n) => (
                <Popover.Item key={n.id} onSelect={() => openItem(n)}
                  className="flex cursor-pointer gap-2.5 border-b border-line px-3.5 py-2.5 outline-none last:border-0 data-[highlighted]:bg-surface-2">
                  <span aria-hidden className={`mt-1.5 size-2 shrink-0 rounded-full ${n.isRead ? 'bg-transparent' : 'bg-accent'}`} />
                  <div className="min-w-0 flex-1">
                    <p className={`text-[13px] leading-snug ${n.isRead ? 'text-ink-2' : 'font-medium text-ink'}`}>{n.message}</p>
                    <div className="mt-1 flex items-center gap-2 text-[11.5px] text-ink-3">
                      <Badge>{label(notificationKinds, n.kind).replace(' (Admins)', '')}</Badge>
                      <time dateTime={n.createdAt} title={new Date(n.createdAt).toLocaleString('en-GB')}>{formatAgo(n.createdAt)}</time>
                    </div>
                  </div>
                </Popover.Item>
              ))}
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}
