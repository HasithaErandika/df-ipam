import { useQuery } from '@tanstack/react-query'
import { Download } from 'lucide-react'
import { useSearchParams } from 'react-router'
import { Badge, Button, Card, EmptyState, ErrorState, Modal, Mono, SkeletonRows, Td, Th } from '../../components/ui'
import { get } from '../../lib/api'
import { useTemplateDownload } from './shared'
import type { TemplateInfo } from './shared'

const steps = [
  ['Download', 'Get the workbook, with a sheet per record type, or one type as CSV.'],
  ['Fill it in', 'Sheets run parents first. Refer to other records by name or code; write a name used twice with its parent, such as DF/Default. Rows starting with # are skipped.'],
  ['Upload', 'Every row is checked and shown. Nothing reaches the inventory yet.'],
  ['Approve', 'Someone else approves. Imports only add: a row that already exists is skipped, never changed.'],
]

function ColumnsModal({ template, onClose }: { template: TemplateInfo; onClose: () => void }) {
  const save = useTemplateDownload()
  return (
    <Modal open onClose={onClose} size="lg" title={template.title} subtitle="The columns of this template."
      footer={<Button busy={save.isPending} onClick={() => save.mutate(template.key)}><Download size={14} aria-hidden /> Download CSV</Button>}>
      <div className="overflow-x-auto rounded-[5px] border border-line">
        <table className="w-full border-collapse">
          <thead><tr><Th>Column</Th><Th>Allowed values</Th><Th>Refers to</Th></tr></thead>
          <tbody>{template.columns.map((c) => (
            <tr key={c.name}>
              <Td className="whitespace-nowrap"><Mono>{c.name}</Mono>{c.required && <span className="ml-1.5"><Badge tone="accent">Required</Badge></span>}</Td>
              <Td className="text-[12.5px] text-ink-2">{c.options?.join(', ') ?? '—'}</Td>
              <Td className="text-ink-2">{c.refersTo ?? '—'}</Td>
            </tr>
          ))}</tbody>
        </table>
      </div>
    </Modal>
  )
}

export function Templates() {
  const [params, setParams] = useSearchParams()
  const save = useTemplateDownload()
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['imports', 'templates'], staleTime: Infinity, queryFn: () => get<TemplateInfo[]>('/imports/templates') })
  const setType = (key: string | null) => setParams((prev) => { const n = new URLSearchParams(prev); if (key) n.set('type', key); else n.delete('type'); return n })
  const open = data?.find((t) => t.key === params.get('type'))
  return (
    <div className="space-y-3">
      <Card title="How a template import works" action={<Button variant="primary" className="h-7" busy={save.isPending && save.variables === 'workbook'} onClick={() => save.mutate('workbook')}><Download size={14} aria-hidden /> Download Excel workbook</Button>}>
        <ol className="grid gap-4 p-4 md:grid-cols-4">{steps.map(([title, body], i) => (
          <li key={title}><p className="font-medium text-ink">{i + 1}. {title}</p><p className="mt-0.5 text-[12.5px] text-ink-2">{body}</p></li>
        ))}</ol>
        <p className="border-t border-line px-4 py-2 text-[12.5px] text-ink-3">Files are limited to 5 MB and 10,000 rows. Users, roles and discovery settings can't be imported, because they grant access or start traffic.</p>
      </Card>
      <Card title="Record types">
        {isPending && <SkeletonRows rows={8} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data?.length === 0 && <EmptyState title="No templates" />}
        {data && data.length > 0 && (
          <table className="w-full border-collapse">
            <thead><tr><Th>Type</Th><Th>Required columns</Th><Th className="text-right">Columns</Th><Th /></tr></thead>
            <tbody>{data.map((t) => (
              <tr key={t.key}>
                <Td><button type="button" className="cursor-pointer font-medium text-accent hover:underline" onClick={() => setType(t.key)}>{t.title}</button></Td>
                <Td className="font-mono text-[12px] text-ink-2">{t.columns.filter((c) => c.required).map((c) => c.name).join(', ')}</Td>
                <Td className="text-right tabular-nums">{t.columns.length}</Td>
                <Td className="text-right"><Button variant="ghost" className="h-7" aria-label={`Download ${t.title} CSV`} busy={save.isPending && save.variables === t.key} onClick={() => save.mutate(t.key)}><Download size={14} aria-hidden /> CSV</Button></Td>
              </tr>
            ))}</tbody>
          </table>
        )}
      </Card>
      {open && <ColumnsModal template={open} onClose={() => setType(null)} />}
    </div>
  )
}
