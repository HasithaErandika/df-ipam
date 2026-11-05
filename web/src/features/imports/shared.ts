import { useInvalidate } from '../../lib/queries'

interface Summary { lines: number; valid: number; invalid: number; ignored: number; duplicate: number; overlap: number; needsEnrichment: number; committed: number }

export interface Batch {
  id: string; fileName: string; fileHash: string; format: string; status: string; summary: Summary; routingDomainName: string | null; environment: string | null
  uploadedBy: string; uploadedByEmail: string; approvedByEmail: string | null; createdAt: string; committedAt: string | null; reversedAt: string | null
}

interface Parsed { cidr?: string; name?: string; gateway?: string; vdom?: string; vlan?: string; comment?: boolean }

export interface ImportRecord { id: string; batchId: string; lineNumber: number; rawText: string; parsed: Parsed; status: string; errors: string[]; createdObjectId: string | null; fileName?: string }

export const formats: Record<string, string> = {
  prod_subnets: 'Production subnets (Prod_Subnets.txt)', prod_vdoms_subnets: 'Production VDOM subnets (Prod_VDOMs_Subnets.txt)', csv: 'CSV: cidr, name, gateway, vlan',
}

export const recordTone: Record<string, 'ok' | 'danger' | 'warn' | 'neutral' | 'accent'> = {
  valid: 'ok', invalid: 'danger', duplicate: 'neutral', overlap: 'warn', needs_enrichment: 'warn', committed: 'accent',
}

export const batchTone: Record<string, 'ok' | 'danger' | 'warn' | 'neutral' | 'accent'> = {
  validated: 'warn', committed: 'ok', reversed: 'neutral', failed: 'danger', uploaded: 'neutral', approved: 'accent',
}

export const useRefresh = () => useInvalidate('imports', 'subnets', 'summary')
