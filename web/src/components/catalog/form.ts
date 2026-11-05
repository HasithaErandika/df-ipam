import { cidr, ip, type Issue, issues, required } from '../../lib/validation'
import type { EntityDef, FieldDef, Row } from './types'

/** What a browser's date and time box holds, in the person's own time zone. */
function toLocalInput(v: string): string {
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return ''
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}

function checkField(f: FieldDef, value: string): Issue {
  const v = value.trim()
  if (v === '') return f.required ? (f.type === 'ref' || f.type === 'select' ? `Choose ${f.label.toLowerCase()}.` : required(v, f.label)) : undefined
  if (f.type === 'cidr') return cidr(v)
  if (f.type === 'ip') return ip(v)
  if (f.type === 'number') {
    const n = Number(v)
    if (!Number.isInteger(n)) return `${f.label} must be a whole number.`
    if ((f.min !== undefined && n < f.min) || (f.max !== undefined && n > f.max)) return `${f.label} must be from ${f.min} to ${f.max}.`
  }
  return f.check?.(v)
}

export function checkForm(def: EntityDef, form: Record<string, string | boolean>, editing: boolean): Record<string, string> {
  const fields = def.fields.filter((f) => f.type !== 'bool' && !(editing && f.createOnly))
  return issues({ ...Object.fromEntries(fields.map((f) => [f.key, checkField(f, String(form[f.key] ?? ''))])), ...def.validate?.(form) })
}

export function toForm(def: EntityDef, row: Row | null): Record<string, string | boolean> {
  const out: Record<string, string | boolean> = {}
  for (const f of def.fields) {
    const v = row?.[f.key]
    if (f.type === 'bool') out[f.key] = Boolean(v)
    else if (f.type === 'date') out[f.key] = typeof v === 'string' ? v.slice(0, 10) : ''
    else if (f.type === 'datetime') out[f.key] = typeof v === 'string' ? toLocalInput(v) : ''
    else out[f.key] = v === null || v === undefined ? '' : String(v)
  }
  return out
}

export function toBody(def: EntityDef, form: Record<string, string | boolean>, editing: boolean) {
  const body: Record<string, unknown> = {}
  for (const f of def.fields) {
    if (editing && f.createOnly) continue
    const v = form[f.key]
    if (f.type === 'bool') body[f.key] = Boolean(v)
    else if (f.type === 'number') body[f.key] = v === '' ? null : Number(v)
    else if (f.type === 'datetime' && v) body[f.key] = new Date(String(v)).toISOString()
    else if (v === '' && f.type !== 'text') body[f.key] = editing ? '' : undefined
    else body[f.key] = v
  }
  return body
}
