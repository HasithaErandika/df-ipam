import { useQuery } from '@tanstack/react-query'
import { Card, ErrorState, SkeletonRows } from '../../components/ui'
import { get } from '../../lib/api'
import type { LinkHistoryEntry } from '../../lib/types'
import { formatColombo } from './labels'

const actions: Record<string, string> = {
  create: 'Added', update: 'Changed', renew: 'Contract renewed', archive: 'Archived', restore: 'Restored', attach: 'File attached', detach: 'File removed',
}

/** Field names as people read them. Stored names are snake_case, the rest as sent. */
const fieldNames: Record<string, string> = {
  link_type: 'Link type', provider_id: 'Provider', circuit_id: 'Circuit ID', source_site: 'Source location', link_role: 'Link role',
  officer_id: 'Responsible officer', down_mbps: 'Download (Mbps)', up_mbps: 'Upload (Mbps)', wan_ip: 'WAN IP', wan_segment_id: 'WAN subnet',
  public_ips: 'Public IPs', gateway_ip: 'Gateway IP', device_id: 'Connected device', device_text: 'Connected device', vpn_tunnel: 'VPN tunnel',
  vpn_type: 'VPN type', local_peer: 'Local peer', remote_peer: 'Remote peer', remote_subnets: 'Remote subnets', remote_org: 'Remote organisation',
  remote_contact: 'Remote contact', last_test_date: 'Last test', vault_ref: 'Vault reference', no_separate_cost: 'No separate cost',
  billing_account: 'Billing account', monthly_rental: 'Monthly rental', install_cost: 'Installation cost', contract_ref: 'Contract reference',
  po_ref: 'PO reference', activation_date: 'Activation date', contract_start: 'Contract start', contract_end: 'Contract expiry', contractEnd: 'Contract expiry',
  renewal_notice_days: 'Renewal notice', cost_centre: 'Cost centre', support_contact: 'Support contact', support_email: 'Support email',
  support_hotline: 'Hotline', account_manager: 'Account manager', sla_pct: 'SLA %', resolution_hours: 'Resolution hours', archived_at: 'Archived',
  local_subnets: 'Local subnets', relations: 'Related links',
}

const label = (f: string) => fieldNames[f] ?? f.charAt(0).toUpperCase() + f.slice(1).replace(/_/g, ' ')

function value(v: unknown): string {
  if (v === null || v === undefined || v === '') return 'none'
  if (Array.isArray(v)) return v.length ? v.map((x) => (typeof x === 'object' ? Object.values(x as object).join(' ') : String(x))).join(', ') : 'none'
  if (typeof v === 'object') return Object.values(v as object).join(' ')
  if (typeof v === 'boolean') return v ? 'yes' : 'no'
  return String(v)
}

/** Who changed the link, when and what, from the audit log. */
export function HistoryCard({ id }: { id: string }) {
  const q = useQuery({ queryKey: [`/network-links/${id}`, 'history'], queryFn: () => get<LinkHistoryEntry[]>(`/network-links/${id}/history`) })
  return (
    <Card title="Change history" subtitle="From the audit log. Times are Sri Lanka time.">
      <div className="p-4">
        {q.isPending ? <SkeletonRows rows={4} /> : q.isError ? <ErrorState error={q.error} onRetry={() => void q.refetch()} /> : q.data.length === 0 ? <p className="text-[13px] text-ink-3">No changes recorded.</p> : (
          <ol className="space-y-3">
            {q.data.map((e, i) => (
              <li key={i} className="border-l-2 border-line pl-3">
                <div className="text-[13px]"><span className="font-medium text-ink">{actions[e.action] ?? e.action}</span> <span className="text-ink-3">by {e.actor}, {formatColombo(e.at)}</span></div>
                {e.action !== 'create' && e.changes.length > 0 && (
                  <ul className="mt-1 space-y-0.5 text-[12.5px] text-ink-2">
                    {e.changes.map((c) => <li key={c.field}>{label(c.field)}: <span className="text-ink-3">{value(c.old)}</span> → {value(c.new)}</li>)}
                  </ul>
                )}
              </li>
            ))}
          </ol>
        )}
      </div>
    </Card>
  )
}
