import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { Button, Field, FormError, Input, Modal, Mono, MoreOptions, Select } from '../../components/ui'
import { Detail, SubnetPicker } from '../../components/workflow'
import { useChecks } from '../../components/workflow'
import { post, fieldIssues } from '../../lib/api'
import { environments, formatDateTime, isoDate } from '../../lib/format'
import type { Allocation, IPRequest, RequestType, Subnet } from '../../lib/types'
import { ip, issues as check, required } from '../../lib/validation'
import { useRefresh } from './useRefresh'

export interface Preset { requestType: RequestType; allocation?: Allocation; reservationId?: string; reservationAddress?: string }

export function NewRequestModal({ open, preset, onClose }: { open: boolean; preset?: Preset; onClose: () => void }) {
  const refresh = useRefresh()
  const navigate = useNavigate()
  const [requestType, setRequestType] = useState<RequestType>(preset?.requestType ?? 'allocation')
  const [subnet, setSubnet] = useState<Subnet | null>(null)
  const [mode, setMode] = useState<'automatic' | 'specific'>('automatic')
  const [address, setAddress] = useState('')
  const [justification, setJustification] = useState('')
  const [department, setDepartment] = useState('')
  const [environment, setEnvironment] = useState('')
  const [requiredDate, setRequiredDate] = useState('')
  const [expiryDate, setExpiryDate] = useState(isoDate(30))
  const [comments, setComments] = useState('')
  const release = requestType === 'release'
  const conversion = Boolean(preset?.reservationId)

  const save = useMutation({ meta: { success: (submit: boolean) => submit ? 'Request submitted for approval.' : 'Draft saved.', inlineError: true },
    mutationFn: (submit: boolean) => post<IPRequest>('/requests', {
      requestType, submit, justification, department, comments,
      subnetId: subnet?.id ?? '', selectionMode: mode, requestedAddress: mode === 'specific' ? address : null,
      allocationId: preset?.allocation?.id ?? null, reservationId: preset?.reservationId ?? null,
      environment: environment || null, requiredDate: requiredDate || null,
      expiryDate: requestType === 'reservation' ? expiryDate : null,
    }),
    onSuccess: (r) => {
      refresh()
      onClose()
      navigate(`/requests/mine?id=${r.id}`)
    },
  })
  // A draft can be unfinished, but what it does hold must be valid.
  const checks = useChecks(check({
    requestedAddress: mode === 'specific' && !release && !conversion ? (address.trim() ? ip(address) : 'Enter the address you want.') : undefined,
  }), open)
  const submitChecks = useChecks(check({
    subnetId: !release && !conversion && !subnet ? 'Choose a subnet.' : undefined,
    requestedAddress: mode === 'specific' && !release && !conversion ? (address.trim() ? ip(address) : 'Enter the address you want.') : undefined,
    justification: required(justification, 'Justification'),
    expiryDate: requestType === 'reservation' && !expiryDate ? 'Choose an end date.' : undefined,
  }), open)
  const errors = { ...fieldIssues(save.error), ...checks.shown, ...submitChecks.shown }
  const fieldLevel = Object.keys(errors).length > 0
  const title = conversion ? 'Convert reservation to allocation' : release ? 'Request a release' : 'New request'

  return (
    <Modal open={open} onClose={onClose} title={title} size="lg"
      subtitle="Requests are approved before anything changes. You can't approve your own."
      footer={<>
        <Button onClick={onClose}>Cancel</Button>
        <Button busy={save.isPending && save.variables === false} disabled={save.isPending} onClick={() => checks.submit(() => save.mutate(false))}>Save draft</Button>
        <Button variant="primary" busy={save.isPending && save.variables === true} disabled={save.isPending} onClick={() => submitChecks.submit(() => save.mutate(true))}>Submit for approval</Button>
      </>}>
      <div className="space-y-4">
        {!preset && (
          <Field label="Request type" htmlFor="rq-type">
            <Select id="rq-type" value={requestType} onChange={(e) => setRequestType(e.target.value as RequestType)}>
              <option value="allocation">Allocation: give an address to a use</option>
              <option value="reservation">Reservation: hold an address for planned work</option>
            </Select>
          </Field>
        )}
        {release && preset?.allocation && (
          <dl className="rounded-[5px] border border-line bg-surface-2 px-3 py-1">
            <Detail label="Address"><Mono>{preset.allocation.address}</Mono></Detail>
            <Detail label="Subnet">{preset.allocation.subnetName} (<Mono>{preset.allocation.subnetCidr}</Mono>)</Detail>
            <Detail label="Allocated">{formatDateTime(preset.allocation.allocatedAt)} by {preset.allocation.allocatedByEmail}</Detail>
          </dl>
        )}
        {conversion && (
          <p className="rounded-[5px] bg-accent-soft px-3 py-2 text-[13px] text-accent">
            Once approved and fulfilled, reserved address <Mono>{preset?.reservationAddress}</Mono> becomes an allocation.
          </p>
        )}
        {!release && !conversion && (
          <>
            <Field label="Subnet" htmlFor="rq-subnet" error={errors.subnetId}>
              <SubnetPicker id="rq-subnet" value={subnet} invalid={Boolean(errors.subnetId)} onChange={(s) => { setSubnet(s); if (s && !environment) setEnvironment(s.environment) }} />
            </Field>
            <div className="grid grid-cols-[15rem_1fr] gap-3">
              <Field label="Address" htmlFor="rq-mode" error={errors.selectionMode}>
                <Select id="rq-mode" value={mode} onChange={(e) => setMode(e.target.value as 'automatic' | 'specific')}>
                  <option value="automatic">Next available candidate</option>
                  <option value="specific">A specific address</option>
                </Select>
              </Field>
              {mode === 'specific' && (
                <Field label="Specific address" htmlFor="rq-address" error={errors.requestedAddress}>
                  <Input id="rq-address" className="font-mono" spellCheck={false} placeholder={subnet?.calculated.firstUsable ?? '10.20.30.40'}
                    value={address} onChange={(e) => setAddress(e.target.value)} />
                </Field>
              )}
            </div>
          </>
        )}
        <Field label="Justification" htmlFor="rq-justification" error={errors.justification} hint="What the address is for. Approvers see this.">
          <textarea id="rq-justification" rows={3} value={justification} onChange={(e) => setJustification(e.target.value)}
            className="w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 py-1.5 text-[13px] text-ink focus:border-accent focus:bg-surface focus:outline-none" />
        </Field>
        {requestType === 'reservation' && (
              <Field label="Reservation ends" htmlFor="rq-expiry" error={errors.expiryDate} hint="At most a year ahead. The holder is reminded a week before.">
                <Input id="rq-expiry" type="date" min={isoDate(1)} max={isoDate(365)} value={expiryDate} onChange={(e) => setExpiryDate(e.target.value)} />
              </Field>
        )}
        <MoreOptions forceOpen={Boolean(errors.environment || errors.requiredDate)}>
          <div className="space-y-4">
        {!release && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Department (optional)" htmlFor="rq-dept"><Input id="rq-dept" value={department} onChange={(e) => setDepartment(e.target.value)} /></Field>
            <Field label="Environment" htmlFor="rq-env" error={errors.environment}>
              <Select id="rq-env" value={environment} onChange={(e) => setEnvironment(e.target.value)}>
                <option value="">Same as the subnet</option>
                {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
              </Select>
            </Field>
            <Field label="Needed by (optional)" htmlFor="rq-required" error={errors.requiredDate}>
              <Input id="rq-required" type="date" value={requiredDate} onChange={(e) => setRequiredDate(e.target.value)} />
            </Field>
          </div>
        )}
        <Field label="Comments (optional)" htmlFor="rq-comments"><Input id="rq-comments" value={comments} onChange={(e) => setComments(e.target.value)} /></Field>
          </div>
        </MoreOptions>
        {!fieldLevel && <FormError error={save.error} />}
      </div>
    </Modal>
  )
}
