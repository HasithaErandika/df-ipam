import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { Button, EmptyState, ErrorState, FormError, Modal, SkeletonRows, Textarea, useConfirmSave } from '../../components/ui'
import { errorMessage, fieldIssues, post, put, upload } from '../../lib/api'
import { listChanges } from '../../lib/changes'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { LinkDetail, LinkSettings } from '../../lib/types'
import { isVpnType } from './labels'
import { clientIssues, emptyLink, formLabels, fromDetail, type LinkForm, toPayload } from './linkForm'
import { BasicSection, TechnicalSection } from './LinkFormSections'
import { CommercialSection, RelationsSection, SupportSection, VpnSection } from './LinkFormMore'
import { maxFileBytes, type PendingFile, PendingFiles } from './PendingFiles'
import { linkKeys, useLinkOptions, useLinkSettings, useProviders } from './queries'

/**
 * Adds a network link, or edits one when detail is given, in a centred dialog.
 * onSaved gets the link's id.
 */
export function LinkFormModal({ open, detail, onClose, onSaved }: { open: boolean; detail?: LinkDetail; onClose: () => void; onSaved: (id: string) => void }) {
  const settings = useLinkSettings()
  const allowed = useCan(detail ? 'links:edit' : 'links:create')
  const title = detail ? `Edit ${detail.link.code}` : 'Add network link'
  const shell = (body: React.ReactNode) => <Modal open={open} onClose={onClose} size="xl" title={title} footer={<Button onClick={onClose}>Close</Button>}>{body}</Modal>
  if (!open) return null
  if (!allowed) return shell(<EmptyState title="You can't do this" body={detail ? 'Editing links needs the Edit network links permission.' : 'Adding links needs the Add network links permission.'} />)
  if (settings.isPending) return shell(<SkeletonRows rows={8} />)
  if (settings.isError) return shell(<ErrorState error={settings.error} onRetry={() => void settings.refetch()} />)
  if (detail?.link.archivedAt) return shell(<EmptyState title="This link is archived" body="Restore it before editing." />)
  return <FormDialog key={detail?.link.id ?? 'new'} title={title} detail={detail} settings={settings.data} onClose={onClose} onSaved={onSaved} />
}

function FormDialog({ title, detail, settings, onClose, onSaved }: { title: string; detail?: LinkDetail; settings: LinkSettings; onClose: () => void; onSaved: (id: string) => void }) {
  const providers = useProviders()
  const options = useLinkOptions()
  const canManage = useCan('links:commercial_manage')
  const canView = useCan('links:commercial_view')
  const confirmSave = useConfirmSave()
  const refresh = useInvalidate(...linkKeys)
  const [start] = useState<LinkForm>(() => (detail ? fromDetail(detail) : emptyLink(settings.sites.length === 1 ? settings.sites[0] : '')))
  const [f, setF] = useState<LinkForm>(start)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [files, setFiles] = useState<PendingFile[]>([])
  const set = <K extends keyof LinkForm>(key: K, value: LinkForm[K]) => setF((x) => ({ ...x, [key]: value }))
  // The link is saved first; then each file is uploaded to it. A file that fails
  // is named in the message and can be added again on the link's page.
  const saveAll = async () => {
    const { id } = detail
      ? await put(`/network-links/${detail.link.id}`, toPayload(f)).then(() => ({ id: detail.link.id }))
      : await post<{ id: string }>('/network-links', toPayload(f))
    const failed: string[] = []
    for (const x of files) {
      try { await upload(`/network-links/${id}/attachments`, x.file, { category: x.category }) } catch (e) { failed.push(`${x.file.name} (${errorMessage(e)})`) }
    }
    return { id, failed }
  }
  const saved = detail ? 'Link saved' : 'Link added'
  const save = useMutation({
    mutationFn: saveAll,
    meta: { inlineError: true, success: (_: void, r: { failed: string[]; id: string }) => (r.failed.length
      ? `${saved}, but these files weren't attached: ${r.failed.join('; ')}. Add them on the link's page.`
      : files.length ? `${saved} with ${files.length} file${files.length === 1 ? '' : 's'}.` : `${saved}.`) },
    onSuccess: (r) => { void refresh(); onSaved(r.id) },
    onError: (e) => setErrors(fieldIssues(e)),
  })
  const submit = () => {
    const issues = clientIssues(f)
    if (files.some((x) => x.file.size > maxFileBytes)) issues.files = 'Remove files over 15 MB.'
    setErrors(issues)
    if (Object.keys(issues).length > 0) return
    if (!detail) { save.mutate(); return }
    const changes = [...listChanges(start, f, formLabels), ...files.map((x) => `Attach ${x.file.name}`)]
    if (changes.length === 0) { onClose(); return }
    void confirmSave(detail.link.code, () => save.mutate(), 'The change is recorded in the link\'s history.', changes)
  }
  const p = { f, set, errors }
  return (
    <Modal open onClose={onClose} size="xl" title={title} subtitle="Fields marked * are required. The VPN section appears for VPN link types."
      footer={<>
        {Object.keys(errors).length > 0 && !save.error && <p role="alert" className="mr-auto self-center text-[13px] text-danger">Some fields need attention.</p>}
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="primary" busy={save.isPending} onClick={submit}>{detail ? 'Save changes' : 'Add link'}</Button>
      </>}>
      <form noValidate onSubmit={(e) => { e.preventDefault(); submit() }} className="space-y-4">
        {(providers.isError || options.isError) && <ErrorState error={providers.error ?? options.error} />}
        <BasicSection p={p} settings={settings} providers={providers.data ?? []} options={options.data} editing={Boolean(detail)} />
        <TechnicalSection p={p} options={options.data} />
        {isVpnType(settings, f.linkType) && <VpnSection p={p} options={options.data} />}
        <RelationsSection p={p} options={options.data} selfId={detail?.link.id} />
        <CommercialSection p={p} canManage={canManage} canView={canView} />
        <SupportSection p={p} providers={providers.data ?? []} provider={providers.data?.find((x) => x.id === f.providerId)} />
        <section className="rounded-md border border-line bg-surface">
          <header className="border-b border-line px-4 py-2.5"><h2 className="text-[13px] font-semibold text-ink">Remarks and technical notes</h2></header>
          <div className="p-4">
            <Textarea aria-label="Remarks" value={f.remarks} onChange={(e) => set('remarks', e.target.value)} />
            <p className="mt-1 text-[12.5px] text-ink-3">Add contracts, quotations, diagrams and configuration files on the link's page once it is saved.</p>
          </div>
        </section>
        <PendingFiles files={files} onChange={setFiles} canMoney={canManage} />
        {errors.files && <p role="alert" className="text-[13px] text-danger">{errors.files}</p>}
        <FormError error={save.error} />
        <button type="submit" hidden />
      </form>
    </Modal>
  )
}
