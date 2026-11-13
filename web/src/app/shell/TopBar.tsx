import { useQueryClient } from '@tanstack/react-query'
import { LogOut } from 'lucide-react'
import { useState } from 'react'
import { signOut } from '../../lib/auth'
import { initialsFromEmail, nameFromEmail } from '../../lib/format'
import { useMe } from '../../lib/session'
import { ProfileModal } from './ProfileModal'

/** Who is signed in, opening their details, and a button to sign out. */
export function UserMenu() {
  const me = useMe()
  const queryClient = useQueryClient()
  const [details, setDetails] = useState(false)
  const [leaving, setLeaving] = useState(false)
  const leave = async () => {
    setLeaving(true)
    try {
      await signOut()
      queryClient.clear()
    } finally {
      setLeaving(false)
    }
  }
  return (
    <>
      <button type="button" onClick={() => setDetails(true)} title={`Your details · ${me.email}`} aria-label={`Your details: ${nameFromEmail(me.email)}, ${me.email}`}
        className="flex cursor-pointer items-center gap-2.5 rounded-full py-1 pr-3 pl-1 hover:bg-surface-2 max-md:pr-1">
        <span aria-hidden className="grid size-8 shrink-0 place-items-center rounded-full bg-accent text-[12px] font-semibold text-white">{initialsFromEmail(me.email)}</span>
        <span className="min-w-0 text-left leading-tight max-md:hidden">
          <span className="block max-w-[220px] truncate text-[12.5px] font-medium text-ink">{nameFromEmail(me.email)}</span>
          <span className="block max-w-[220px] truncate text-[11.5px] text-ink-3">{me.roleName || 'No role yet'}</span>
        </span>
      </button>
      <button type="button" onClick={() => void leave()} disabled={leaving} title="Sign out" aria-label="Sign out"
        className="flex h-8 cursor-pointer items-center gap-1.5 rounded-md border border-line-strong px-2.5 text-[13px] font-medium text-ink-2 hover:border-danger/40 hover:bg-danger-soft hover:text-danger disabled:opacity-55 max-sm:px-2">
        <LogOut size={15} aria-hidden /><span className="max-sm:hidden">Sign out</span>
      </button>
      <ProfileModal open={details} onClose={() => setDetails(false)} />
    </>
  )
}
