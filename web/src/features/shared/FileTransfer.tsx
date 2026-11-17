import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useMutation } from '@tanstack/react-query'
import { Download, FileSpreadsheet, FileText, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { Button, FormError, Modal } from '../../components/ui'
import { download, queryString, upload } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import type { ImportResult } from '../../lib/types'
import { ImportPreview } from './ImportPreview'

const item = 'flex h-8 cursor-pointer items-center gap-2 rounded-[4px] px-2.5 text-[13px] text-ink-2 outline-none data-[highlighted]:bg-bg data-[highlighted]:text-ink'

/** Downloads a list as Excel, CSV or tab-separated text, with the filters on screen. */
export function ExportMenu({ path, params = {}, name, label = 'Export' }: { path: string; params?: Record<string, string>; name: string; label?: string }) {
  const [busy, setBusy] = useState(false)
  const get = async (format: 'csv' | 'txt' | 'xlsx') => {
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
          <DropdownMenu.Item className={item} onSelect={() => void get('txt')}><FileText size={14} aria-hidden />Text (.txt)</DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

/**
 * Imports rows from a CSV, text or Excel file in three steps: choose the file,
 * review exactly what would be added and skipped (a dry run that saves
 * nothing), then confirm. The header row names the columns.
 */
export function ImportButton({ path, title, columns, required, refreshKeys, notes, keyLabel = 'Key' }: {
  path: string; title: string; columns: string[]; required: string; refreshKeys: string[]; notes?: React.ReactNode; keyLabel?: string
}) {
  const [open, setOpen] = useState(false)
  const [file, setFile] = useState<File | null>(null)
  const input = useRef<HTMLInputElement>(null)
  const refresh = useInvalidate(...refreshKeys)
  const dry = `${path}${path.includes('?') ? '&' : '?'}dryRun=true`
  const preview = useMutation({ mutationFn: (f: File) => upload<ImportResult>(dry, f), meta: { inlineError: true } })
  const send = useMutation({
    mutationFn: (f: File) => upload<ImportResult>(path, f),
    meta: { inlineError: true, success: (_: File, r: ImportResult) => `Imported ${r.imported} row${r.imported === 1 ? '' : 's'}.` },
    onSuccess: () => void refresh(),
  })
  const close = () => { setOpen(false); setFile(null); preview.reset(); send.reset() }
  const choose = (f: File | null) => { setFile(f); preview.reset() }
  const done = send.data
  const checked = preview.data
  let footer
  if (done) footer = <Button variant="primary" onClick={close}>Done</Button>
  else if (checked) {
    footer = <><Button onClick={() => preview.reset()}>Back</Button>
      <Button variant="primary" disabled={checked.imported === 0} busy={send.isPending} onClick={() => file && send.mutate(file)}>
        Import {checked.imported} {checked.imported === 1 ? 'row' : 'rows'}
      </Button></>
  } else {
    footer = <><Button onClick={close}>Cancel</Button><Button variant="primary" disabled={!file} busy={preview.isPending} onClick={() => file && preview.mutate(file)}>Review</Button></>
  }
  return (
    <>
      <Button onClick={() => setOpen(true)}><Upload size={14} aria-hidden />Import</Button>
      <Modal open={open} onClose={close} title={title} size="lg" subtitle={done ? 'Imported.' : checked ? 'Check what will happen, then import.' : 'Step 1 of 2: choose a file.'} footer={footer}>
        {done ? <ImportPreview result={done} keyLabel={keyLabel} />
          : checked ? <div className="space-y-3"><ImportPreview result={checked} keyLabel={keyLabel} /><FormError error={send.error} /></div>
            : (
              <div className="space-y-4">
                <p className="text-ink-2">Upload a <strong>CSV</strong>, <strong>text (.txt, .tsv)</strong> or <strong>Excel (.xlsx)</strong> file, up to 5 MB and 5,000 rows. Text files may use commas, tabs, semicolons or pipes. The first row names the columns; only <code className="font-mono">{required}</code> is required.</p>
                <div className="rounded-md bg-surface-2 px-3 py-2 font-mono text-[12px] text-ink-2">{columns.join(', ')}</div>
                {notes && <p className="text-[12.5px] text-ink-3">{notes}</p>}
                <button type="button" onClick={() => input.current?.click()}
                  className="flex w-full cursor-pointer flex-col items-center gap-1.5 rounded-md border-2 border-dashed border-line-strong px-4 py-8 text-center hover:border-accent hover:bg-accent-soft/30">
                  <FileSpreadsheet size={22} className="text-ink-3" aria-hidden />
                  <span className="font-medium text-ink">{file ? file.name : 'Choose a file'}</span>
                  <span className="text-[12px] text-ink-3">{file ? `${(file.size / 1024).toFixed(0)} KB` : '.csv, .txt, .tsv or .xlsx'}</span>
                </button>
                <input ref={input} type="file" className="sr-only" accept=".csv,.txt,.tsv,.xlsx,text/csv,text/plain,text/tab-separated-values,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                  onChange={(e) => choose(e.target.files?.[0] ?? null)} />
                <FormError error={preview.error} />
              </div>
            )}
      </Modal>
    </>
  )
}
