import { describe, expect, it, vi } from 'vitest'
import type { Batch } from './shared'

vi.mock('../../lib/auth', () => ({ getAccessToken: async () => null }))
const { fileIssue, maxFileBytes, recordLabel, statusOptions, toBase64 } = await import('./shared')

describe('fileIssue', () => {
  it('accepts each kind of file only where it belongs', () => {
    expect(fileIssue({ name: 'network.XLSX', size: 10 }, 'template')).toBeUndefined()
    expect(fileIssue({ name: 'sites.csv', size: 10 }, 'template')).toBeUndefined()
    expect(fileIssue({ name: 'Prod_Subnets.txt', size: 10 }, 'template')).toMatch(/template/)
    expect(fileIssue({ name: 'Prod_Subnets.txt', size: 10 }, 'export')).toBeUndefined()
    expect(fileIssue({ name: 'network.xlsx', size: 10 }, 'export')).toMatch(/export/)
  })

  it('refuses missing, empty and oversized files', () => {
    expect(fileIssue(null, 'template')).toBe('Choose a file.')
    expect(fileIssue({ name: 'a.csv', size: 0 }, 'template')).toBe('This file is empty.')
    expect(fileIssue({ name: 'a.csv', size: maxFileBytes + 1 }, 'template')).toMatch(/5 MB/)
  })
})

describe('toBase64', () => {
  it('encodes binary data across chunk boundaries', () => {
    const bytes = new Uint8Array(0x8000 * 2 + 5).map((_, i) => i % 256)
    expect(Uint8Array.from(atob(toBase64(bytes)), (c) => c.charCodeAt(0))).toEqual(bytes)
  })
})

describe('line statuses', () => {
  const summary = { lines: 6, valid: 3, invalid: 2, ignored: 0, duplicate: 1, overlap: 0, needsEnrichment: 0, committed: 0 }

  it('calls a template duplicate what it is', () => {
    expect(recordLabel('duplicate', true)).toBe('Already exists')
    expect(recordLabel('duplicate', false)).toBe('Duplicate')
  })

  it('offers only the statuses a batch holds', () => {
    const options = statusOptions({ format: 'template', summary } as Batch)
    expect(options).toEqual([{ value: 'valid', label: 'Ready (3)' }, { value: 'invalid', label: 'Invalid (2)' }, { value: 'duplicate', label: 'Already exists (1)' }])
  })
})
