import { useMutation, useQuery } from '@tanstack/react-query'
import { Copy, Plus } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, SkeletonRows, Td, Th, useConfirm } from '../../components/ui'
import { fieldIssues, list, post } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useCan } from '../../lib/session'
import { useRefresh } from './shared'
import type { Agent } from './shared'

export function Agents() {
  const refresh = useRefresh()
  const canManage = useCan('discovery:manage')
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('')
  const [token, setToken] = useState<{ token: string; agent: string; expiresAt: string } | null>(null)
  const [revoking, setRevoking] = useState<Agent | null>(null)
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['discovery', 'agents'], refetchInterval: 30000, queryFn: () => list<Agent>('/agents') })
  type TokenResponse = { agent: Agent; registrationToken: string; expiresAt: string }
  const show = (r: TokenResponse) => { refresh(); setToken({ token: r.registrationToken, agent: r.agent.name, expiresAt: r.expiresAt }) }
  const confirm = useConfirm()
  const create = useMutation({ meta: { success: 'Agent created. Copy its registration token now.', inlineError: true }, mutationFn: () => post<TokenResponse>('/agents', { name }), onSuccess: (r) => { setCreating(false); setName(''); show(r) } })
  const rotate = useMutation({ meta: { success: 'New registration token issued.' }, mutationFn: (id: string) => post<TokenResponse>(`/agents/${id}/rotate`), onSuccess: show })
  const revoke = useMutation({ meta: { success: 'Agent revoked.', inlineError: true }, mutationFn: (id: string) => post(`/agents/${id}/revoke`), onSuccess: () => { refresh(); setRevoking(null) } })
  const tone = (s: string) => (s === 'active' ? 'ok' : s === 'revoked' ? 'danger' : s === 'offline' ? 'warn' : 'neutral') as 'ok'
  return (
    <Card title="Agents" action={canManage && <Button variant="primary" className="h-7" onClick={() => setCreating(true)}><Plus size={14} aria-hidden /> New agent</Button>}>
      <p className="border-b border-line px-4 py-2 text-[12.5px] text-ink-2">Agents run inside network segments, pull approved jobs and post results. They never connect to the database.</p>
      {isPending && <SkeletonRows rows={3} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No agents yet" body="Create one, then start the agent with its registration token." />}
      {data && data.data.length > 0 && (
        <table className="w-full border-collapse">
          <thead><tr><Th>Name</Th><Th>Site</Th><Th>Version</Th><Th>Last heartbeat</Th><Th>Status</Th>{canManage && <Th />}</tr></thead>
          <tbody>{data.data.map((a) => (
            <tr key={a.id}>
              <Td className="font-medium text-ink">{a.name}</Td>
              <Td className="text-ink-2">{a.siteName ?? '—'}</Td>
              <Td className="text-ink-2">{a.version || '—'}</Td>
              <Td className="text-ink-2">{a.lastHeartbeatAt ? formatDateTime(a.lastHeartbeatAt) : 'Never'}</Td>
              <Td><Badge tone={tone(a.status)}>{a.status}{a.registrationPending ? ' · token issued' : ''}</Badge></Td>
              {canManage && <Td className="text-right whitespace-nowrap">{a.status !== 'revoked' && <>
                <Button variant="ghost" className="h-7" onClick={async () => { if (await confirm({ title: `Rotate the credential of ${a.name}?`, body: 'A new one-time token is issued. The current credential keeps working until the agent registers with that token.', confirmLabel: 'Rotate credential' })) rotate.mutate(a.id) }}>Rotate credential</Button>
                <Button variant="ghost" className="h-7" onClick={() => { revoke.reset(); setRevoking(a) }}>Revoke</Button>
              </>}</Td>}
            </tr>
          ))}</tbody>
        </table>
      )}
      <Modal open={creating} onClose={() => setCreating(false)} title="New agent"
        footer={<><Button onClick={() => setCreating(false)}>Cancel</Button><Button variant="primary" busy={create.isPending} onClick={() => create.mutate()}>Create agent</Button></>}>
        <Field label="Name" htmlFor="agent-name" error={fieldIssues(create.error).name}><Input id="agent-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="agent-cmb-dc1" /></Field>
        <div className="mt-3"><FormError error={create.error} /></div>
      </Modal>
      <Modal open={token !== null} onClose={() => setToken(null)} title={`Registration token for ${token?.agent}`} footer={<Button variant="primary" onClick={() => setToken(null)}>I've copied it</Button>}>
        <p className="text-[13px] text-ink-2">Shown once. It works a single time, until {token && formatDateTime(token.expiresAt)}. Start the agent with:</p>
        <pre className="mt-2 overflow-x-auto rounded-[5px] bg-surface-2 p-2 font-mono text-[12px]">AGENT_API_URL={window.location.origin} AGENT_REGISTRATION_TOKEN={token?.token} ./agent</pre>
        <Button className="mt-2" onClick={() => token && navigator.clipboard?.writeText(token.token)}><Copy size={14} aria-hidden /> Copy token</Button>
      </Modal>
      <ConfirmDialog open={revoking !== null} title={`Revoke ${revoking?.name}?`} body="Its credential stops working at once, and its running jobs fail. Create a new agent to replace it."
        confirmLabel="Revoke" busy={revoke.isPending} error={revoke.error} onConfirm={() => revoking && revoke.mutate(revoking.id)} onCancel={() => setRevoking(null)} />
    </Card>
  )
}
