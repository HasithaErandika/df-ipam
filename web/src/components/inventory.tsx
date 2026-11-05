import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bookmark, Check, Columns3, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { del, list, post } from '../lib/api'
import { Badge, Button, Field, FormError, Input, Modal } from './ui'

export const allocationStates: Record<string, string> = {
  allocated: 'Allocated', reserved: 'Reserved', available_candidate: 'Available candidate', quarantined: 'Quarantined', retired: 'Retired',
}

export const observationStates: Record<string, string> = {
  unknown: 'Unknown', observed_active: 'Observed active', observed_inactive: 'Observed inactive', conflict_suspected: 'Conflict suspected',
}

const allocationTone: Record<string, 'accent' | 'warn' | 'ok' | 'danger' | 'neutral'> = {
  allocated: 'accent', reserved: 'warn', available_candidate: 'ok', quarantined: 'danger', retired: 'neutral',
}
const observationTone: Record<string, 'accent' | 'warn' | 'ok' | 'danger' | 'neutral'> = {
  unknown: 'neutral', observed_active: 'ok', observed_inactive: 'neutral', conflict_suspected: 'danger',
}

/** Allocation and observation are always two separate badges. */
export function StateBadges({ allocation, observation }: { allocation: string; observation: string }) {
  return (
    <span className="inline-flex flex-wrap gap-1">
      <Badge tone={allocationTone[allocation] ?? 'neutral'}>{allocationStates[allocation] ?? allocation}</Badge>
      <Badge tone={observationTone[observation] ?? 'neutral'}>{observationStates[observation] ?? observation}</Badge>
    </span>
  )
}

const menu = 'z-30 min-w-56 rounded-md border border-line-strong/70 bg-surface p-1 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]'
const item = 'flex h-8 cursor-pointer items-center gap-2 rounded-[4px] px-2.5 text-[13px] text-ink-2 outline-none data-[highlighted]:bg-bg data-[highlighted]:text-ink'

export function ColumnMenu({ columns, visible, onToggle }:
  { columns: Record<string, string>; visible: (c: string) => boolean; onToggle: (c: string) => void }) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <Button variant="secondary" aria-label="Choose columns"><Columns3 size={15} aria-hidden /> Columns</Button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={6} className={menu}>
          {Object.entries(columns).map(([key, label]) => (
            <DropdownMenu.CheckboxItem key={key} checked={visible(key)} onSelect={(e) => e.preventDefault()} onCheckedChange={() => onToggle(key)} className={item}>
              <span className="grid size-4 place-items-center rounded-[3px] border border-line-strong">
                <DropdownMenu.ItemIndicator><Check size={12} aria-hidden /></DropdownMenu.ItemIndicator>
              </span>
              {label}
            </DropdownMenu.CheckboxItem>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

interface Saved { id: string; name: string; filters: Record<string, string> }

/** Named filter sets, stored per user on the server. */
export function SavedFilters({ page, current, onApply }:
  { page: 'subnets' | 'ip-addresses'; current: Record<string, string>; onApply: (filters: Record<string, string>) => void }) {
  const queryClient = useQueryClient()
  const key = ['saved-filters', page]
  const saved = useQuery({ queryKey: key, queryFn: () => list<Saved>(`/saved-filters?page=${page}&pageSize=200`) })
  const [naming, setNaming] = useState(false)
  const [name, setName] = useState('')
  const active = Object.fromEntries(Object.entries(current).filter(([, v]) => v))
  const create = useMutation({
    mutationFn: () => post('/saved-filters', { page, name, filters: active }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: key }); setNaming(false); setName('') },
  })
  const remove = useMutation({
    mutationFn: (id: string) => del(`/saved-filters/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: key }),
  })
  const submit = (e: FormEvent) => { e.preventDefault(); create.mutate() }
  const rows = saved.data?.data ?? []
  return (
    <>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <Button variant="secondary" aria-label="Saved filters"><Bookmark size={15} aria-hidden /> Saved{rows.length > 0 && <Badge>{rows.length}</Badge>}</Button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content align="end" sideOffset={6} className={menu}>
            {saved.isPending && <div className="px-2.5 py-1.5 text-[13px] text-ink-3">Loading…</div>}
            {saved.error && <div className="px-2.5 py-1.5 text-[13px] text-danger">Saved filters could not be loaded.</div>}
            {saved.data && rows.length === 0 && <div className="px-2.5 py-1.5 text-[13px] text-ink-3">No saved filters yet.</div>}
            {rows.map((f) => (
              <div key={f.id} className="flex items-center">
                <DropdownMenu.Item className={`${item} flex-1`} onSelect={() => onApply(f.filters)}>{f.name}</DropdownMenu.Item>
                <button type="button" aria-label={`Delete ${f.name}`} onClick={() => remove.mutate(f.id)}
                  className="grid size-7 cursor-pointer place-items-center rounded text-ink-3 hover:bg-bg hover:text-danger"><Trash2 size={13} /></button>
              </div>
            ))}
            <DropdownMenu.Separator className="my-1 h-px bg-line" />
            <DropdownMenu.Item disabled={Object.keys(active).length === 0} onSelect={() => setNaming(true)}
              className={`${item} data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50`}>Save current filters…</DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      <Modal open={naming} onClose={() => setNaming(false)} title="Save filters"
        footer={<><Button onClick={() => setNaming(false)}>Cancel</Button><Button variant="primary" type="submit" form="save-filter" busy={create.isPending}>Save</Button></>}>
        <form id="save-filter" onSubmit={submit} className="space-y-3">
          <Field label="Name" htmlFor="filter-name"><Input id="filter-name" required autoFocus maxLength={80} value={name} onChange={(e) => setName(e.target.value)} /></Field>
          <p className="text-[13px] text-ink-3">Only you see your saved filters.</p>
          <FormError error={create.error} />
        </form>
      </Modal>
    </>
  )
}

/** Bar shown while rows are selected. */
export function SelectionBar({ count, onClear, children }: { count: number; onClear: () => void; children: React.ReactNode }) {
  if (count === 0) return null
  return (
    <div role="region" aria-label="Bulk actions" className="flex flex-wrap items-center gap-2 border-b border-line bg-accent-soft px-3 py-2 text-[13px]">
      <span className="font-medium text-accent">{count} selected</span>
      <Button variant="ghost" className="h-7 px-2" onClick={onClear}>Clear</Button>
      <div className="ml-auto flex flex-wrap gap-2">{children}</div>
    </div>
  )
}
