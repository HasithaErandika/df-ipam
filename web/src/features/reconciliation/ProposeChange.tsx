import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { PersonSelect } from '../../components/SearchSelect'
import { Button, Field, FormError, Input } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import { post } from '../../lib/api'
import type { Finding } from '../../lib/types'
import { issues, optionalMAC, required } from '../../lib/validation'

export const changeableTypes = ['ip_address', 'subnet', 'device']

type FieldKind = 'text' | 'person'

/** What each record type allows. Allocation state isn't here: it moves only through the allocation workflow. */
const fields: Record<string, { key: string; label: string; kind: FieldKind }[]> = {
  ip_address: [{ key: 'ownerId', label: 'Owner', kind: 'person' }, { key: 'hostname', label: 'Hostname', kind: 'text' }, { key: 'notes', label: 'Notes', kind: 'text' }],
  subnet: [{ key: 'technicalOwnerId', label: 'Technical owner', kind: 'person' }, { key: 'businessOwnerId', label: 'Business owner', kind: 'person' }],
  device: [{ key: 'ownerId', label: 'Owner', kind: 'person' }, { key: 'notes', label: 'Notes', kind: 'text' }],
}

export const label = (key: string) => key.replace(/Id$/, '').replace(/([A-Z])/g, ' $1').toLowerCase()

export function ProposeChange({ finding, onDone }: { finding: Finding; onDone: () => void }) {
  const queryClient = useQueryClient()
  const [values, setValues] = useState<Record<string, string>>({})
  const [reason, setReason] = useState('')
  const propose = useMutation({ meta: { success: 'Change proposed. A second person must approve it.', inlineError: true },
    mutationFn: () => post(`/reconciliation/results/${finding.id}/change-requests`, {
      changes: Object.fromEntries(Object.entries(values).filter(([, v]) => v !== '')), reason,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['findings'] })
      queryClient.invalidateQueries({ queryKey: ['changes'] })
      onDone()
    },
  })
  const set = (key: string, value: string) => setValues((v) => ({ ...v, [key]: value }))
  const checks = useChecks(issues({
    changes: Object.values(values).some((v) => v.trim()) ? undefined : 'Propose at least one change.',
    reason: required(reason, 'Reason'),
    mac: optionalMAC(values.mac ?? ''),
  }))
  return (
    <div className="mt-4 rounded-md border border-line p-3">
      <h3 className="mb-2 text-[13px] font-semibold text-ink">Propose a change</h3>
      <div className="grid gap-2 sm:grid-cols-2">
        {fields[finding.objectType].map((f) => (
          <Field key={f.key} label={f.label} htmlFor={`change-${f.key}`}>
            {f.kind === 'person' ? (
              <PersonSelect id={`change-${f.key}`} value={values[f.key] ?? ''} onChange={(v) => set(f.key, v)} emptyLabel="No change" />
            ) : <Input id={`change-${f.key}`} value={values[f.key] ?? ''} onChange={(e) => set(f.key, e.target.value)} />}
          </Field>
        ))}
      </div>
      <div className="mt-2"><Field label="Reason" htmlFor="change-reason" error={checks.shown.reason}><Input id="change-reason" value={reason} onChange={(e) => setReason(e.target.value)} /></Field></div>
      {checks.shown.changes && <p role="alert" className="mt-2 text-[13px] text-danger">{checks.shown.changes}</p>}
      <div className="mt-2"><FormError error={propose.error} /></div>
      <div className="mt-2 flex justify-end"><Button busy={propose.isPending} onClick={() => checks.submit(() => propose.mutate())}>Send for approval</Button></div>
    </div>
  )
}
