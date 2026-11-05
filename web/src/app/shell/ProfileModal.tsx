import { useQuery } from '@tanstack/react-query'
import { UserRound } from 'lucide-react'
import type { ReactNode } from 'react'
import { Badge, Button, ErrorState, Modal, SkeletonRows } from '../../components/ui'
import { get } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useMe } from '../../lib/session'
import type { Profile } from '../../lib/types'

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-[12px] text-ink-3">{label}</dt>
      <dd className="mt-0.5 text-[13px] break-words text-ink">{children}</dd>
    </div>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="border-t border-line pt-3.5">
      <h3 className="mb-2 text-[13px] font-semibold text-ink">{title}</h3>
      {children}
    </section>
  )
}

/** Permissions like "inventory:view", grouped under "inventory". */
function groupPermissions(permissions: string[]): [string, string[]][] {
  const groups = new Map<string, string[]>()
  for (const permission of permissions) {
    const [resource, action = permission] = permission.split(':')
    groups.set(resource, [...(groups.get(resource) ?? []), action])
  }
  return [...groups]
}

function Details({ profile, permissions }: { profile: Profile; permissions: string[] }) {
  return (
    <div className="space-y-3.5">
      <div className="flex items-center gap-3">
        <span aria-hidden className="grid size-11 shrink-0 place-items-center rounded-full bg-accent-soft text-accent"><UserRound size={22} /></span>
        <div className="min-w-0">
          <div className="truncate text-[14px] font-semibold text-ink">{profile.displayName || profile.email}</div>
          <div className="truncate text-[13px] text-ink-2">{profile.email}</div>
        </div>
        <span className="ml-auto"><Badge tone={profile.isActive ? 'ok' : 'danger'}>{profile.isActive ? 'Active' : 'Inactive'}</Badge></span>
      </div>
      <Section title="Account">
        <dl className="grid grid-cols-3 gap-x-4 gap-y-3 max-sm:grid-cols-1">
          <Row label="Display name">{profile.displayName || 'Not set'}</Row>
          <Row label="Sign-in provider"><span className="capitalize">{profile.authProvider}</span></Row>
          <Row label="Last sign-in">{profile.lastLoginAt ? formatDateTime(profile.lastLoginAt) : 'Never'}</Row>
          <Row label="Account created">{formatDateTime(profile.createdAt)}</Row>
          <Row label="Last updated">{formatDateTime(profile.updatedAt)}</Row>
          <Row label="User ID"><span className="font-mono text-[12px]">{profile.id}</span></Row>
        </dl>
      </Section>
      <Section title="Roles">
        {profile.assignments.length === 0 ? <p className="text-[13px] text-ink-2">No role assigned yet.</p> : (
          <ul className="space-y-2">
            {profile.assignments.map((a) => (
              <li key={`${a.role}/${a.scope}`} className="text-[13px]">
                <span className="font-medium text-ink">{a.role}</span>
                <span className="text-ink-3"> · {a.scope || 'All scopes'}</span>
                {a.description && <p className="text-[12px] text-ink-2">{a.description}</p>}
              </li>
            ))}
          </ul>
        )}
      </Section>
      <Section title={`Permissions (${permissions.length})`}>
        {permissions.length === 0 ? <p className="text-[13px] text-ink-2">No permissions yet.</p> : (
          <dl className="space-y-1.5">
            {groupPermissions(permissions).map(([resource, actions]) => (
              <div key={resource} className="flex items-baseline gap-3 text-[13px]">
                <dt className="w-28 shrink-0 text-ink-3 capitalize">{resource}</dt>
                <dd className="flex min-w-0 flex-wrap gap-1">{actions.map((action) => <Badge key={action}>{action}</Badge>)}</dd>
              </div>
            ))}
          </dl>
        )}
      </Section>
    </div>
  )
}

export function ProfileModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { permissions } = useMe()
  const query = useQuery({ queryKey: ['me', 'profile'], enabled: open, queryFn: () => get<Profile>('/auth/me/profile') })
  return (
    <Modal open={open} onClose={onClose} title="Account details" size="xl" footer={<Button onClick={onClose}>Close</Button>}>
      {query.isPending && <SkeletonRows rows={5} />}
      {query.isError && <ErrorState error={query.error} onRetry={() => query.refetch()} />}
      {query.data && <Details profile={query.data} permissions={permissions} />}
    </Modal>
  )
}
