import { z } from 'zod'
import type { RegisterEntry } from '../../lib/types'
import { choose, ipv4, optionalInt, optionalMAC, text } from '../../lib/validation'

// The register entry form's schema and starting values, used by RegisterIPModal.
export const schema = z.object({
  segmentId: choose('a segment'),
  ip: ipv4,
  status: z.enum(['assigned', 'reserved', 'blocked']),
  ipType: z.string(),
  deviceId: z.string(),
  environmentId: z.string(),
  assignedName: text('Assigned to', 120),
  platform: z.string(),
  services: z.array(z.string()).max(10),
  expectedMac: optionalMAC,
  owner: text('Owner', 120),
  department: text('Department', 120),
  monitor: z.boolean(),
  critical: z.boolean(),
  checkPort: optionalInt('Check port', 1, 65535),
  notes: text('Notes', 1000),
}).superRefine((v, ctx) => {
  if (v.status === 'assigned' && !v.assignedName) ctx.addIssue({ code: 'custom', path: ['assignedName'], message: 'Say who or what the address is assigned to.' })
  if (v.critical && !v.monitor) ctx.addIssue({ code: 'custom', path: ['critical'], message: 'Turn on Monitor to mark an entry critical.' })
})
export type Values = z.infer<typeof schema>

/** What to pre-fill when registering from something seen on the network. */
export interface RegisterPrefill { segmentId?: string; ip?: string; mac?: string | null; name?: string | null; ipType?: string; platform?: string; platformGuess?: string | null; owner?: string; notes?: string }

export const fillable = ['deviceId', 'ipType', 'assignedName', 'platform', 'owner', 'department'] as const

export function initial(entry?: RegisterEntry, prefill?: RegisterPrefill): Values {
  return {
    segmentId: entry?.segmentId ?? prefill?.segmentId ?? '', ip: entry?.ip ?? prefill?.ip ?? '', status: entry?.status ?? 'assigned',
    ipType: entry?.ipType ?? prefill?.ipType ?? 'other', deviceId: entry?.deviceId ?? '', environmentId: entry?.environmentId ?? '', assignedName: entry?.assignedName ?? prefill?.name ?? '',
    platform: entry?.platform ?? prefill?.platform ?? '', services: entry?.services ?? [],
    expectedMac: entry?.expectedMac ?? prefill?.mac ?? '', owner: entry?.owner ?? prefill?.owner ?? '', department: entry?.department ?? '',
    monitor: entry?.monitor ?? false, critical: entry?.critical ?? false, checkPort: entry?.checkPort ? String(entry.checkPort) : '', notes: entry?.notes ?? prefill?.notes ?? '',
  }
}
