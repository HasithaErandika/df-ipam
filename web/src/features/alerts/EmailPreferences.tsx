import { useMutation, useQuery } from '@tanstack/react-query'
import { Mail } from 'lucide-react'
import { useState } from 'react'
import { Button, ErrorState, Modal, Notice, SkeletonRows, Switch } from '../../components/ui'
import { get, put } from '../../lib/api'
import { label, notificationKinds } from '../../lib/labels'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'

interface Preferences { items: { kind: string; email: boolean }[]; emailEnabled: boolean }

/** Each person chooses which notifications also come by email. */
export function EmailPreferences() {
  const [open, setOpen] = useState(false)
  const isAdmin = useCan('system:manage')
  const refresh = useInvalidate('/notification-preferences')
  const q = useQuery({ queryKey: ['/notification-preferences'], queryFn: () => get<Preferences>('/notification-preferences'), enabled: open })
  const save = useMutation({
    mutationFn: (v: { kind: string; email: boolean }) => put('/notification-preferences', v),
    meta: { success: (v: { kind: string; email: boolean }) => `Email ${v.email ? 'on' : 'off'} for ${label(notificationKinds, v.kind).toLowerCase()}.` },
    onSuccess: () => void refresh(),
  })
  return (
    <>
      <Button onClick={() => setOpen(true)}><Mail size={14} aria-hidden />Email preferences</Button>
      <Modal open={open} onClose={() => setOpen(false)} title="Email preferences" subtitle="Notifications always appear under the bell. Choose which also come by email."
        footer={<Button onClick={() => setOpen(false)}>Done</Button>}>
        {q.isPending ? <SkeletonRows rows={4} /> : q.isError ? <ErrorState error={q.error} onRetry={() => void q.refetch()} /> : (
          <div className="space-y-4">
            {!q.data.emailEnabled && (
              <Notice tone="warn">Email is switched off for everyone{isAdmin ? ' in Settings > System' : ' by an Admin'}. Your choices are kept and apply when it is switched on.</Notice>
            )}
            {q.data.items.filter((p) => isAdmin || !['request', 'alert', 'server_up'].includes(p.kind)).map((p) => (
              <Switch key={p.kind} id={`pref-${p.kind}`} checked={p.email} disabled={save.isPending} label={label(notificationKinds, p.kind)}
                onChange={(email) => save.mutate({ kind: p.kind, email })} />
            ))}
            <p className="text-[12px] text-ink-3">To avoid flooding, at most 20 emails an hour go to one person, and a burst of new alerts arrives as one summary.</p>
          </div>
        )}
      </Modal>
    </>
  )
}
