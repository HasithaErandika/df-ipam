import { useMutation } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { Button, Field, Input, Select } from '../../components/ui'
import { errorMessage, fieldIssues, put } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { LinkSettings } from '../../lib/types'
import type { SectionProps } from './formParts'

/**
 * The link's source location: Head Office, a branch, the BCP site. People who
 * set up the network can add a location here; it joins the list in
 * Network setup > Network links.
 */
export function SiteField({ p, settings }: { p: SectionProps; settings?: LinkSettings }) {
  const { f, set, errors } = p
  const canAdd = useCan('network:manage')
  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const [issue, setIssue] = useState('')
  const refresh = useInvalidate('/link-settings')
  const listed = settings?.sites ?? []
  // A location removed from the list stays on the link that already has it.
  const sites = [...listed, ...(f.sourceSite && !listed.includes(f.sourceSite) ? [f.sourceSite] : [])]
  const add = useMutation({
    mutationFn: (site: string) => put('/link-settings', { ...settings, sites: [...listed, site] }),
    meta: { inlineError: true, success: 'Location added.' },
    onSuccess: (_: unknown, site: string) => { void refresh(); set('sourceSite', site); setAdding(false); setName('') },
    onError: (e) => setIssue(fieldIssues(e).sites ?? errorMessage(e)),
  })
  const submit = () => {
    const site = name.trim()
    const existing = listed.find((x) => x.toLowerCase() === site.toLowerCase())
    if (!site) setIssue('Enter the location’s name.')
    else if (site.length > 120) setIssue('A location can be at most 120 characters.')
    else if (existing) { set('sourceSite', existing); setAdding(false); setName('') }
    else if (settings) { setIssue(''); add.mutate(site) }
  }
  const hint = canAdd ? 'Where the link starts. Add a location if it isn’t listed.' : 'Where the link starts. Administrators add locations in Network setup > Network links.'
  return (
    <div className="space-y-2">
      <Field label="Source location *" htmlFor="link-sourceSite" error={errors.sourceSite} hint={adding ? undefined : hint}>
        <div className="flex gap-2">
          <Select id="link-sourceSite" className="min-w-0 flex-1" value={f.sourceSite} onChange={(e) => set('sourceSite', e.target.value)}>
            <option value="">Choose…</option>
            {sites.map((x) => <option key={x} value={x}>{x}</option>)}
          </Select>
          {canAdd && !adding && <Button aria-label="New location" title="New location" onClick={() => { setAdding(true); setIssue('') }}><Plus size={14} aria-hidden /></Button>}
        </div>
      </Field>
      {adding && (
        <Field label="New location" htmlFor="link-newSite" error={issue}>
          <div className="flex gap-2">
            <Input id="link-newSite" autoFocus className="min-w-0 flex-1" placeholder="BCP - Nugegoda" value={name} onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); submit() } if (e.key === 'Escape') { e.stopPropagation(); setAdding(false) } }} />
            <Button variant="primary" busy={add.isPending} onClick={submit}>Add</Button>
            <Button onClick={() => setAdding(false)}>Cancel</Button>
          </div>
        </Field>
      )}
    </div>
  )
}
