import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useMutation } from '@tanstack/react-query'
import { ChevronDown, Radar } from 'lucide-react'
import { Button } from '../../components/ui'
import { post } from '../../lib/api'
import { label, segmentKinds } from '../../lib/labels'
import { useInvalidate, useSegments } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { Segment } from '../../lib/types'

const item = 'flex cursor-pointer items-center justify-between gap-4 rounded-[4px] px-2.5 py-1.5 text-[13px] text-ink-2 outline-none data-[highlighted]:bg-bg data-[highlighted]:text-ink data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50'

function useScan() {
  const refresh = useInvalidate('/discovery', '/segments')
  return useMutation({
    mutationFn: (segmentId?: string) => post('/discovery/scan', segmentId ? { segmentId } : {}),
    meta: { success: (id?: string) => (id ? 'Scan of the segment started.' : 'Scan of all segments started. Results appear as each one finishes.') },
    onSuccess: () => void refresh(),
  })
}

/**
 * Starts a scan now, for Admins. With a segment it scans that one; without, it
 * offers all segments or a chosen one. The server refuses while a scan runs and
 * spaces manual scans a minute apart, so this can't flood the network.
 */
export function ScanButton({ segmentId, running, label: text = 'Scan now' }: { segmentId?: string; running?: boolean; label?: string }) {
  const can = useCan('discovery:scan')
  const segments = useSegments()
  const scan = useScan()
  if (!can) return null
  if (segmentId) {
    return (
      <Button busy={scan.isPending || running} disabled={running} onClick={() => scan.mutate(segmentId)} title={running ? 'A scan is running' : undefined}>
        {!running && !scan.isPending && <Radar size={14} aria-hidden />}{running ? 'Scanning…' : text}
      </Button>
    )
  }
  const groups: [string, Segment[]][] = (['dhcp', 'server', 'cloud'] as const)
    .map((k) => [k, (segments.data ?? []).filter((s) => s.kind === k)] as [string, Segment[]])
    .filter(([, list]) => list.length > 0)
  const sweepable = (segments.data ?? []).filter((s) => s.kind !== 'cloud').length
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild disabled={running}>
        <Button busy={scan.isPending || running} title={running ? 'A scan is running' : 'Choose what to scan'}>
          {!running && !scan.isPending && <Radar size={14} aria-hidden />}{running ? 'Scanning…' : text}{!running && <ChevronDown size={13} aria-hidden />}
        </Button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={4} className="z-30 max-h-[70vh] w-72 overflow-y-auto rounded-md border border-line-strong/70 bg-surface p-1 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]">
          <DropdownMenu.Item className={item} disabled={sweepable === 0} onSelect={() => scan.mutate(undefined)}>
            <span className="font-medium text-ink">All segments</span><span className="text-[12px] text-ink-3">{sweepable}</span>
          </DropdownMenu.Item>
          {sweepable === 0 && <p className="px-2.5 py-1.5 text-[12px] text-ink-3">Add segments in Network setup to scan them.</p>}
          {groups.map(([kind, list]) => (
            <DropdownMenu.Group key={kind}>
              <DropdownMenu.Separator className="my-1 h-px bg-line" />
              <DropdownMenu.Label className="px-2.5 pt-1 pb-0.5 text-[10.5px] font-semibold tracking-[.06em] text-ink-3 uppercase">
                {label(segmentKinds, kind)}{kind === 'cloud' ? ' · not swept' : ''}
              </DropdownMenu.Label>
              {list.map((s) => (
                <DropdownMenu.Item key={s.id} className={item} disabled={kind === 'cloud'} onSelect={() => scan.mutate(s.id)}>
                  <span className="truncate">{s.name}</span><span className="shrink-0 font-mono text-[11.5px] text-ink-3">{s.network}</span>
                </DropdownMenu.Item>
              ))}
            </DropdownMenu.Group>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
