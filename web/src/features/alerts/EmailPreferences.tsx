import { useMutation, useQuery } from '@tanstack/react-query'
import { Mail } from 'lucide-react'
import { useState } from 'react'
import { Button, ErrorState, Modal, Notice, SkeletonRows, Switch } from '../../components/ui'
import { get, put } from '../../lib/api'
import { emailKinds } from '../../lib/labels'
import { useInvalidate } from '../../lib/queries'
import { useMe } from '../../lib/session'

interface Preferences { items: { kind: string; email: boolean }[]; emailEnabled: boolean }

/**
 * Each person chooses which emails they get, among those their role can get.
 * Everything also appears under the bell; email is kept for what needs someone.
 */
export function EmailPreferences() {
  const [open, setOpen] = useState(false)
  const me = useMe()
  const has = (p?: string) => !p || me.permissions.includes(p)
  const refresh = useInvalidate('/notification-preferences')
  const q = useQuery({ queryKey: ['/notification-preferences'], queryFn: () => get<Preferences>('/notification-preferences'), enabled: open })
  const save = useMutation({
    mutationFn: (v: { kind: string; email: boolean }) => put('/notification-preferences', v),
    meta: { success: (v: { kind: string; email: boolean }) => `${emailKinds.find((k) => k.kind === v.kind)?.label ?? 'Email'}: email ${v.email ? 'on' : 'off'}.` },
    onSuccess: () => void refresh(),
  })
  const on = (kind: string) => q.data?.items.find((p) => p.kind === kind)?.email ?? true
  return (
    <>
      <Button onClick={() => setOpen(true)}><Mail size={14} aria-hidden />Email preferences</Button>
      <Modal open={open} onClose={() => setOpen(false)} title="Email preferences"
        subtitle="Everything appears under the bell. Email is kept for what needs you, and gathered so it doesn't pile up."
        footer={<Button onClick={() => setOpen(false)}>Done</Button>}>
        {q.isPending ? <SkeletonRows rows={4} /> : q.isError ? <ErrorState error={q.error} onRetry={() => void q.refetch()} /> : (
          <div className="space-y-4">
            {!q.data.emailEnabled && (
              <Notice tone="warn">Email is switched off for everyone{has('system:manage') ? ' in System › Settings' : ' by an administrator'}. Your choices are kept and apply when it is switched on.</Notice>
            )}
            {emailKinds.filter((k) => has(k.permission)).map((k) => (
              <Switch key={k.kind} id={`pref-${k.kind}`} checked={on(k.kind)} disabled={save.isPending} label={k.label} hint={k.when}
                onChange={(email) => save.mutate({ kind: k.kind, email })} />
            ))}
          </div>
        )}
      </Modal>
    </>
  )
}
