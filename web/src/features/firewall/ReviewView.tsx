import { useMutation } from '@tanstack/react-query'
import { CheckCircle2, RefreshCw, XCircle } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { Button, Card, EmptyState, FormError, Notice, useConfirm } from '../../components/ui'
import { get, post } from '../../lib/api'
import { formatAgo } from '../../lib/format'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { FirewallApplyResult, FirewallReview } from '../../lib/types'
import { linkKeys, useLinkSettings, useProviders } from '../links/queries'
import { notConnected } from './LiveView'
import { type Choice, missing, ProposalCard, startChoice } from './ProposalCard'

/**
 * Fetch, review, confirm. The firewall's WAN interfaces and tunnels are shown
 * as proposals; nothing is saved until people tick what to add or update and
 * confirm. The server reads the firewall again before saving.
 */
export function ReviewView() {
  const settings = useLinkSettings()
  const providers = useProviders()
  const confirm = useConfirm()
  const canAdd = useCan('links:create')
  const canEdit = useCan('links:edit')
  const refresh = useInvalidate(...linkKeys)
  const [choices, setChoices] = useState<Record<string, Choice>>({})
  const fetch = useMutation({
    mutationFn: () => get<FirewallReview>('/firewall/links/review'),
    meta: { inlineError: true },
    onSuccess: (r) => { setChoices(Object.fromEntries(r.proposals.map((p) => [p.key, startChoice(p, settings.data)]))); apply.reset() },
  })
  const apply = useMutation({
    mutationFn: (items: unknown[]) => post<{ results: FirewallApplyResult[] }>('/firewall/links/apply', { items }),
    meta: { inlineError: true, success: (_: unknown[], r: { results: FirewallApplyResult[] }) => `${r.results.filter((x) => x.ok).length} of ${r.results.length} saved.` },
    onSuccess: () => { void refresh(); fetch.reset(); setChoices({}) },
  })
  const review = fetch.data
  const proposals = review?.proposals ?? []
  const picked = proposals.filter((p) => choices[p.key]?.picked)
  const adds = picked.filter((p) => !p.existing)
  const updates = picked.filter((p) => p.existing && choices[p.key].fields.length > 0)
  const blocked = adds.filter((p) => missing(choices[p.key]).length > 0)
  const confirmApply = async () => {
    const lines = [...adds.map((p) => `Add “${choices[p.key].name}” (${p.title})`), ...updates.map((p) => `Update ${p.existing!.code}: ${choices[p.key].fields.length} field(s)`)]
    if (!(await confirm({ title: 'Save these to Network Links?', body: 'The firewall is read again first, so only what it still has is saved. Each change is recorded in the audit log.', confirmLabel: 'Save', tone: 'primary', changes: lines }))) return
    apply.mutate([
      ...adds.map((p) => { const { picked: _p, fields: _f, ...c } = choices[p.key]; return { key: p.key, action: 'add', ...c } }),
      ...updates.map((p) => ({ key: p.key, action: 'update', fields: choices[p.key].fields })),
    ])
  }

  if (!canAdd && !canEdit) return <Card><EmptyState title="You can't add or edit links" body="Reviewing the firewall needs the Add or Edit network links permission." /></Card>
  return (
    <div className="space-y-4">
      <Card title="Bring links in from the firewall" subtitle="1. Fetch what the firewall has. 2. Tick what to add or update, and fill in the business details. 3. Confirm.">
        <div className="flex flex-wrap items-center gap-3 p-4">
          <Button variant="primary" busy={fetch.isPending} onClick={() => fetch.mutate()}><RefreshCw size={14} aria-hidden />{review ? 'Fetch again' : 'Fetch from firewall'}</Button>
          {review && <span className="text-[13px] text-ink-2">{review.firewall}, fetched {formatAgo(review.at)}: {proposals.length} WAN interface{proposals.length === 1 ? '' : 's'} and tunnel{proposals.length === 1 ? '' : 's'}.</span>}
          {!review && <span className="text-[13px] text-ink-3">Only reads the firewall. Nothing is saved until you confirm.</span>}
        </div>
        {fetch.isError && <div className="px-4 pb-4">{notConnected(fetch.error) ? <Notice tone="warn">No firewall is connected. An administrator connects it in System &gt; FortiGate.</Notice> : <FormError error={fetch.error} />}</div>}
      </Card>
      {apply.data && <Results results={apply.data.results} />}
      {review && proposals.length === 0 && <Card><EmptyState title="Nothing to propose" body="The firewall has no interfaces with the WAN role and no IPsec tunnels." /></Card>}
      {proposals.length > 0 && (
        <>
          <ul className="space-y-2">
            {proposals.map((p) => (
              <ProposalCard key={p.key} p={p} c={choices[p.key]} settings={settings.data} providers={providers.data ?? []}
                onChange={(c) => setChoices((all) => ({ ...all, [p.key]: c }))} />
            ))}
          </ul>
          <div className="sticky bottom-0 flex flex-wrap items-center justify-end gap-3 rounded-md border border-line bg-surface px-4 py-3 shadow-[0_-8px_24px_-16px_rgba(11,18,32,.3)]">
            <span className="mr-auto text-[13px] text-ink-2">{adds.length} to add, {updates.length} to update{blocked.length ? ` · ${blocked.length} still need details` : ''}</span>
            {(!canAdd && adds.length > 0) && <span className="text-[12.5px] text-danger">You may update links, not add them.</span>}
            <FormError error={apply.error} />
            <Button variant="primary" busy={apply.isPending} disabled={adds.length + updates.length === 0 || blocked.length > 0 || (!canAdd && adds.length > 0) || (!canEdit && updates.length > 0)} onClick={() => void confirmApply()}>
              Review and save
            </Button>
          </div>
        </>
      )}
    </div>
  )
}

function Results({ results }: { results: FirewallApplyResult[] }) {
  return (
    <Card title="Saved">
      <ul className="divide-y divide-line">
        {results.map((r) => (
          <li key={r.key} className="flex flex-wrap items-center gap-2 px-4 py-2 text-[13px]">
            {r.ok ? <CheckCircle2 size={15} className="text-ok" aria-hidden /> : <XCircle size={15} className="text-danger" aria-hidden />}
            <span className="font-mono text-[12px] text-ink-3">{r.key}</span>
            {r.ok ? <>{r.action === 'add' ? 'Added as' : 'Updated'} <Link to={`/network-links/${r.linkId}`} className="text-accent hover:underline">{r.code}</Link></> : <span className="text-danger">{r.error}</span>}
          </li>
        ))}
      </ul>
    </Card>
  )
}
