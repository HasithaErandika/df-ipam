import { useMutation } from '@tanstack/react-query'
import { Download, FileUp, Upload } from 'lucide-react'
import { type DragEvent, useState } from 'react'
import { RoutingDomainSelect } from '../../components/SearchSelect'
import { Button, Field, FormError, Modal, Select } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import { ApiError, post } from '../../lib/api'
import { environments } from '../../lib/format'
import { choose, issues as check } from '../../lib/validation'
import { fileContent, fileIssue, formats, templateFormat, useRefresh, useTemplateDownload } from './shared'
import type { Batch, UploadKind } from './shared'

const kinds: { key: UploadKind; label: string; body: string }[] = [
  { key: 'template', label: 'Template', body: 'A filled-in DF-IPAM workbook or CSV, for any record type.' },
  { key: 'export', label: 'Production export', body: 'A subnet export from production, for one routing domain.' },
]

function KindPicker({ value, onChange }: { value: UploadKind; onChange: (k: UploadKind) => void }) {
  return (
    <div role="radiogroup" aria-label="What are you uploading?" className="grid grid-cols-2 gap-2">
      {kinds.map((k) => (
        <button key={k.key} type="button" role="radio" aria-checked={value === k.key} onClick={() => onChange(k.key)}
          className={`cursor-pointer rounded-[5px] border p-2.5 text-left ${value === k.key ? 'border-accent bg-accent-soft' : 'border-line-strong hover:bg-surface-2'}`}>
          <span className="block text-[13px] font-medium text-ink">{k.label}</span>
          <span className="block text-[12.5px] text-ink-2">{k.body}</span>
        </button>
      ))}
    </div>
  )
}

function FileDrop({ id, file, accept, onChange }: { id: string; file: File | null; accept: string; onChange: (f: File | null) => void }) {
  const [over, setOver] = useState(false)
  const drop = (e: DragEvent) => { e.preventDefault(); setOver(false); onChange(e.dataTransfer.files[0] ?? null) }
  return (
    <label htmlFor={id} onDragOver={(e) => { e.preventDefault(); setOver(true) }} onDragLeave={() => setOver(false)} onDrop={drop}
      className={`flex cursor-pointer flex-col items-center rounded-[5px] border border-dashed px-4 py-5 text-center focus-within:border-accent ${over ? 'border-accent bg-accent-soft' : 'border-line-strong hover:bg-surface-2'}`}>
      <FileUp size={20} className="mb-1 text-ink-3" aria-hidden />
      {file ? <span className="text-[13px] font-medium break-all text-ink">{file.name} <span className="font-normal text-ink-3">({(file.size / 1024).toFixed(0)} KB)</span></span>
        : <span className="text-[13px] text-ink-2">Drop a file here, or <span className="font-medium text-accent">choose one</span></span>}
      <input id={id} type="file" accept={accept} className="sr-only" onChange={(e) => onChange(e.target.files?.[0] ?? null)} />
    </label>
  )
}

export function UploadModal({ onClose, onDone }: { onClose: () => void; onDone: (id: string) => void }) {
  const refresh = useRefresh()
  const template = useTemplateDownload()
  const [kind, setKind] = useState<UploadKind>('template')
  const [file, setFile] = useState<File | null>(null)
  const [format, setFormat] = useState('prod_subnets')
  const [routingDomainId, setRd] = useState('')
  const [environment, setEnvironment] = useState('production')
  const isTemplate = kind === 'template'
  const upload = useMutation({ meta: { success: 'File uploaded and checked.', inlineError: true },
    mutationFn: async () => {
      const content = file ? await fileContent(file) : ''
      const fileName = file?.name ?? ''
      return post<Batch>('/imports', isTemplate ? { fileName, format: templateFormat, content } : { fileName, format, routingDomainId, environment, content })
    },
    onSuccess: (b) => { refresh(); onDone(b.id) },
  })
  const checks = useChecks(check({ content: fileIssue(file, kind), routingDomainId: isTemplate ? undefined : choose(routingDomainId, 'a routing domain') }))
  const errors: Record<string, string> = {}
  if (upload.error instanceof ApiError) for (const d of upload.error.details) errors[d.field] = d.issue
  Object.assign(errors, checks.shown)
  const switchKind = (k: UploadKind) => { setKind(k); setFile(null); upload.reset() }
  return (
    <Modal open onClose={onClose} title="Upload a file" subtitle="Every row is kept and checked. Nothing reaches the inventory until someone else approves it."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={upload.isPending} onClick={() => checks.submit(() => upload.mutate())}><Upload size={14} aria-hidden /> Upload and check</Button></>}>
      <div className="space-y-3.5">
        <KindPicker value={kind} onChange={switchKind} />
        <Field label="File" htmlFor="imp-file" error={errors.content ?? errors.fileName} hint={isTemplate ? 'Excel (.xlsx) or CSV, at most 5 MB and 10,000 rows.' : 'Plain text, at most 5 MB.'}>
          <FileDrop id="imp-file" file={file} accept={isTemplate ? '.xlsx,.csv' : '.txt,.csv,text/plain'} onChange={(f) => { setFile(f); upload.reset() }} />
        </Field>
        {isTemplate && <Button variant="ghost" className="-ml-3" busy={template.isPending} onClick={() => template.mutate('workbook')}><Download size={14} aria-hidden /> Download the Excel template</Button>}
        {!isTemplate && <>
          <Field label="Format" htmlFor="imp-format" error={errors.format}>
            <Select id="imp-format" value={format} onChange={(e) => setFormat(e.target.value)}>{Object.entries(formats).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
          </Field>
          <Field label="Routing domain" htmlFor="imp-rd" error={errors.routingDomainId}>
            <RoutingDomainSelect id="imp-rd" value={routingDomainId} onChange={setRd} />
          </Field>
          <Field label="Environment" htmlFor="imp-env" error={errors.environment}>
            <Select id="imp-env" value={environment} onChange={(e) => setEnvironment(e.target.value)}>{Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
          </Field>
        </>}
        {Object.keys(errors).length === 0 && <FormError error={upload.error} />}
      </div>
    </Modal>
  )
}
