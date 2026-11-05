import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bookmark, X } from 'lucide-react'
import { useState } from 'react'
import { del, list, post } from '../lib/api'
import type { SavedFilter } from '../lib/types'
import { Button, Input } from './ui'

/**
 * Lets a person save the page's current filters under a name and apply them
 * later. Filters are the page's own query parameters, so they pass through the
 * same server-side allowlists as anything typed by hand.
 */
export function SavedFilters({ page, current, onApply }: { page: string; current: Record<string, string>; onApply: (f: Record<string, string>) => void }) {
  const queryClient = useQueryClient()
  const [naming, setNaming] = useState(false)
  const [name, setName] = useState('')
  const saved = useQuery({ queryKey: ['saved-filters', page], queryFn: () => list<SavedFilter>(`/saved-filters?page=${page}`) })
  const filters = Object.fromEntries(Object.entries(current).filter(([, v]) => v !== ''))
  const save = useMutation({
    mutationFn: () => post('/saved-filters', { page, name, filters }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['saved-filters', page] }); setNaming(false); setName('') },
  })
  const remove = useMutation({
    mutationFn: (id: string) => del(`/saved-filters/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['saved-filters', page] }),
  })
  const options = saved.data?.data ?? []
  if (naming) {
    return (
      <form className="flex items-center gap-1" onSubmit={(e) => { e.preventDefault(); if (name.trim()) save.mutate() }}>
        <Input aria-label="Filter name" autoFocus className="w-40!" placeholder="Name this filter" value={name} onChange={(e) => setName(e.target.value)} />
        <Button type="submit" variant="primary" busy={save.isPending} disabled={!name.trim()}>Save</Button>
        <Button variant="ghost" className="px-1.5" aria-label="Cancel" onClick={() => setNaming(false)}><X size={14} /></Button>
      </form>
    )
  }
  return (
    <div className="flex flex-wrap items-center gap-1">
      <Button variant="ghost" title="Save these filters" disabled={Object.keys(filters).length === 0} onClick={() => setNaming(true)}>
        <Bookmark size={14} aria-hidden /> Save filter
      </Button>
      {options.map((o) => (
        <span key={o.id} className="inline-flex h-7 items-center rounded-[5px] border border-line bg-surface text-[12.5px]">
          <button type="button" className="cursor-pointer px-2 text-ink-2 hover:text-accent" title="Apply this filter" onClick={() => onApply(o.filters)}>{o.name}</button>
          <button type="button" className="cursor-pointer border-l border-line px-1 text-ink-3 hover:text-danger" aria-label={`Delete saved filter ${o.name}`}
            disabled={remove.isPending} onClick={() => remove.mutate(o.id)}><X size={12} /></button>
        </span>
      ))}
    </div>
  )
}
