import { useRouteTab } from '../../lib/tabs'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, CheckCheck, Info, OctagonAlert } from 'lucide-react'
import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { Badge, Button, Card, EmptyState, ErrorState, FormError, Pagination, Select, SkeletonRows, Td, Th } from '../../components/ui'
import { RouteTabs } from '../../components/workflow'
import { get, list, post, put, queryString } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'
import type { AppNotification, NotificationPreference, NotificationRule, Summary } from '../../lib/types'

const severityIcon = { info: Info, warning: AlertTriangle, critical: OctagonAlert }

function useInvalidate() {
  const queryClient = useQueryClient()
  return () => queryClient.invalidateQueries({ queryKey: ['notifications'] })
}

function Inbox() {
  const navigate = useNavigate()
  const invalidate = useInvalidate()
  const paging = usePaging()
  const [unread, setUnread] = useState(false)
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, unread })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['notifications', 'list', qs], placeholderData: keepPreviousData, queryFn: () => list<AppNotification>(`/notifications${qs}`),
  })
  useClampPage(paging, data?.meta)
  const read = useMutation({ mutationFn: (id: string) => post(`/notifications/${id}/read`), onSuccess: invalidate })
  const readAll = useMutation({ meta: { success: 'All alerts marked read.' }, mutationFn: () => post('/notifications/read-all'), onSuccess: invalidate })
  const open = (n: AppNotification) => {
    if (!n.readAt) read.mutate(n.id)
    if (n.link) navigate(n.link)
  }
  return (
    <Card>
      <div className="flex items-center justify-between gap-2 border-b border-line p-2.5">
        <Select aria-label="Show" className="w-40!" value={unread ? 'unread' : 'all'} onChange={(e) => { setUnread(e.target.value === 'unread'); paging.setPage(1) }}>
          <option value="all">All alerts</option><option value="unread">Unread only</option>
        </Select>
        <Button busy={readAll.isPending} onClick={() => readAll.mutate()}><CheckCheck size={14} aria-hidden /> Mark all read</Button>
      </div>
      {isPending && <SkeletonRows rows={6} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title={unread ? 'Nothing unread' : 'No alerts yet'} body="Approvals, decisions and reservation reminders appear here." />}
      {data && data.data.length > 0 && (
        <>
          <ul>
            {data.data.map((n) => {
              const Icon = severityIcon[n.severity] ?? Info
              return (
                <li key={n.id} className={`flex items-start gap-3 border-b border-line px-4 py-2.5 ${n.readAt ? '' : 'bg-accent-soft/40'}`}>
                  <Icon size={16} className={`mt-0.5 shrink-0 ${n.severity === 'critical' ? 'text-danger' : n.severity === 'warning' ? 'text-warn' : 'text-accent'}`} aria-label={n.severity} />
                  <button type="button" onClick={() => open(n)} className="min-w-0 flex-1 cursor-pointer text-left">
                    <div className={`text-[13px] ${n.readAt ? 'text-ink' : 'font-semibold text-ink'}`}>{n.title}</div>
                    <div className="mt-0.5 line-clamp-2 text-[12.5px] whitespace-pre-line text-ink-2">{n.body}</div>
                  </button>
                  <div className="shrink-0 text-right">
                    <div className="text-[12px] text-ink-3">{formatDateTime(n.createdAt)}</div>
                    {!n.readAt && <button type="button" className="mt-0.5 cursor-pointer text-[12px] text-accent hover:underline" onClick={() => read.mutate(n.id)}>Mark read</button>}
                  </div>
                </li>
              )
            })}
          </ul>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
    </Card>
  )
}

function Preferences() {
  const queryClient = useQueryClient()
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['notifications', 'preferences'], queryFn: () => get<NotificationPreference[]>('/notifications/preferences') })
  const save = useMutation({ meta: { success: 'Preference saved.', inlineError: true },
    mutationFn: (p: Omit<NotificationPreference, 'label'>) => put('/notifications/preferences', [p]),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notifications', 'preferences'] }),
  })
  const labels = Object.fromEntries((data ?? []).map((p) => [p.eventType, p.label]))
  const events = [...new Set((data ?? []).map((p) => p.eventType))]
  const value = (eventType: string, channel: string) => data?.find((p) => p.eventType === eventType && p.channel === channel)?.isEnabled ?? true
  return (
    <Card title="My notification settings">
      <p className="border-b border-line px-4 py-2 text-[12.5px] text-ink-2">Choose how you hear about each event. Email is sent only when the server has a mail provider configured.</p>
      {isPending && <SkeletonRows rows={5} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && (
        <table className="w-full border-collapse">
          <thead><tr><Th>Event</Th><Th className="text-center">In the app</Th><Th className="text-center">Email</Th></tr></thead>
          <tbody>
            {events.map((e) => (
              <tr key={e}>
                <Td className="text-ink">{labels[e] ?? e.replaceAll('_', ' ')}</Td>
                {(['in_app', 'email'] as const).map((ch) => (
                  <Td key={ch} className="text-center">
                    <input type="checkbox" aria-label={`${labels[e] ?? e} ${ch === 'email' ? 'by email' : 'in the app'}`} checked={value(e, ch)}
                      disabled={save.isPending} onChange={(ev) => save.mutate({ eventType: e, channel: ch, isEnabled: ev.target.checked })} />
                  </Td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <div className="px-4 py-2"><FormError error={save.error} /></div>
    </Card>
  )
}

function Rules({ rules, isPending, error, refetch }: { rules: NotificationRule[] | undefined; isPending: boolean; error: unknown; refetch: () => void }) {
  const queryClient = useQueryClient()
  const save = useMutation({ meta: { success: 'Rule saved.', inlineError: true },
    mutationFn: (r: Pick<NotificationRule, 'eventType'> & Partial<NotificationRule>) => put(`/notifications/rules/${r.eventType}`, { severity: r.severity, isEnabled: r.isEnabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notifications', 'rules'] }),
  })
  return (
    <Card title="Event rules">
      <p className="border-b border-line px-4 py-2 text-[12.5px] text-ink-2">Switch an event off for everyone, or change its severity. Each event reaches a person at most once, so repeats don't flood anyone.</p>
      {isPending && <SkeletonRows rows={5} />}
      {error ? <ErrorState error={error} onRetry={refetch} /> : null}
      {rules && (
        <table className="w-full border-collapse">
          <thead><tr><Th>Event</Th><Th>Sent to</Th><Th>Severity</Th><Th className="text-center">On</Th></tr></thead>
          <tbody>
            {rules.map((r) => (
              <tr key={r.eventType}>
                <Td className="text-ink">{r.label}</Td>
                <Td className="text-ink-2">{r.recipients}</Td>
                <Td>
                  <Select aria-label={`${r.label} severity`} className="w-32!" value={r.severity} disabled={save.isPending}
                    onChange={(e) => save.mutate({ eventType: r.eventType, severity: e.target.value })}>
                    <option value="info">Info</option><option value="warning">Warning</option><option value="critical">Critical</option>
                  </Select>
                </Td>
                <Td className="text-center">
                  <input type="checkbox" aria-label={`${r.label} on`} checked={r.isEnabled} disabled={save.isPending}
                    onChange={(e) => save.mutate({ eventType: r.eventType, isEnabled: e.target.checked })} />
                </Td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <div className="px-4 py-2"><FormError error={save.error} /></div>
    </Card>
  )
}

function Attention() {
  const canView = useCan('inventory:view')
  const { data } = useQuery({ queryKey: ['summary'], enabled: canView, queryFn: () => get<Summary>('/dashboard/summary') })
  if (!data) return null
  const items = [
    { n: data.pendingApprovals, label: 'requests waiting for approval', to: '/requests/all', tone: 'warn' as const },
    { n: data.readyToFulfil, label: 'approved requests ready to fulfil', to: '/requests/all', tone: 'accent' as const },
    { n: data.reservationsExpiringSoon, label: 'reservations ending within a week', to: '/reservations', tone: 'warn' as const },
    { n: data.conflictSuspected, label: 'addresses with a suspected conflict', to: '/reconciliation', tone: 'danger' as const },
    { n: data.openFindings, label: 'open reconciliation findings', to: '/reconciliation', tone: 'accent' as const },
  ].filter((i) => i.n > 0)
  if (items.length === 0) return null
  return (
    <div className="mb-3 flex flex-wrap gap-1.5">
      {items.map((i) => (
        <Link key={i.label} to={i.to} className="flex h-7 items-center gap-1.5 rounded-[5px] border border-line bg-surface px-2 text-[12.5px] text-ink-2 hover:border-line-strong">
          <Badge tone={i.tone}>{i.n}</Badge> {i.label}
        </Link>
      ))}
    </div>
  )
}

export function Alerts() {
  const canManage = useCan('users:manage')
  const rules = useQuery({ queryKey: ['notifications', 'rules'], enabled: canManage, queryFn: () => get<NotificationRule[]>('/notifications/rules') })
  const tabs = [{ key: 'inbox', label: 'My alerts' }, { key: 'settings', label: 'Settings' }, ...(canManage ? [{ key: 'rules', label: 'Event rules' }] : [])]
  const tab = useRouteTab(tabs.map((t) => t.key))
  return (
    <>
      <div className="mb-4">
        <h1 className="text-xl leading-tight font-semibold tracking-tight text-ink">Alerts and Notifications</h1>
        <p className="mt-0.5 max-w-2xl text-[13px] text-ink-2">What needs your attention, and how you hear about it.</p>
      </div>
      <Attention />
      <RouteTabs tabs={tabs} active={tab} base="/alerts" />
      {tab === 'inbox' && <Inbox />}
      {tab === 'settings' && <Preferences />}
      {tab === 'rules' && <Rules rules={rules.data} isPending={rules.isPending} error={rules.error} refetch={rules.refetch} />}
      <p className="mt-3 text-[12px] text-ink-3">Alerts for discovery, utilisation and integrations arrive with those features.</p>
    </>
  )
}
