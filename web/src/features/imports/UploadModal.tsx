import { useMutation } from '@tanstack/react-query'
import { Upload } from 'lucide-react'
import { useState } from 'react'
import { RoutingDomainSelect } from '../../components/SearchSelect'
import { Button, Field, FormError, Modal, Select } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import { ApiError, post } from '../../lib/api'
import { environments } from '../../lib/format'
import { choose, issues as check } from '../../lib/validation'
import { formats, useRefresh } from './shared'
import type { Batch } from './shared'

export function UploadModal({ onClose, onDone }: { onClose: () => void; onDone: (id: string) => void }) {
  const refresh = useRefresh()
  const [file, setFile] = useState<File | null>(null)
  const [format, setFormat] = useState('prod_subnets')
  const [routingDomainId, setRd] = useState('')
  const [environment, setEnvironment] = useState('production')
  const upload = useMutation({ meta: { success: 'File uploaded and checked.', inlineError: true },
    mutationFn: async () => post<Batch>('/imports', { fileName: file?.name ?? '', format, routingDomainId, environment, content: file ? await file.text() : '' }),
    onSuccess: (b) => { refresh(); onClose(); onDone(b.id) },
  })
  const checks = useChecks(check({
    content: !file ? 'Choose a file.' : file.size > 5 * 1024 * 1024 ? 'The file is larger than 5 MB.' : undefined,
    routingDomainId: choose(routingDomainId, 'a routing domain'),
  }))
  const errors: Record<string, string> = {}
  if (upload.error instanceof ApiError) for (const d of upload.error.details) errors[d.field] = d.issue
  Object.assign(errors, checks.shown)
  return (
    <Modal open onClose={onClose} title="Upload an export" subtitle="Every line is kept and checked. Nothing reaches the inventory until someone else approves the batch."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={upload.isPending} onClick={() => checks.submit(() => upload.mutate())}><Upload size={14} aria-hidden /> Upload and check</Button></>}>
      <div className="space-y-3.5">
        <Field label="File" htmlFor="imp-file" error={errors.content ?? errors.fileName} hint="Plain text, at most 5 MB.">
          <input id="imp-file" type="file" accept=".txt,.csv,text/plain" onChange={(e) => setFile(e.target.files?.[0] ?? null)} className="block text-[13px]" />
        </Field>
        <Field label="Format" htmlFor="imp-format" error={errors.format}>
          <Select id="imp-format" value={format} onChange={(e) => setFormat(e.target.value)}>{Object.entries(formats).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
        </Field>
        <Field label="Routing domain" htmlFor="imp-rd" error={errors.routingDomainId}>
          <RoutingDomainSelect id="imp-rd" value={routingDomainId} onChange={setRd} />
        </Field>
        <Field label="Environment" htmlFor="imp-env" error={errors.environment}>
          <Select id="imp-env" value={environment} onChange={(e) => setEnvironment(e.target.value)}>{Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
        </Field>
        {Object.keys(errors).length === 0 && <FormError error={upload.error} />}
      </div>
    </Modal>
  )
}
