import { Link } from 'react-router'
import { none, Stack, StateBadge, type Column } from '../../components/ui'
import type { LinkRenewal } from '../../lib/types'
import { daysText, expiryTone, formatDay, formatMoney } from './labels'

/** Columns for contracts ending soon, on the renewals page and the overview. */
export const renewalColumns: Column<LinkRenewal>[] = [
  { key: 'link', header: 'Link', cell: (r) => <Stack main={<Link to={`/network-links/${r.id}`} className="font-medium text-ink hover:text-accent hover:underline">{r.name}</Link>} sub={<span className="font-mono">{r.code}</span>} /> },
  { key: 'provider', header: 'Provider', hideBelow: 'md', cell: (r) => r.provider ?? none },
  { key: 'end', header: 'Contract expiry', cell: (r) => <Stack main={formatDay(r.contractEnd)} sub={<StateBadge tone={expiryTone(r.daysLeft)}>{daysText(r.daysLeft)}</StateBadge>} /> },
  { key: 'notice', header: 'Notice period', hideBelow: 'lg', cell: (r) => (r.noticeDays != null
    ? <span className={r.daysLeft <= r.noticeDays ? 'text-danger' : undefined}>{r.noticeDays} days{r.daysLeft <= r.noticeDays && r.daysLeft >= 0 ? ' (inside it now)' : ''}</span>
    : none) },
  { key: 'rental', header: 'Monthly rental', hideBelow: 'lg', className: 'text-right tabular-nums', cell: (r) => (r.monthlyRental == null ? none : formatMoney(r.monthlyRental, r.currency)) },
  { key: 'officer', header: 'Responsible officer', hideBelow: 'xl', cell: (r) => r.officer ?? <span className="text-warn">Nobody</span> },
  { key: 'ref', header: 'Contract reference', hideBelow: 'xl', cell: (r) => r.contractRef || none },
]
