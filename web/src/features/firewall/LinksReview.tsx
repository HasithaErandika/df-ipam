import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { Card, EmptyState, FormError, useConfirm } from '../../components/ui'
import { post } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { FirewallApplyResult, FirewallReview } from '../../lib/types'
import { linkKeys, useLinkSettings, useProviders } from '../links/queries'
import { ListTools, SaveBar, type SavedItem } from './ImportParts'
import { type Choice, missing, ProposalCard, startChoice } from './ProposalCard'
import { linkOpen } from './reviewCounts'

/**
 * WAN interfaces and tunnels as Network Links: tick what to add or update and
 * fill in the business details. The server reads the firewall again before saving.
 */
export function LinksReview({ review, onSaved }: { review: FirewallReview; onSaved: (r: SavedItem[]) => void }) {
  const settings = useLinkSettings()
  const providers = useProviders()
  const confirm = useConfirm()
  const canAdd = useCan('links:create')
  const canEdit = useCan('links:edit')
  const refresh = useInvalidate(...linkKeys)
  const [choices, setChoices] = useState<Record<string, Choice>>({})
  const [onlyOpen, setOnlyOpen] = useState(true)
  const proposals = review.proposals
  useEffect(() => {
    setChoices((old) => Object.fromEntries(proposals.map((p) => [p.key, old[p.key] && linkOpen(p) ? old[p.key] : startChoice(p, settings.data)])))
  }, [proposals, settings.data])
  const apply = useMutation({
    mutationFn: (items: unknown[]) => post<{ results: FirewallApplyResult[] }>('/firewall/links/apply', { items }),
    meta: { inlineError: true, success: (_: unknown[], r: { results: FirewallApplyResult[] }) => `${r.results.filter((x) => x.ok).length} of ${r.results.length} saved.` },
    onSuccess: (r) => {
      void refresh()
      onSaved(r.results.map((x) => ({ key: x.key, ok: x.ok, error: x.error, label: x.ok
        ? <>{x.action === 'add' ? 'Added' : 'Updated'} <Link to={`/network-links/${x.linkId}`} className="text-accent hover:underline">{x.code}</Link> <span className="text-ink-3">({x.key})</span></>
        : x.key })))
    },
  })
  const choice = (key: string) => choices[key]
  const shown = proposals.filter((p) => !onlyOpen || linkOpen(p))
  const picked = proposals.filter((p) => choice(p.key)?.picked)
  const adds = picked.filter((p) => !p.existing)
  const updates = picked.filter((p) => p.existing && choice(p.key).fields.length > 0)
  const blocked = adds.filter((p) => missing(choice(p.key)).length > 0)
  const openShown = shown.filter(linkOpen)
  const chooseAll = (on: boolean) => setChoices((all) => ({ ...all, ...Object.fromEntries(openShown.map((p) => [p.key, { ...all[p.key], picked: on }])) }))
  const note = blocked.length ? `${blocked.length} still need${blocked.length === 1 ? 's' : ''} details`
    : !canAdd && adds.length ? 'You may update links, not add them.' : !canEdit && updates.length ? 'You may add links, not update them.' : undefined
  const save = async () => {
    const lines = [...adds.map((p) => `Add “${choice(p.key).name}” (${p.title})`), ...updates.map((p) => `Update ${p.existing!.code}: ${choice(p.key).fields.length} field(s)`)]
    if (!(await confirm({ title: 'Save these to Network Links?', body: 'The firewall is read again first, so only what it still has is saved. Each change is recorded in the audit log.', confirmLabel: 'Save', tone: 'primary', changes: lines }))) return
    apply.mutate([
      ...adds.map((p) => { const { picked: _p, fields: _f, ...c } = choice(p.key); return { key: p.key, action: 'add', ...c } }),
      ...updates.map((p) => ({ key: p.key, action: 'update', fields: choice(p.key).fields })),
    ])
  }

  if (proposals.length === 0) return <Card><EmptyState title="Nothing to propose" body="The firewall has no interfaces with the WAN role and no IPsec tunnels." /></Card>
  return (
    <Card title="Network links" subtitle="WAN interfaces and VPN tunnels. New ones need a few business details before they can be added.">
      <ListTools all={proposals.length} open={proposals.filter(linkOpen).length} onlyOpen={onlyOpen} setOnlyOpen={setOnlyOpen}
        allChosen={openShown.length > 0 && openShown.every((p) => choice(p.key)?.picked)} chooseAll={chooseAll} />
      {shown.length === 0
        ? <EmptyState title="Everything is recorded" body="Every interface and tunnel matches its network link. Choose All to see them." />
        : <ul className="space-y-2 p-4">
            {shown.map((p) => choice(p.key) && (
              <ProposalCard key={p.key} p={p} c={choice(p.key)} settings={settings.data} providers={providers.data ?? []}
                onChange={(c) => setChoices((all) => ({ ...all, [p.key]: c }))} />
            ))}
          </ul>}
      {apply.error ? <div className="px-4 pb-3"><FormError error={apply.error} /></div> : null}
      <SaveBar count={adds.length + updates.length} noun="links" note={note} busy={apply.isPending}
        disabled={blocked.length > 0 || (!canAdd && adds.length > 0) || (!canEdit && updates.length > 0)}
        onClear={() => setChoices((all) => Object.fromEntries(Object.entries(all).map(([k, c]) => [k, { ...c, picked: false }])))} onSave={() => void save()} />
    </Card>
  )
}
