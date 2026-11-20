import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { Card, EmptyState, useConfirm } from '../../components/ui'
import { errorMessage, get } from '../../lib/api'
import { inventoryKeys, useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { FirewallInventory, FirewallReview } from '../../lib/types'
import { DevicesReview } from './DevicesReview'
import { FetchError, KindTabs, SavedResults, SourceBar, StartPanel, type KindTab, type SavedItem } from './ImportParts'
import { LinksReview } from './LinksReview'
import { NetworksReview } from './NetworksReview'
import { openCounts } from './reviewCounts'
import { ServersReview } from './ServersReview'

/** One confirmed item: what it is, and the request that saves it through the normal route. */
export interface SaveTask { label: string; run: () => Promise<unknown> }

export interface ReviewProps { inv: FirewallInventory; busy: boolean; onSave: (tasks: SaveTask[], what: string) => void }

/**
 * Import from the FortiGate: one fetch reads everything this person may bring
 * in, a tab per kind shows what waits for a decision, and each confirmed item
 * is saved through the route the rest of DF-IPAM uses, so every check,
 * permission and audit record applies. The firewall is only read.
 */
export function FirewallImport() {
  const confirm = useConfirm()
  const refresh = useInvalidate(...inventoryKeys)
  const [params, setParams] = useSearchParams()
  const [results, setResults] = useState<SavedItem[] | null>(null)
  const canAddLinks = useCan('links:create')
  const canEditLinks = useCan('links:edit')
  const can = { networks: useCan('network:manage'), inventory: useCan('inventory:edit'), links: canAddLinks || canEditLinks }
  const inventory = useMutation({ mutationFn: () => get<FirewallInventory>('/firewall/inventory'), meta: { inlineError: true } })
  const links = useMutation({ mutationFn: () => get<FirewallReview>('/firewall/links/review'), meta: { inlineError: true } })
  const fetchAll = () => {
    setResults(null)
    if (can.networks || can.inventory) inventory.mutate()
    if (can.links) links.mutate()
  }
  const save = useMutation({
    mutationFn: async (tasks: SaveTask[]) => {
      const out: SavedItem[] = []
      for (const [i, t] of tasks.entries()) {
        try { await t.run(); out.push({ key: String(i), label: t.label, ok: true }) } catch (e) { out.push({ key: String(i), label: t.label, ok: false, error: errorMessage(e) }) }
      }
      return out
    },
    meta: { success: (_: SaveTask[], r: SavedItem[]) => `${r.filter((x) => x.ok).length} of ${r.length} saved.` },
    onSuccess: (r) => { setResults(r); void refresh(); inventory.mutate() },
  })
  const onSave = async (tasks: SaveTask[], what: string) => {
    if (tasks.length === 0) return
    if (await confirm({ title: `Save ${tasks.length} ${what}?`, body: 'Each is saved with the normal checks and recorded in the audit log. The firewall is read again afterwards.',
      confirmLabel: 'Save', tone: 'primary', changes: tasks.map((t) => t.label) })) save.mutate(tasks)
  }

  const inv = inventory.data
  const review = links.data
  const counts = openCounts(inv, review)
  const tabs: KindTab[] = [
    ...(can.networks ? [{ value: 'networks', text: 'Networks', count: counts.networks }] : []),
    ...(can.inventory ? [{ value: 'devices', text: 'Devices', count: counts.devices }, { value: 'servers', text: 'Servers', count: counts.servers }] : []),
    ...(can.links ? [{ value: 'links', text: 'Network links', count: counts.links }] : []),
  ]
  const show = tabs.some((t) => t.value === params.get('show')) ? params.get('show')! : tabs[0]?.value
  const busy = inventory.isPending || links.isPending
  const source = inv ?? review

  if (tabs.length === 0) return <Card><EmptyState title="Nothing you may import" body="Importing needs permission to change the register, devices, the network setup or network links." /></Card>
  if (!source) return <StartPanel busy={busy} error={inventory.error ?? links.error} onFetch={fetchAll} />
  const error = show === 'links' ? links.error : inventory.error
  const props = { inv: inv!, busy: save.isPending, onSave: (t: SaveTask[], what: string) => void onSave(t, what) }
  return (
    <div className="space-y-4">
      <SourceBar firewall={source.firewall} at={source.at} busy={busy} onFetch={fetchAll} problems={inv?.problems ?? []} />
      {results && <SavedResults items={results} onClose={() => setResults(null)} />}
      <KindTabs value={show} tabs={tabs} onChange={(v) => setParams({ show: v }, { replace: true })} />
      {error ? <FetchError error={error} /> : null}
      {show === 'links' && review && <LinksReview review={review} onSaved={(r) => { setResults(r); links.mutate() }} />}
      {inv && show === 'networks' && <NetworksReview {...props} />}
      {inv && show === 'devices' && <DevicesReview {...props} />}
      {inv && show === 'servers' && <ServersReview {...props} />}
    </div>
  )
}
