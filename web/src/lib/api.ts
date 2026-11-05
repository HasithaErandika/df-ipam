import { getAccessToken } from './auth'

export interface FieldIssue { field: string; issue: string }

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public details: FieldIssue[] = [],
    public correlationId?: string,
  ) {
    super(message)
  }
}

export interface PageMeta {
  page: number
  pageSize: number
  total: number
  totalPages: number
  hasNext: boolean
  hasPrev: boolean
}

export interface ListResult<T> {
  data: T[]
  meta: PageMeta
}

export async function parseFailure(res: Response): Promise<ApiError> {
  try {
    const { error } = await res.json()
    if (error?.code) return new ApiError(res.status, error.code, error.message, error.details ?? [], error.correlationId)
  } catch {
    // Not our error format: the proxy answered because the API isn't running.
  }
  return new ApiError(res.status, 'API_UNAVAILABLE', "The DF-IPAM API isn't responding.")
}

async function send(path: string, init: RequestInit = {}): Promise<Response> {
  const token = await getAccessToken()
  const headers = new Headers(init.headers)
  if (token) headers.set('Authorization', `Bearer ${token}`)
  if (init.body) headers.set('Content-Type', 'application/json')
  let res: Response
  try {
    res = await fetch(`${import.meta.env.VITE_API_URL ?? ''}/api/v1${path}`, { ...init, headers })
  } catch {
    throw new ApiError(0, 'API_UNAVAILABLE', "The DF-IPAM API isn't responding.")
  }
  if (!res.ok) throw await parseFailure(res)
  return res
}

export async function get<T>(path: string): Promise<T> {
  return (await (await send(path)).json()).data
}

/** The whole response body, for endpoints whose metadata isn't a page. */
export async function getBody<T>(path: string): Promise<T> {
  return (await send(path)).json()
}

export async function list<T>(path: string): Promise<ListResult<T>> {
  return (await send(path)).json()
}

/** Loads every page, for dropdowns. A capped page would silently hide options. */
export async function listAll<T>(path: string): Promise<T[]> {
  const join = path.includes('?') ? '&' : '?'
  const out: T[] = []
  for (let page = 1; ; page++) {
    const res = await list<T>(`${path}${join}page=${page}&pageSize=200`)
    out.push(...res.data)
    if (!res.meta.hasNext) return out
  }
}

export async function post<T>(path: string, body?: unknown): Promise<T> {
  const res = await send(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) })
  return res.status === 204 ? (undefined as T) : (await res.json()).data
}

export async function del(path: string): Promise<void> {
  await send(path, { method: 'DELETE' })
}

export async function put(path: string, body: unknown): Promise<void> {
  await send(path, { method: 'PUT', body: JSON.stringify(body) })
}

/** PATCH for records without a version, such as reference data. */
export async function patchPlain<T>(path: string, body: unknown): Promise<T> {
  const res = await send(path, { method: 'PATCH', body: JSON.stringify(body) })
  return (await res.json()).data
}

export async function patch<T>(path: string, body: unknown, version: number): Promise<T> {
  const res = await send(path, { method: 'PATCH', body: JSON.stringify(body), headers: { 'If-Match': String(version) } })
  return (await res.json()).data
}

export function queryString(params: Record<string, string | number | boolean | undefined>): string {
  const q = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '' && value !== false) q.set(key, String(value))
  }
  const s = q.toString()
  return s ? `?${s}` : ''
}
