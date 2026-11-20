import { Paperclip, Plus, X } from 'lucide-react'
import { useRef, useState } from 'react'
import { Button, Select } from '../../components/ui'
import { commercialFiles, fileCategories, formatSize } from './labels'

export interface PendingFile { file: File; category: string }

const accept = '.pdf,.png,.jpg,.jpeg,.docx,.xlsx,.pptx,.vsdx,.txt,.csv,.cfg,.conf'
export const maxFileBytes = 15 * 1024 * 1024

/** Files chosen in the link dialog, uploaded to the link once it is saved. */
export function PendingFiles({ files, onChange, canMoney }: { files: PendingFile[]; onChange: (f: PendingFile[]) => void; canMoney: boolean }) {
  const input = useRef<HTMLInputElement>(null)
  const [category, setCategory] = useState('diagram')
  const categories = Object.entries(fileCategories).filter(([k]) => canMoney || !commercialFiles.includes(k))
  return (
    <section className="rounded-md border border-line bg-surface">
      <header className="flex flex-wrap items-center justify-between gap-2 border-b border-line px-4 py-2.5">
        <div>
          <h2 className="text-[13px] font-semibold text-ink">Attachments</h2>
          <p className="text-[12px] text-ink-3">Contracts, quotations, diagrams and configurations without secrets, up to 15 MB each. Uploaded when you save.</p>
        </div>
        <div className="flex items-center gap-2">
          <Select aria-label="Kind of document" className="w-52" value={category} onChange={(e) => setCategory(e.target.value)}>
            {categories.map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
          <Button onClick={() => input.current?.click()}><Plus size={14} aria-hidden />Add files</Button>
          <input ref={input} type="file" multiple accept={accept} className="hidden"
            onChange={(e) => { onChange([...files, ...Array.from(e.target.files ?? []).map((file) => ({ file, category }))]); e.target.value = '' }} />
        </div>
      </header>
      <div className="p-4">
        {files.length === 0 ? <p className="text-[13px] text-ink-3">No files chosen.</p> : (
          <ul className="divide-y divide-line">
            {files.map((f, i) => (
              <li key={i} className="flex flex-wrap items-center justify-between gap-2 py-1.5 text-[13px]">
                <span className="flex min-w-0 items-center gap-2"><Paperclip size={14} aria-hidden className="shrink-0 text-ink-3" />
                  <span className="truncate text-ink">{f.file.name}</span>
                  <span className="text-[12px] text-ink-3">{fileCategories[f.category]} · {formatSize(f.file.size)}</span>
                  {f.file.size > maxFileBytes && <span className="text-[12px] text-danger">Over 15 MB</span>}
                </span>
                <Button variant="ghost" aria-label={`Remove ${f.file.name}`} onClick={() => onChange(files.filter((_, j) => j !== i))}><X size={14} /></Button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}
