import { useQuery } from '@tanstack/react-query'
import { FileSpreadsheet, FileText, FileType } from 'lucide-react'
import { useState } from 'react'
import { Button, Card, EmptyState, ErrorState, Field, FormError, Input, PageHeader, Select, SkeletonRows } from '../../components/ui'
import { download, get, queryString } from '../../lib/api'
import { isoDate } from '../../lib/format'
import { useEnvironments } from '../../lib/queries'
import type { ReportDefinition } from '../../lib/types'

const presets = [{ days: 7, label: '7 days' }, { days: 30, label: '30 days' }, { days: 90, label: '90 days' }, { days: 365, label: '1 year' }]

function Download({ report }: { report: ReportDefinition }) {
  const environments = useEnvironments()
  const [from, setFrom] = useState(isoDate(-30))
  const [to, setTo] = useState(isoDate())
  const [env, setEnv] = useState('')
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<unknown>(null)
  const bad = report.dated && (!from || !to || to < from)
  const get = async (format: 'csv' | 'txt' | 'xlsx' | 'pdf') => {
    setBusy(format)
    setError(null)
    try {
      await download(`/reports/${report.key}${queryString({ format, from: report.dated ? from : undefined, to: report.dated ? to : undefined, environmentId: env || undefined })}`, `${report.key}.${format}`)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(null)
    }
  }
  return (
    <Card title={report.title} subtitle={report.description}>
      <div className="space-y-4 p-4">
        {report.dated && (
          <div className="space-y-2">
            <div className="flex flex-wrap items-end gap-3">
              <Field label="From" htmlFor="rp-from"><Input id="rp-from" type="date" value={from} max={to} onChange={(e) => setFrom(e.target.value)} /></Field>
              <Field label="To" htmlFor="rp-to" error={bad ? 'The end is before the start.' : undefined}><Input id="rp-to" type="date" value={to} min={from} max={isoDate()} onChange={(e) => setTo(e.target.value)} /></Field>
            </div>
            <div className="flex flex-wrap gap-1">{presets.map((p) => <Button key={p.days} variant="ghost" className="h-7" onClick={() => { setFrom(isoDate(-p.days)); setTo(isoDate()) }}>Last {p.label}</Button>)}</div>
          </div>
        )}
        {report.environment && (
          <Field label="Environment" htmlFor="rp-env">
            <Select id="rp-env" value={env} onChange={(e) => setEnv(e.target.value)} className="max-w-xs"><option value="">All environments</option>{environments.data?.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}</Select>
          </Field>
        )}
        <div>
          <p className="mb-1.5 text-[12px] text-ink-3">Columns</p>
          <div className="flex flex-wrap gap-1">{report.columns.map((c) => <span key={c} className="rounded-[3px] bg-surface-2 px-1.5 text-[11.5px] leading-[18px] text-ink-2">{c}</span>)}</div>
        </div>
        <div className="flex flex-wrap gap-2 border-t border-line pt-4">
          <Button variant="primary" disabled={bad} busy={busy === 'xlsx'} onClick={() => void get('xlsx')}><FileSpreadsheet size={14} aria-hidden />Excel</Button>
          <Button disabled={bad} busy={busy === 'csv'} onClick={() => void get('csv')}><FileText size={14} aria-hidden />CSV</Button>
          <Button disabled={bad} busy={busy === 'txt'} onClick={() => void get('txt')}><FileText size={14} aria-hidden />Text</Button>
          <Button disabled={bad} busy={busy === 'pdf'} onClick={() => void get('pdf')}><FileType size={14} aria-hidden />PDF</Button>
        </div>
        <p className="text-[12px] text-ink-3">PDF holds up to 5,000 rows; Excel, CSV and text up to 200,000. A cut-off file says so on its last line.</p>
        <FormError error={error} />
      </div>
    </Card>
  )
}

export function ReportsPage() {
  const q = useQuery({ queryKey: ['/reports'], queryFn: () => get<ReportDefinition[]>('/reports'), staleTime: Infinity })
  const [key, setKey] = useState<string | null>(null)
  if (q.isPending) return <><PageHeader title="Reports" /><SkeletonRows rows={6} /></>
  if (q.isError) return <><PageHeader title="Reports" /><Card><ErrorState error={q.error} onRetry={() => void q.refetch()} /></Card></>
  const selected = q.data.find((r) => r.key === (key ?? q.data[0]?.key))
  return (
    <>
      <PageHeader title="Reports" description="Made when you ask, from live data. Choose a report, then download it as Excel, CSV or PDF." />
      {q.data.length === 0 ? <Card><EmptyState title="No reports available" /></Card> : (
        <div className="grid gap-4 lg:grid-cols-[300px_1fr]">
          <nav aria-label="Reports" className="h-fit rounded-md border border-line bg-surface p-1">
            {q.data.map((r) => (
              <button key={r.key} type="button" onClick={() => setKey(r.key)} aria-current={selected?.key === r.key ? 'true' : undefined}
                className={`block w-full cursor-pointer rounded-[4px] px-3 py-2 text-left ${selected?.key === r.key ? 'bg-accent-soft' : 'hover:bg-surface-2'}`}>
                <span className={`block text-[13px] font-medium ${selected?.key === r.key ? 'text-accent' : 'text-ink'}`}>{r.title}</span>
                <span className="block text-[12px] text-ink-3">{r.dated ? 'Date range' : 'Current state'}{r.environment ? ' · by environment' : ''}</span>
              </button>
            ))}
          </nav>
          {selected && <Download key={selected.key} report={selected} />}
        </div>
      )}
    </>
  )
}
