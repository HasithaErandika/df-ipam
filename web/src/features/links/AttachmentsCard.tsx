import { useMutation } from '@tanstack/react-query'
import { Download, Paperclip, Trash2, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { Button, Card, Field, FormError, Modal, Select, useConfirm } from '../../components/ui'
import { del, download, upload } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { LinkDetail } from '../../lib/types'
import { commercialFiles, fileCategories, formatColombo, formatSize } from './labels'

const accept = '.pdf,.png,.jpg,.jpeg,.docx,.xlsx,.pptx,.vsdx,.txt,.csv,.cfg,.conf'

/** The link's documents: contracts, quotations, diagrams and configurations without secrets. */
export function AttachmentsCard({ d }: { d: LinkDetail }) {
  const canEdit = useCan('links:edit') && !d.link.archivedAt
  const canMoney = useCan('links:commercial_manage')
  const confirm = useConfirm()
  const refresh = useInvalidate(`/network-links/${d.link.id}`)
  const [adding, setAdding] = useState(false)
  const remove = useMutation({
    mutationFn: (id: string) => del(`/network-links/${d.link.id}/attachments/${id}`),
    meta: { success: 'Attachment removed. It stays in the link\'s history.' },
    onSuccess: () => void refresh(),
  })
  const current = d.attachments.filter((a) => !a.removedAt)
  const removed = d.attachments.filter((a) => a.removedAt)
  const ask = async (id: string, name: string) => {
    if (await confirm({ title: `Remove ${name}?`, body: 'It is taken off this link. The file and the record of it are kept for the history.', confirmLabel: 'Remove' })) remove.mutate(id)
  }
  return (
    <Card title="Attachments" subtitle="Up to 15 MB each: PDF, images, Office, Visio and text files. Never attach files that contain passwords or keys."
      action={canEdit ? <Button onClick={() => setAdding(true)}><Upload size={14} aria-hidden />Add file</Button> : undefined}>
      <div className="p-4">
        {current.length === 0 ? <p className="text-[13px] text-ink-3">No files yet.</p> : (
          <ul className="divide-y divide-line">
            {current.map((a) => (
              <li key={a.id} className="flex flex-wrap items-center justify-between gap-2 py-2 text-[13px]">
                <span className="flex min-w-0 items-center gap-2"><Paperclip size={14} aria-hidden className="shrink-0 text-ink-3" />
                  <span className="min-w-0"><span className="block truncate font-medium text-ink">{a.fileName}</span>
                    <span className="block text-[12px] text-ink-3">{fileCategories[a.category]} · {formatSize(a.size)} · {a.uploadedBy ?? 'Unknown'}, {formatColombo(a.uploadedAt)}</span></span>
                </span>
                <span className="flex gap-1">
                  <Button variant="ghost" onClick={() => void download(`/network-links/${d.link.id}/attachments/${a.id}`, a.fileName)}><Download size={14} aria-hidden />Download</Button>
                  {canEdit && (!commercialFiles.includes(a.category) || canMoney) && <Button variant="ghost" className="text-danger" aria-label={`Remove ${a.fileName}`} onClick={() => void ask(a.id, a.fileName)}><Trash2 size={14} /></Button>}
                </span>
              </li>
            ))}
          </ul>
        )}
        {removed.length > 0 && <p className="mt-2 text-[12px] text-ink-3">Removed earlier: {removed.map((a) => a.fileName).join(', ')}.</p>}
      </div>
      <AddFile linkId={d.link.id} open={adding} onClose={() => setAdding(false)} canMoney={canMoney} />
    </Card>
  )
}

function AddFile({ linkId, open, onClose, canMoney }: { linkId: string; open: boolean; onClose: () => void; canMoney: boolean }) {
  const [file, setFile] = useState<File | null>(null)
  const [category, setCategory] = useState('diagram')
  const input = useRef<HTMLInputElement>(null)
  const refresh = useInvalidate(`/network-links/${linkId}`)
  const close = () => { setFile(null); send.reset(); onClose() }
  const send = useMutation({
    mutationFn: () => upload(`/network-links/${linkId}/attachments`, file!, { category }),
    meta: { inlineError: true, success: 'File attached.' },
    onSuccess: () => { void refresh(); close() },
  })
  const tooBig = file && file.size > 15 * 1024 * 1024
  const categories = Object.entries(fileCategories).filter(([k]) => canMoney || !commercialFiles.includes(k))
  return (
    <Modal open={open} onClose={close} title="Add a file"
      footer={<><Button onClick={close}>Cancel</Button><Button variant="primary" disabled={!file || Boolean(tooBig)} busy={send.isPending} onClick={() => send.mutate()}>Attach</Button></>}>
      <div className="space-y-4">
        <Field label="Kind of document" htmlFor="att-kind">
          <Select id="att-kind" value={category} onChange={(e) => setCategory(e.target.value)}>{categories.map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
        </Field>
        <Field label="File" htmlFor="att-file" error={tooBig ? 'Files can be at most 15 MB.' : undefined} hint="Remove passwords, keys and other secrets from configuration files first.">
          <input id="att-file" ref={input} type="file" accept={accept} className="block w-full text-[13px] text-ink-2 file:mr-3 file:cursor-pointer file:rounded-[5px] file:border file:border-line-strong file:bg-surface file:px-3 file:py-1.5 file:text-[13px] file:text-ink"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        </Field>
        <FormError error={send.error} />
      </div>
    </Modal>
  )
}
