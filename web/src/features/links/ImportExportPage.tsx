import { useMutation } from '@tanstack/react-query'
import { FileSpreadsheet, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { Button, Card, FormError, Notice, PageHeader, Switch } from '../../components/ui'
import { download, upload } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import type { LinkImportResult } from '../../lib/types'
import { ExportMenu } from '../shared/FileTransfer'
import { linkKeys } from './queries'

const box = 'max-h-72 overflow-y-auto rounded-md border border-line'
const th = 'sticky top-0 bg-surface-2 px-3 py-1.5 text-left text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase'

/** Bring links in from Excel in checked steps, and take them out. */
export function ImportExportPage() {
  return (
    <>
      <PageHeader title="Import / Export" description="Excel files hold one link per row. Exports respect what you may see: costs and contracts only for people with commercial access." />
      <div className="grid gap-4 xl:grid-cols-[1fr_360px]">
        <ImportCard />
        <div className="space-y-4">
          <Card title="Export">
            <div className="space-y-3 p-4 text-[13px] text-ink-2">
              <p>All links that aren't archived. Excel adds sheets for VPN subnets and related links. To export a filtered list, use Export on All Network Links.</p>
              <ExportMenu path="/network-links/export" name="network-links" label="Export all links" />
            </div>
          </Card>
          <Card title="Import template">
            <div className="space-y-3 p-4 text-[13px] text-ink-2">
              <p>An empty sheet with every column, and a second sheet that says what each accepts.</p>
              <Button onClick={() => void download('/network-links/template', 'network-links-import-template.xlsx')}><FileSpreadsheet size={14} aria-hidden />Download template</Button>
            </div>
          </Card>
        </div>
      </div>
    </>
  )
}

function ImportCard() {
  const [file, setFile] = useState<File | null>(null)
  const [update, setUpdate] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const refresh = useInvalidate(...linkKeys)
  const path = `/network-links/import${update ? '?update=true' : ''}`
  const preview = useMutation({ mutationFn: (f: File) => upload<LinkImportResult>(`${path}${update ? '&' : '?'}dryRun=true`, f), meta: { inlineError: true } })
  const send = useMutation({
    mutationFn: (f: File) => upload<LinkImportResult>(path, f),
    meta: { inlineError: true, success: (_: File, r: LinkImportResult) => `Imported: ${r.added} added, ${r.updated} updated.` },
    onSuccess: () => void refresh(),
  })
  const reset = () => { preview.reset(); send.reset() }
  const choose = (f: File | null) => { setFile(f); reset() }
  const result = send.data ?? preview.data
  return (
    <Card title="Import links" subtitle="Step 1: choose a file. Step 2: check what will happen. Step 3: import.">
      <div className="space-y-4 p-4">
        <div className="flex flex-wrap items-center gap-2">
          <input ref={input} type="file" accept=".xlsx,.csv,.txt,.tsv" className="hidden" onChange={(e) => choose(e.target.files?.[0] ?? null)} />
          <Button onClick={() => input.current?.click()}><Upload size={14} aria-hidden />{file ? 'Choose another file' : 'Choose file'}</Button>
          {file && <span className="text-[13px] text-ink-2">{file.name}</span>}
        </div>
        <Switch id="import-update" checked={update} onChange={(v) => { setUpdate(v); reset() }} label="Update existing links"
          hint="A row whose Link ID, or provider and circuit ID, matches a link changes that link. Columns the file leaves out keep their values. Off: such rows are skipped." />
        {!result && <Button variant="primary" disabled={!file} busy={preview.isPending} onClick={() => file && preview.mutate(file)}>Check the file</Button>}
        <FormError error={preview.error ?? send.error} />
        {result && <ImportReport r={result} />}
        {preview.data && !send.data && (
          <div className="flex gap-2">
            <Button onClick={reset}>Back</Button>
            <Button variant="primary" disabled={preview.data.imported === 0} busy={send.isPending} onClick={() => file && send.mutate(file)}>
              Import {preview.data.added} new{preview.data.updated ? ` and update ${preview.data.updated}` : ''}
            </Button>
          </div>
        )}
        {send.data && <Button onClick={() => { choose(null); if (input.current) input.current.value = '' }}><Upload size={14} aria-hidden />Import another file</Button>}
      </div>
    </Card>
  )
}

function ImportReport({ r }: { r: LinkImportResult }) {
  return (
    <div className="space-y-3">
      <Notice tone={r.imported ? 'ok' : 'warn'}>
        {r.dryRun ? 'If you import:' : 'Imported:'} <strong>{r.added}</strong> added, <strong>{r.updated}</strong> updated, <strong>{r.skipped}</strong> skipped.
        {r.dryRun && ' Nothing has been saved yet.'}
      </Notice>
      {r.preview.length > 0 && (
        <div className={box}>
          <table className="w-full text-[12.5px]">
            <thead><tr><th className={th}>Row</th><th className={th}>Link ID</th><th className={th}>Name</th><th className={th}>Change</th></tr></thead>
            <tbody>{r.preview.map((p) => (
              <tr key={p.row} className="border-t border-line">
                <td className="px-3 py-1.5 text-ink-3 tabular-nums">{p.row}</td><td className="px-3 py-1.5 font-mono">{p.key}</td>
                <td className="px-3 py-1.5">{p.label}</td><td className="px-3 py-1.5">{p.action === 'add' ? 'Add' : <span className="text-warn">Update</span>}</td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      )}
      {r.issues.length > 0 && (
        <div className={box}>
          <table className="w-full text-[12.5px]">
            <thead><tr><th className={th}>Row</th><th className={th}>Skipped because</th></tr></thead>
            <tbody>{r.issues.map((i, n) => <tr key={n} className="border-t border-line"><td className="px-3 py-1.5 text-ink-3 tabular-nums">{i.row}</td><td className="px-3 py-1.5">{i.issue}</td></tr>)}</tbody>
          </table>
        </div>
      )}
    </div>
  )
}
