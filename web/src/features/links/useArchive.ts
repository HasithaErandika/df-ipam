import { useMutation } from '@tanstack/react-query'
import { useConfirm } from '../../components/ui'
import { post } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import { linkKeys } from './queries'

/** Archive or restore a link, after a confirmation. Links are never deleted. */
export function useArchive() {
  const confirm = useConfirm()
  const refresh = useInvalidate(...linkKeys)
  const archive = useMutation({
    mutationFn: ({ id, restore }: { id: string; restore: boolean }) => post(`/network-links/${id}/${restore ? 'restore' : 'archive'}`),
    meta: { success: (v: { restore: boolean }) => (v.restore ? 'Link restored.' : 'Link archived.') },
    onSuccess: () => void refresh(),
  })
  return async (link: { id: string; code: string; name: string; archivedAt: string | null }) => {
    const restore = Boolean(link.archivedAt)
    const ok = await confirm(restore
      ? { title: `Restore ${link.code}?`, body: `${link.name} returns to the lists, totals and contract reminders.`, confirmLabel: 'Restore link', tone: 'primary' }
      : { title: `Archive ${link.code}?`, body: `${link.name} leaves the lists, totals and contract reminders. Its details, files and history are kept, and it can be restored.`, confirmLabel: 'Archive link' })
    if (ok) archive.mutate({ id: link.id, restore })
  }
}
