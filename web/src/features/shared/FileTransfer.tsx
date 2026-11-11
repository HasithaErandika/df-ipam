import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useMutation } from '@tanstack/react-query'
import { Download, FileSpreadsheet, FileText, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { Button, FormError, Modal, Notice } from '../../components/ui'
import { download, queryString, upload } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import type { ImportResult } from '../../lib/types'

const item = 'flex h-8 cursor-pointer items-center gap-2 rounded-[4px] px-2.5 text-[13px] text-ink-2 outline-none data-[highlighted]:bg-bg data-[highlighted]:text-ink'

/** Downloads a list as CSV or Excel, with the filters on screen. */
export function ExportMenu({ path, params = {}, name, label = 'Export' }: { path: string; params?: Record<string, string>; name: string; label?: string }) {
  const [busy, setBusy] = useState(false)
  const get = async (format: 'csv' | 'xlsx') => {
    setBusy(true)
    try {
      await download(`${path}${queryString({ ...params, format })}`, `${name}.${format}`)
    } finally {
      setBusy(false)
    }
  }
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild><Button busy={busy}>{!busy && <Download size={14} aria-hidden />}{label}</Button></DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={4} className="z-30 min-w-44 rounded-md border border-line-strong/70 bg-surface p-1 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]">
          <DropdownMenu.Item className={item} onSelect={() => void get('xlsx')}><FileSpreadsheet size={14} aria-hidden />Excel (.xlsx)</DropdownMenu.Item>
          <DropdownMenu.Item className={item} onSelect={() => void get('csv')}><FileText size={14} aria-hidden />CSV</DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

/**
 * Imports rows from a CSV or Excel file. The header row names the columns; rows
 * with problems are skipped and listed, so nothing half-valid gets in.
 */
export function ImportButton({ path, title, columns, required, refreshKeys, notes }: {
  path: string; title: string; columns: string[]; required: string; refreshKeys: string[]; notes?: React.ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [file, setFile] = useState<File | null>(null)
  const input = useRef<HTMLInputElement>(null)
  const refresh = useInvalidate(...refreshKeys)
  const send = useMutation({
    mutationFn: (f: File) => upload<ImportResult>(path, f),
    meta: { inlineError: true, success: (_: File, r: ImportResult) => `Imported ${r.imported} row${r.imported === 1 ? '' : 's'}.` },
    onSuccess: () => void refresh(),
  })
  const close = () => { setOpen(false); setFile(null); send.reset() }
  const result = send.data
  return (
    <>
      <Button onClick={() => setOpen(true)}><Upload size={14} aria-hidden />Import</Button>
      <Modal open={open} onClose={close} title={title} size="lg"
        footer={result ? <Button variant="primary" onClick={close}>Done</Button>
          : <><Button onClick={close}>Cancel</Button><Button variant="primary" disabled={!file} busy={send.isPending} onClick={() => file && send.mutate(file)}>Import</Button></>}>
        {result ? (
          <div className="space-y-3">
            <Notice tone={result.imported ? 'ok' : 'warn'}>{result.imported} imported, {result.skipped} skipped.</Notice>
            {result.issues.length > 0 && (
              <div className="max-h-72 overflow-y-auto rounded-md border border-line">
                <table className="w-full text-[12.5px]">
                  <thead><tr><th className="bg-surface-2 px-3 py-1.5 text-left text-ink-3">Row</th><th className="bg-surface-2 px-3 py-1.5 text-left text-ink-3">Why it was skipped</th></tr></thead>
                  <tbody>{result.issues.map((i, n) => <tr key={n} className="border-t border-line"><td className="px-3 py-1.5 tabular-nums">{i.row || '—'}</td><td className="px-3 py-1.5">{i.issue}</td></tr>)}</tbody>
                </table>
              </div>
            )}
          </div>
        ) : (
          <div className="space-y-4">
            <p className="text-ink-2">Upload a <strong>CSV</strong> or <strong>Excel (.xlsx)</strong> file, up to 5 MB and 5,000 rows. The first row names the columns; only <code className="font-mono">{required}</code> is required.</p>
            <div className="rounded-md bg-surface-2 px-3 py-2 font-mono text-[12px] text-ink-2">{columns.join(', ')}</div>
            {notes && <p className="text-[12.5px] text-ink-3">{notes}</p>}
            <button type="button" onClick={() => input.current?.click()}
              className="flex w-full cursor-pointer flex-col items-center gap-1.5 rounded-md border-2 border-dashed border-line-strong px-4 py-8 text-center hover:border-accent hover:bg-accent-soft/30">
              <FileSpreadsheet size={22} className="text-ink-3" aria-hidden />
              <span className="font-medium text-ink">{file ? file.name : 'Choose a file'}</span>
              <span className="text-[12px] text-ink-3">{file ? `${(file.size / 1024).toFixed(0)} KB` : '.csv or .xlsx'}</span>
            </button>
            <input ref={input} type="file" className="sr-only" accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
            <FormError error={send.error} />
          </div>
        )}
      </Modal>
    </>
  )
}
