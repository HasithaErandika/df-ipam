import { describe, expect, it, vi } from 'vitest'

vi.mock('./auth', () => ({ getAccessToken: async () => null }))
const { parseFailure, queryString } = await import('./api')

describe('parseFailure', () => {
  it('reads the shared error format', async () => {
    const res = new Response(
      JSON.stringify({ error: { code: 'SUBNET_OVERLAP', message: 'Overlaps.', correlationId: 'abc' } }),
      { status: 409 },
    )
    const err = await parseFailure(res)
    expect([err.status, err.code, err.correlationId]).toEqual([409, 'SUBNET_OVERLAP', 'abc'])
  })
  it('treats anything else as the API being down', async () => {
    expect((await parseFailure(new Response('Bad gateway', { status: 502 }))).code).toBe('API_UNAVAILABLE')
  })
})

describe('queryString', () => {
  it('drops empty values', () => expect(queryString({ q: '', page: 2, archived: false })).toBe('?page=2'))
})
