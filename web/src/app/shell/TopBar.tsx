import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useQueryClient } from '@tanstack/react-query'
import { IdCard, LogOut, UserRound } from 'lucide-react'
import { useState } from 'react'
import { signOut } from '../../lib/auth'
import { ProfileModal } from './ProfileModal'

const menuItem = 'flex h-8 cursor-pointer items-center gap-2 rounded-[4px] px-2.5 text-[13px] text-ink-2 outline-none data-[highlighted]:bg-bg data-[highlighted]:text-ink'

export function UserMenu() {
  const queryClient = useQueryClient()
  const [details, setDetails] = useState(false)
  return (
    <>
      {/* Not modal: a modal menu would fight the details dialog for focus as it closes. */}
      <DropdownMenu.Root modal={false}>
        <DropdownMenu.Trigger aria-label="Account menu" title="Account"
          className="grid size-8 cursor-pointer place-items-center rounded-full bg-accent-soft text-accent hover:bg-accent hover:text-white data-[state=open]:bg-accent data-[state=open]:text-white">
          <UserRound size={17} strokeWidth={1.75} aria-hidden />
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content align="end" sideOffset={6}
            className="z-30 w-44 rounded-md border border-line-strong/70 bg-surface p-1 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]">
            <DropdownMenu.Item onSelect={() => setDetails(true)} className={menuItem}>
              <IdCard size={14} aria-hidden /> Details
            </DropdownMenu.Item>
            <DropdownMenu.Item onSelect={async () => { await signOut(); queryClient.clear() }} className={menuItem}>
              <LogOut size={14} aria-hidden /> Sign out
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      <ProfileModal open={details} onClose={() => setDetails(false)} />
    </>
  )
}
