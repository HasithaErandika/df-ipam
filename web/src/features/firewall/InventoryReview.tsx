import { useMutation } from '@tanstack/react-query'
import { CheckCircle2, RefreshCw, XCircle } from 'lucide-react'
import { useState } from 'react'
import { Button, Card, FormError, Notice, useConfirm } from '../../components/ui'
import { errorMessage, get } from '../../lib/api'
import { formatAgo } from '../../lib/format'
import { inventoryKeys, useInvalidate } from '../../lib/queries'
import type { FirewallInventory } from '../../lib/types'
import { DevicesReview } from './DevicesReview'
import { notConnected } from './LiveView'
import { NetworksReview } from './NetworksReview'
import { ServersReview } from './ServersReview'

export type InventoryKind = 'networks' | 'devices' | 'servers'

/** One confirmed item: what it is, and the request that saves it through the normal route. */
export interface SaveTask { label: string; run: () => Promise<unknown> }

interface SaveResult { label: string; ok: boolean; error?: string }

/**
 * Networks, devices and servers from the firewall. One fetch serves all three
 * tabs. Each confirmed item is saved through the route the rest of DF-IPAM
 * uses, one at a time, so every check, permission and audit record applies.
 */
export function InventoryReview({ kind }: { kind: InventoryKind }) {
  const confirm = useConfirm()
  const refresh = useInvalidate(...inventoryKeys)
  const [results, setResults] = useState<SaveResult[] | null>(null)
  const fetch = useMutation({ mutationFn: () => get<FirewallInventory>('/firewall/inventory'), meta: { inlineError: true }, onSuccess: () => setResults(null) })
  const save = useMutation({
    mutationFn: async (tasks: SaveTask[]) => {
      const out: SaveResult[] = []
      for (const t of tasks) {
        try { await t.run(); out.push({ label: t.label, ok: true }) } catch (e) { out.push({ label: t.label, ok: false, error: errorMessage(e) }) }
      }
      return out
    },
    meta: { success: (_: SaveTask[], r: SaveResult[]) => `${r.filter((x) => x.ok).length} of ${r.length} saved.` },
    onSuccess: (r) => { setResults(r); void refresh(); fetch.mutate() },
  })
  const apply = async (tasks: SaveTask[], what: string) => {
    if (tasks.length === 0) return
    const ok = await confirm({ title: `Save ${tasks.length} ${what}?`, body: 'Each is saved with the normal checks and recorded in the audit log. The firewall is fetched again afterwards.', confirmLabel: 'Save', tone: 'primary', changes: tasks.map((t) => t.label) })
    if (ok) save.mutate(tasks)
  }
  const inv = fetch.data
  const props = { inv: inv!, busy: save.isPending, onSave: (tasks: SaveTask[], what: string) => void apply(tasks, what) }
  return (
    <div className="space-y-4">
      <Card>
        <div className="flex flex-wrap items-center gap-3 p-4">
          <Button variant="primary" busy={fetch.isPending} onClick={() => fetch.mutate()}><RefreshCw size={14} aria-hidden />{inv ? 'Fetch again' : 'Fetch from firewall'}</Button>
          {inv
            ? <span className="text-[13px] text-ink-2">{inv.firewall}, fetched {formatAgo(inv.at)}: {inv.networks.length} networks, {inv.devices.length} devices, {inv.servers.length} named hosts.</span>
            : <span className="text-[13px] text-ink-3">Only reads the firewall. Nothing is saved until you confirm.</span>}
        </div>
        {fetch.isError && <div className="px-4 pb-4">{notConnected(fetch.error) ? <Notice tone="warn">No firewall is connected. An administrator connects it in System &gt; FortiGate.</Notice> : <FormError error={fetch.error} />}</div>}
        {inv && inv.problems.length > 0 && <div className="px-4 pb-4"><Notice tone="warn">Not everything could be read: {inv.problems.join(' · ')}.</Notice></div>}
      </Card>
      {results && (
        <Card title="Saved">
          <ul className="divide-y divide-line">{results.map((r, i) => (
            <li key={i} className="flex items-center gap-2 px-4 py-2 text-[13px]">
              {r.ok ? <CheckCircle2 size={15} className="text-ok" aria-hidden /> : <XCircle size={15} className="text-danger" aria-hidden />}
              <span>{r.label}</span>{r.error && <span className="text-danger">· {r.error}</span>}
            </li>
          ))}</ul>
        </Card>
      )}
      {inv && kind === 'networks' && <NetworksReview {...props} />}
      {inv && kind === 'devices' && <DevicesReview {...props} />}
      {inv && kind === 'servers' && <ServersReview {...props} />}
    </div>
  )
}

export interface ReviewProps { inv: FirewallInventory; busy: boolean; onSave: (tasks: SaveTask[], what: string) => void }
