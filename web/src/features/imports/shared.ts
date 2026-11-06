import { useMutation } from '@tanstack/react-query'
import { download } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'

interface Summary {
  lines: number; valid: number; invalid: number; ignored: number; duplicate: number; overlap: number; needsEnrichment: number; committed: number
  sheets?: Record<string, number>
}

export interface Batch {
  id: string; fileName: string; fileHash: string; format: string; status: string; summary: Summary; routingDomainName: string | null; environment: string | null
  uploadedBy: string; uploadedByEmail: string; approvedByEmail: string | null; createdAt: string; committedAt: string | null; reversedAt: string | null
}

interface Parsed { cidr?: string; name?: string; gateway?: string; vdom?: string; vlan?: string; comment?: boolean; type?: string; sheet?: string; row?: number }

export interface ImportRecord { id: string; batchId: string; lineNumber: number; rawText: string; parsed: Parsed; status: string; errors: string[]; createdObjectId: string | null; fileName?: string }

export interface TemplateColumn { name: string; required: boolean; options?: string[]; refersTo?: string }

export interface TemplateInfo { key: string; title: string; columns: TemplateColumn[] }

export type Tone = 'ok' | 'danger' | 'warn' | 'neutral' | 'accent'

export const templateFormat = 'template'

export const formats: Record<string, string> = {
  prod_subnets: 'Production subnets (Prod_Subnets.txt)', prod_vdoms_subnets: 'Production VDOM subnets (Prod_VDOMs_Subnets.txt)', csv: 'CSV: cidr, name, gateway, vlan',
}

export const formatLabel = (format: string) => format === templateFormat ? 'Template' : formats[format] ?? format

export const recordTone: Record<string, Tone> = {
  valid: 'ok', invalid: 'danger', duplicate: 'neutral', overlap: 'warn', needs_enrichment: 'warn', committed: 'accent',
}

export const batchTone: Record<string, Tone> = {
  validated: 'warn', committed: 'ok', reversed: 'neutral', discarded: 'neutral', failed: 'danger', uploaded: 'neutral', approved: 'accent',
}

const batchLabels: Record<string, string> = {
  validated: 'Waiting for approval', committed: 'Imported', reversed: 'Reversed', discarded: 'Discarded', failed: 'Failed', uploaded: 'Uploaded', approved: 'Approved',
}

export const batchLabel = (status: string) => batchLabels[status] ?? status

const recordLabels: Record<string, string> = {
  valid: 'Ready', invalid: 'Invalid', duplicate: 'Duplicate', overlap: 'Overlap', needs_enrichment: 'Needs enrichment', committed: 'Imported',
}

/** In a template, a duplicate is a record that already exists, and it is skipped. */
export const recordLabel = (status: string, template: boolean) =>
  template && status === 'duplicate' ? 'Already exists' : recordLabels[status] ?? status

/** Lines that won't be imported as they stand. A duplicate isn't a fault, so this isn't "problems". */
export const skippedCount = (s: Summary) => s.invalid + s.duplicate + s.overlap + s.needsEnrichment

/** The line statuses a batch holds, with their counts, for its filter. */
export function statusOptions(batch: Batch): { value: string; label: string }[] {
  const s = batch.summary
  const counts: [string, number][] = [['valid', s.valid], ['invalid', s.invalid], ['duplicate', s.duplicate], ['overlap', s.overlap], ['needs_enrichment', s.needsEnrichment], ['committed', s.committed]]
  const template = batch.format === templateFormat
  return counts.filter(([, n]) => n > 0).map(([value, n]) => ({ value, label: `${recordLabel(value, template)} (${n})` }))
}

export const maxFileBytes = 5 * 1024 * 1024

export type UploadKind = 'template' | 'export'

const isWorkbook = (name: string) => name.toLowerCase().endsWith('.xlsx')

/** What is wrong with a chosen file, since a dropped file skips the picker's filter. */
export function fileIssue(file: { name: string; size: number } | null, kind: UploadKind): string | undefined {
  if (!file) return 'Choose a file.'
  if (file.size === 0) return 'This file is empty.'
  if (file.size > maxFileBytes) return 'The file is larger than 5 MB.'
  const allowed = kind === 'template' ? /\.(xlsx|csv)$/i : /\.(txt|csv)$/i
  if (allowed.test(file.name)) return undefined
  return kind === 'template' ? 'Choose an .xlsx or .csv template.' : 'Choose a .txt or .csv export.'
}

export function toBase64(bytes: Uint8Array): string {
  let text = ''
  for (let i = 0; i < bytes.length; i += 0x8000) text += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  return btoa(text)
}

/** A workbook is binary, so it travels as base64. Everything else is text. */
export async function fileContent(file: File): Promise<string> {
  return isWorkbook(file.name) ? toBase64(new Uint8Array(await file.arrayBuffer())) : file.text()
}

export const useRefresh = () => useInvalidate('imports', 'subnets', 'summary')

/** Saves the workbook, or one type as CSV. */
export function useTemplateDownload() {
  return useMutation({
    meta: { success: 'Template downloaded.' },
    mutationFn: (key: string) => download(`/imports/templates/${key}`, key === 'workbook' ? 'dfipam-import-template.xlsx' : `dfipam-${key.replaceAll('_', '-')}.csv`),
  })
}
