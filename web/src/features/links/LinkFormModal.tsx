import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { Button, EmptyState, ErrorState, Field, FormError, Modal, SkeletonRows, Textarea, useConfirmSave } from '../../components/ui'
import { errorMessage, fieldIssues, post, put, upload } from '../../lib/api'
import { listChanges } from '../../lib/changes'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { LinkDetail, LinkSettings } from '../../lib/types'
import { isVpnType } from './labels'
import { clientIssues, emptyLink, formLabels, fromDetail, type LinkForm, toPayload } from './linkForm'
import { BasicSection, TechnicalSection } from './LinkFormSections'
import { CommercialSection, RelationsSection, VpnSection } from './LinkFormMore'
import { type TabKey, tabLabels, tabOf, tabsWithErrors } from './formTabs'
import { badFiles, type PendingFile, PendingFiles } from './PendingFiles'
import { linkKeys, useLinkOptions, useLinkSettings, useProviders } from './queries'

/**
 * Adds a network link, or edits one when detail is given, in a centred dialog.
 * onSaved gets the link's id.
 */
export function LinkFormModal({ open, detail, onClose, onSaved }: { open: boolean; detail?: LinkDetail; onClose: () => void; onSaved: (id: string) => void }) {
  const settings = useLinkSettings()
  const allowed = useCan(detail ? 'links:edit' : 'links:create')
  const title = detail ? `Edit ${detail.link.code}` : 'Add network link'
  const shell = (body: React.ReactNode) => <Modal open={open} onClose={onClose} size="xl" fixed title={title} footer={<Button onClick={onClose}>Close</Button>}>{body}</Modal>
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
  const [tab, setTab] = useState<TabKey>('basic')
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
    onError: (e) => { const issues = fieldIssues(e); setErrors(issues); const first = Object.keys(issues)[0]; if (first) setTab(tabOf(first)) },
  })
  /** Checks, then saves (asking first for an edit). Returns the fields with problems. */
  const submit = (): string[] => {
    const issues = clientIssues(f)
    if (badFiles(files).length) issues.files = 'Remove the files marked in red: over 15 MB, or a kind that isn’t accepted.'
    setErrors(issues)
    if (Object.keys(issues).length > 0) return Object.keys(issues)
    if (!detail) { save.mutate(); return [] }
    const changes = [...listChanges(start, f, formLabels), ...files.map((x) => `Attach ${x.file.name}`)]
    if (changes.length === 0) onClose()
    else void confirmSave(detail.link.code, () => save.mutate(), 'The change is recorded in the link\'s history.', changes)
    return []
  }
  const p = { f, set, errors }
  const vpn = isVpnType(settings, f.linkType)
  const tabs: TabKey[] = ['basic', 'technical', ...(vpn ? ['vpn' as const] : []), 'related', ...(canManage ? ['commercial' as const] : []), 'notes']
  const bad = tabsWithErrors(errors, tabs)
  const current = tabs.includes(tab) ? tab : 'basic'
  const at = tabs.indexOf(current)
  const trySubmit = () => {
    const issues = submit()
    if (issues.length) setTab(tabOf(issues[0]))
  }
  const panel = (key: TabKey, body: React.ReactNode) => <div role="tabpanel" id={`tab-${key}`} aria-labelledby={`tab-btn-${key}`} hidden={current !== key}>{body}</div>
  return (
    <Modal open onClose={onClose} size="xl" fixed title={title} subtitle={detail ? detail.link.name : 'Fields marked * are required. Support details come from the service provider.'}
      footer={<>
        {bad.length > 0 && !save.error && <p role="alert" className="mr-auto self-center text-[13px] text-danger">Check {bad.map((t) => tabLabels[t]).join(', ')}.</p>}
        <Button onClick={onClose}>Cancel</Button>
        {at > 0 && <Button onClick={() => setTab(tabs[at - 1])}>Back</Button>}
        {at < tabs.length - 1 && <Button onClick={() => setTab(tabs[at + 1])}>Next</Button>}
        <Button variant="primary" busy={save.isPending} onClick={trySubmit}>{detail ? 'Save changes' : 'Add link'}</Button>
      </>}>
      <form noValidate onSubmit={(e) => { e.preventDefault(); trySubmit() }} className="grid gap-5 md:grid-cols-[170px_1fr] md:items-start">
        <div role="tablist" aria-orientation="vertical" aria-label="Sections" className="flex gap-1 overflow-x-auto md:sticky md:top-0 md:flex-col md:border-r md:border-line md:pr-3">
          {tabs.map((t, n) => (
            <button key={t} id={`tab-btn-${t}`} type="button" role="tab" aria-selected={current === t} aria-controls={`tab-${t}`} onClick={() => setTab(t)}
              className={`flex shrink-0 cursor-pointer items-center gap-2 rounded-[5px] px-2.5 py-1.5 text-left text-[13px] ${current === t ? 'bg-accent-soft font-medium text-accent' : 'text-ink-2 hover:bg-surface-2 hover:text-ink'}`}>
              <span className={`grid size-5 shrink-0 place-items-center rounded-full text-[11px] tabular-nums ${current === t ? 'bg-accent text-white' : 'bg-surface-2 text-ink-3'}`}>{n + 1}</span>
              <span className="flex-1">{tabLabels[t]}</span>
              {bad.includes(t) && <span className="size-2 rounded-full bg-danger" aria-label="has problems" />}
            </button>
          ))}
        </div>
        <div className="min-w-0 space-y-4">
          {(providers.isError || options.isError) && <ErrorState error={providers.error ?? options.error} />}
          {panel('basic', <BasicSection p={p} settings={settings} providers={providers.data ?? []} options={options.data} editing={Boolean(detail)} />)}
          {panel('technical', <TechnicalSection p={p} options={options.data} isNew={!detail}
            known={detail ? detail.link.deviceIds.map((id, i) => ({ id, label: detail.link.deviceNames[i] ?? 'Device' })) : []} />)}
          {vpn && panel('vpn', <VpnSection p={p} />)}
          {panel('related', <RelationsSection p={p} options={options.data} selfId={detail?.link.id} />)}
          {canManage && panel('commercial', <CommercialSection p={p} canManage={canManage} canView={canView} />)}
          {panel('notes', <div className="space-y-5">
            <Field label="Remarks and technical notes" htmlFor="link-remarks" error={errors.remarks}>
              <Textarea id="link-remarks" className="min-h-28" value={f.remarks} onChange={(e) => set('remarks', e.target.value)} />
            </Field>
            <PendingFiles files={files} onChange={setFiles} canMoney={canManage} />
            {errors.files && <p role="alert" className="text-[13px] text-danger">{errors.files}</p>}
          </div>)}
          <FormError error={save.error} />
        </div>
        <button type="submit" hidden />
      </form>
    </Modal>
  )
}
