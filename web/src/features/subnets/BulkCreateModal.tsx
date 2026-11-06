import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { RoutingDomainSelect } from '../../components/SearchSelect'
import { Badge, Button, Field, FormError, Modal, Mono, Select, Td, Th } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import { post, fieldIssues } from '../../lib/api'
import { environments } from '../../lib/format'
import { choose, issues as check } from '../../lib/validation'

interface BulkResult { row: number; cidr: string; name: string; ok: boolean; code?: string; issue?: string }

interface BulkResponse { created: boolean; valid: number; invalid: number; results: BulkResult[] }

/** Parses "cidr, name, gateway" lines. Blank lines and # comments are skipped. */
function parseBulk(text: string) {
  return text.split('\n').map((l) => l.trim()).filter((l) => l && !l.startsWith('#')).map((line) => {
    const [cidr = '', name = '', gateway = ''] = line.split(/[,\t]/).map((p) => p.trim())
    return { cidr, name: name || cidr, gateway: gateway || null }
  })
}

export function BulkCreateModal({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient()
  const [routingDomainId, setRd] = useState('')
  const [environment, setEnvironment] = useState('')
  const [text, setText] = useState('')
  const [result, setResult] = useState<BulkResponse | null>(null)
  const run = useMutation({ meta: { success: (dryRun: boolean) => dryRun ? undefined : 'Subnets created.', inlineError: true },
    mutationFn: (dryRun: boolean) => post<BulkResponse>('/subnets/bulk', { routingDomainId, environment, dryRun, subnets: parseBulk(text) }),
    onSuccess: (r) => { setResult(r); if (r.created) queryClient.invalidateQueries({ queryKey: ['subnets'] }) },
  })
  const rows = parseBulk(text)
  const checks = useChecks(check({
    routingDomainId: choose(routingDomainId, 'a routing domain'), environment: choose(environment, 'an environment'),
    subnets: rows.length === 0 ? 'Add at least one subnet.' : undefined,
  }))
  const errors = { ...fieldIssues(run.error), ...checks.shown }
  return (
    <Modal open onClose={onClose} size="xl" title="Bulk create subnets" subtitle="Check first, then create. All rows are created together, or none are."
      footer={result?.created ? <Button variant="primary" onClick={onClose}>Done</Button> : <>
        <Button onClick={onClose}>Cancel</Button>
        <Button busy={run.isPending && run.variables === true} disabled={run.isPending} onClick={() => checks.submit(() => run.mutate(true))}>Check {rows.length} rows</Button>
        <Button variant="primary" busy={run.isPending && run.variables === false} disabled={run.isPending || !result || result.invalid > 0} onClick={() => run.mutate(false)}>Create {rows.length} subnets</Button>
      </>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Routing domain" htmlFor="bulk-rd" error={errors.routingDomainId}>
          <RoutingDomainSelect id="bulk-rd" value={routingDomainId} onChange={(v) => { setRd(v); setResult(null) }} />
        </Field>
        <Field label="Environment" htmlFor="bulk-env" error={errors.environment}>
          <Select id="bulk-env" value={environment} onChange={(e) => { setEnvironment(e.target.value); setResult(null) }}>
            <option value="">Select</option>
            {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </Field>
        <div className="col-span-2">
          <Field label="Subnets, one per line" htmlFor="bulk-text" hint="CIDR, name, gateway (optional). Commas or tabs, so a spreadsheet column paste works." error={errors.subnets}>
            <textarea id="bulk-text" rows={7} spellCheck={false} value={text} onChange={(e) => { setText(e.target.value); setResult(null) }}
              placeholder={'10.40.1.0/24, App tier, 10.40.1.1\n10.40.2.0/24, DB tier'}
              className="w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 py-1.5 font-mono text-[12.5px] text-ink focus:border-accent focus:bg-surface focus:outline-none" />
          </Field>
        </div>
      </div>
      {Object.keys(errors).length === 0 && <div className="mt-3"><FormError error={run.error} /></div>}
      {result && (
        <div className="mt-3">
          <p className={`mb-2 text-[13px] ${result.created ? 'text-ok' : result.invalid ? 'text-danger' : 'text-ink-2'}`}>
            {result.created ? `Created ${result.valid} subnets.` : result.invalid ? `${result.invalid} of ${result.results.length} rows need fixing. Nothing was saved.` : `All ${result.valid} rows are valid. Nothing is saved until you create them.`}
          </p>
          <div className="max-h-64 overflow-y-auto rounded-[5px] border border-line">
            <table className="w-full border-collapse">
              <thead><tr><Th>Row</Th><Th>CIDR</Th><Th>Name</Th><Th>Result</Th></tr></thead>
              <tbody>{result.results.map((r) => (
                <tr key={r.row}><Td className="tabular-nums">{r.row}</Td><Td><Mono>{r.cidr}</Mono></Td><Td>{r.name}</Td>
                  <Td>{r.ok ? <Badge tone="ok">{result.created ? 'Created' : 'Valid'}</Badge> : <span className="text-[12.5px] text-danger">{r.issue}</span>}</Td></tr>
              ))}</tbody>
            </table>
          </div>
        </div>
      )}
    </Modal>
  )
}
