import { describe, expect, it, vi } from 'vitest'

vi.mock('./auth', () => ({ getAccessToken: async () => null }))
const { ApiError, errorMessage, isStaleBuild, parseFailure, queryString } = await import('./api')

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

describe('errorMessage', () => {
  it('uses the server message', () => expect(errorMessage(new ApiError(422, 'VALIDATION_FAILED', 'Enter a name.'))).toBe('Enter a name.'))
  it('explains a lost connection', () => expect(errorMessage(new ApiError(0, 'API_UNAVAILABLE', 'x'))).toContain("Can't reach the server"))
  it('explains an ended session', () => expect(errorMessage(new ApiError(401, 'UNAUTHENTICATED', 'x'))).toContain('Sign in again'))
  it('hides technical messages', () => expect(errorMessage(new TypeError('x is undefined'))).toBe('Something went wrong. Try again.'))
})

describe('isStaleBuild', () => {
  it('spots a missing screen after a deploy', () => expect(isStaleBuild(new TypeError('Failed to fetch dynamically imported module: /assets/a.js'))).toBe(true))
  it('ignores other errors', () => expect(isStaleBuild(new Error('boom'))).toBe(false))
})
