import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Button, Field, FormError, FormSection, Input, Modal, Notice, Select, Switch, useConfirm } from '../../components/ui'
import { post } from '../../lib/api'
import { deviceTypes, ipTypes, options } from '../../lib/labels'
import { inventoryKeys, useInvalidate } from '../../lib/queries'
import { knownPlatform, likelyPlatform } from '../../lib/platforms'
import { AddressCheck } from './AddressCheck'
import { DepartmentSelect } from './DepartmentSelect'
import { PlatformSelect } from './PlatformFields'

/** What the approval dialog needs to know about a device. */
export interface DecisionTarget {
  id: string
  mac: string
  name?: string | null
  owner?: string
  department?: string
  deviceType?: string
  lastIp?: string | null
  segmentId?: string | null
  segmentName?: string | null
  hostname?: string | null
  registered?: boolean
  vendor?: string | null
  platform?: string | null
  model?: string | null
  osHint?: string | null
  /** The register type the evidence suggests, for registering its IP in the same step. */
  ipType?: string
}

/**
 * Approves a device, naming it, and can add the address it was seen on to the
 * register in the same step (docs/product/register.md).
 */
export function ApproveDeviceModal({ target, onClose }: { target: DecisionTarget | null; onClose: () => void }) {
  const refresh = useInvalidate(...inventoryKeys)
  const [name, setName] = useState('')
  const [owner, setOwner] = useState('')
  const [department, setDepartment] = useState('')
  const [deviceType, setDeviceType] = useState('other')
  const [platform, setPlatform] = useState('')
  const [registerIp, setRegisterIp] = useState(false)
  const [ipType, setIpType] = useState('user_pc')
  const [error, setError] = useState<unknown>(null)
  const canRegister = Boolean(target?.lastIp && target.segmentId && !target.registered)

  useEffect(() => {
    if (!target) return
    setName(target.name || target.hostname || '')
    setOwner(target.owner ?? '')
    setDepartment(target.department ?? '')
    setDeviceType(target.deviceType && target.deviceType !== 'other' ? target.deviceType : 'pc')
    setPlatform(knownPlatform(target.platform))
    setRegisterIp(false)
    setIpType(target.ipType ?? 'user_pc')
    setError(null)
  }, [target])

  const approve = useMutation({
    mutationFn: () => post(`/devices/${target!.id}/decision`, {
      status: 'approved', name, owner, department, deviceType, platform,
      register: registerIp ? { segmentId: target!.segmentId, ip: target!.lastIp, ipType } : undefined,
    }),
    meta: { inlineError: true, success: registerIp ? 'Device approved and its IP registered.' : 'Device approved.' },
    onSuccess: () => { void refresh(); onClose() },
    onError: setError,
  })

  return (
    <Modal open={target !== null} onClose={onClose} title="Approve device" subtitle={<span className="font-mono">{target?.mac}</span>}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={approve.isPending} disabled={!name.trim()} onClick={() => approve.mutate()}>Approve</Button></>}>
      <div className="space-y-5">
        <Notice>Filled in from what discovery found: name, type, platform and user. Check them before approving.</Notice>
        <FormSection title="Device">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Name" htmlFor="dev-name" error={!name.trim() ? 'Give the device a name.' : undefined}><Input id="dev-name" value={name} maxLength={120} onChange={(e) => setName(e.target.value)} /></Field>
            <Field label="Type" htmlFor="dev-type"><Select id="dev-type" value={deviceType} onChange={(e) => setDeviceType(e.target.value)}>{options(deviceTypes).map((o) => <option key={o.value} value={o.value}>{o.text}</option>)}</Select></Field>
            <PlatformSelect id="dev-platform" value={platform} onChange={setPlatform} guess={target ? likelyPlatform(target) : null} />
            <div className="hidden sm:block" />
            <Field label="Owner" htmlFor="dev-owner"><Input id="dev-owner" value={owner} maxLength={120} onChange={(e) => setOwner(e.target.value)} /></Field>
            <DepartmentSelect id="dev-dept" value={department} onChange={setDepartment} />
          </div>
        </FormSection>
        {canRegister && (
          <FormSection title="Register its IP">
            <Switch id="dev-register" checked={registerIp} onChange={setRegisterIp} label={<>Also register <span className="font-mono">{target?.lastIp}</span> in {target?.segmentName}</>}
              hint="The address it was last seen on becomes an Assigned entry for this device." />
            {registerIp && (
              <>
                <Field label="IP type" htmlFor="dev-iptype"><Select id="dev-iptype" value={ipType} onChange={(e) => setIpType(e.target.value)}>{options(ipTypes).map((o) => <option key={o.value} value={o.value}>{o.text}</option>)}</Select></Field>
                <AddressCheck segmentId={target!.segmentId!} ip={target!.lastIp!} />
              </>
            )}
          </FormSection>
        )}
        <FormError error={error} />
      </div>
    </Modal>
  )
}

/** Rejecting needs only a confirmation. A rejected MAC raises a High alert if it comes back. */
export function useRejectDevice() {
  const confirm = useConfirm()
  const refresh = useInvalidate(...inventoryKeys)
  const reject = useMutation({
    mutationFn: (id: string) => post(`/devices/${id}/decision`, { status: 'rejected' }),
    meta: { success: 'Device rejected. A High alert is raised if it appears again.' },
    onSuccess: () => void refresh(),
  })
  return async (target: { id: string; mac: string }) => {
    if (await confirm({ title: `Reject ${target.mac}?`, body: 'The device is marked as not allowed. DF-IPAM does not block it: it raises a High alert whenever it is seen, so someone can act.', confirmLabel: 'Reject device' })) {
      reject.mutate(target.id)
    }
  }
}
