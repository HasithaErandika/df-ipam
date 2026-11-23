import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { Button, Field, FormError, Input, Modal, Select } from '../../components/ui'
import { fieldIssues, post } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'

const kinds = [
  { value: 'cloud', label: 'Cloud / WAN: never swept (recommended for WAN, provider and VPN addresses)' },
  { value: 'server', label: 'Server network: swept' },
  { value: 'dhcp', label: 'DHCP network (floor or Wi-Fi): swept' },
]

/**
 * Adds the segment for a link's WAN subnet without leaving the link dialog.
 * The network comes from the address as typed ("10.10.40.253 /24" gives
 * 10.10.40.0/24; no prefix gives a /32). It defaults to the Cloud type, which
 * is never swept: WAN hand-offs and other organisations' subnets shouldn't be
 * scanned. The segment is saved and audited like one added in Network setup.
 */
export function NewSegmentDialog({ network, suggestedName, onClose, onCreated }:
  { network: string; suggestedName: string; onClose: () => void; onCreated: (id: string, name: string) => void }) {
  const [name, setName] = useState(suggestedName.slice(0, 80))
  const [kind, setKind] = useState('cloud')
  const [description, setDescription] = useState('Added from the network link form.')
  const [issues, setIssues] = useState<Record<string, string>>({})
  const [failure, setFailure] = useState<unknown>(null)
  const refresh = useInvalidate('/network-links', '/segments')
  const save = useMutation({
    mutationFn: () => post<{ id: string }>('/segments', { name: name.trim(), kind, network, description }),
    meta: { inlineError: true, success: `Segment ${network} added.` },
    onSuccess: async (res) => { await refresh(); onCreated(res.id, name.trim()) },
    onError: (e) => { const f = fieldIssues(e); setIssues(f); setFailure(Object.keys(f).length ? null : e) },
  })
  const submit = () => {
    if (!name.trim()) { setIssues({ name: 'Enter a name for the segment.' }); return }
    save.mutate()
  }
  return (
    <Modal open onClose={onClose} title="New segment for the WAN subnet" subtitle={<span className="font-mono">{network}</span>} size="md"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={submit}>Add segment</Button></>}>
      <div className="space-y-3">
        <FormError error={failure} />
        {issues.network && <p role="alert" className="rounded-md bg-danger-soft px-3 py-2 text-[13px] text-danger">{issues.network}</p>}
        <Field label="Name *" htmlFor="seg-name" error={issues.name}>
          <Input id="seg-name" autoFocus maxLength={80} value={name} onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); submit() } }} />
        </Field>
        <Field label="Type" htmlFor="seg-kind" error={issues.kind} hint="Discovery sweeps server and DHCP networks, and only private ranges.">
          <Select id="seg-kind" value={kind} onChange={(e) => setKind(e.target.value)}>
            {kinds.map((k) => <option key={k.value} value={k.value}>{k.label}</option>)}
          </Select>
        </Field>
        <Field label="Description" htmlFor="seg-description" error={issues.description}>
          <Input id="seg-description" maxLength={500} value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
      </div>
    </Modal>
  )
}
