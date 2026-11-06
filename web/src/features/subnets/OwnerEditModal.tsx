import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Button, Field, FormError, Input, Modal, useConfirmSave } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import { patch } from '../../lib/api'
import type { Subnet } from '../../lib/types'
import { issues as check, required } from '../../lib/validation'

/** What the owner of a subnet may change. The rest decides how the network is run. */
export function OwnerEditModal({ subnet, onClose, onSaved }: { subnet: Subnet; onClose: () => void; onSaved: (s: Subnet) => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(subnet.name)
  const [description, setDescription] = useState(subnet.description)
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Subnet saved.', inlineError: true },
    mutationFn: () => patch<Subnet>(`/subnets/${subnet.id}`, { name, description }, subnet.version),
    onSuccess: (s) => { queryClient.invalidateQueries({ queryKey: ['subnets'] }); onSaved(s) },
  })
  const checks = useChecks(check({ name: required(name, 'a name') }))
  return (
    <Modal open onClose={onClose} title={`Edit ${subnet.calculated.cidr}`} subtitle="As its owner you can change the name and description."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => confirmSave(subnet.name, () => save.mutate()))}>Save</Button></>}>
      <div className="space-y-3.5">
        <Field label="Name" htmlFor="own-name" error={checks.shown.name}><Input id="own-name" value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Description" htmlFor="own-desc"><Input id="own-desc" value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
        <FormError error={save.error} />
      </div>
    </Modal>
  )
}
