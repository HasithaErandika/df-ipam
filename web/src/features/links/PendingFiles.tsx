import { FileImage, FileSpreadsheet, FileText, UploadCloud, X } from 'lucide-react'
import { useRef, useState } from 'react'
import { Button, Select } from '../../components/ui'
import { commercialFiles, fileCategories, formatSize } from './labels'

export interface PendingFile { file: File; category: string }

const accept = '.pdf,.png,.jpg,.jpeg,.docx,.xlsx,.pptx,.vsdx,.txt,.csv,.cfg,.conf'
const allowed = accept.split(',')
export const maxFileBytes = 15 * 1024 * 1024

function FileIcon({ name }: { name: string }) {
  const ext = name.slice(name.lastIndexOf('.')).toLowerCase()
  const Icon = ['.png', '.jpg', '.jpeg'].includes(ext) ? FileImage : ['.xlsx', '.csv'].includes(ext) ? FileSpreadsheet : FileText
  return <span className="grid size-8 shrink-0 place-items-center rounded-md bg-accent-soft text-accent"><Icon size={16} aria-hidden /></span>
}

/**
 * Files chosen in the link dialog, uploaded to the link once it is saved.
 * Drop files on the area or click it; each file keeps the kind chosen above.
 */
export function PendingFiles({ files, onChange, canMoney }: { files: PendingFile[]; onChange: (f: PendingFile[]) => void; canMoney: boolean }) {
  const input = useRef<HTMLInputElement>(null)
  const [category, setCategory] = useState('diagram')
  const [over, setOver] = useState(false)
  const categories = Object.entries(fileCategories).filter(([k]) => canMoney || !commercialFiles.includes(k))
  const add = (list: FileList | null) => onChange([...files, ...Array.from(list ?? []).map((file) => ({ file, category }))])
  const kindOk = (name: string) => allowed.includes(name.slice(name.lastIndexOf('.')).toLowerCase())
  return (
    <section aria-label="Attachments" className="space-y-3">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h3 className="text-[13px] font-medium text-ink">Attachments</h3>
          <p className="text-[12.5px] text-ink-3">Uploaded when you save. Remove passwords and keys from configuration files first.</p>
        </div>
        <label className="flex items-center gap-2 text-[12.5px] text-ink-2">Kind of document
          <Select className="w-52" value={category} onChange={(e) => setCategory(e.target.value)}>{categories.map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
        </label>
      </div>
      <button type="button" onClick={() => input.current?.click()}
        onDragOver={(e) => { e.preventDefault(); setOver(true) }} onDragLeave={() => setOver(false)}
        onDrop={(e) => { e.preventDefault(); setOver(false); add(e.dataTransfer.files) }}
        className={`flex w-full cursor-pointer flex-col items-center gap-1.5 rounded-lg border-2 border-dashed px-4 py-6 text-center transition-colors ${over ? 'border-accent bg-accent-soft' : 'border-line-strong bg-surface-2 hover:border-accent/60 hover:bg-accent-soft/40'}`}>
        <span className="grid size-10 place-items-center rounded-full bg-surface text-accent shadow-sm"><UploadCloud size={20} aria-hidden /></span>
        <span className="text-[13px] font-medium text-ink">Drop files here, or <span className="text-accent">choose files</span></span>
        <span className="text-[12px] text-ink-3">PDF, images, Word, Excel, PowerPoint, Visio or text · up to 15 MB each · added as {fileCategories[category].toLowerCase()}</span>
      </button>
      <input ref={input} type="file" multiple accept={accept} className="hidden" onChange={(e) => { add(e.target.files); e.target.value = '' }} />
      {files.length > 0 && (
        <ul className="space-y-1.5">
          {files.map((f, i) => {
            const problem = f.file.size > maxFileBytes ? 'Over 15 MB' : !kindOk(f.file.name) ? 'This kind of file isn’t accepted' : ''
            return (
              <li key={i} className={`flex items-center gap-3 rounded-md border px-3 py-2 ${problem ? 'border-danger/40 bg-danger-soft/40' : 'border-line bg-surface'}`}>
                <FileIcon name={f.file.name} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px] font-medium text-ink">{f.file.name}</span>
                  <span className="block text-[12px] text-ink-3">{fileCategories[f.category]} · {formatSize(f.file.size)}{problem && <span className="text-danger"> · {problem}</span>}</span>
                </span>
                <Button variant="ghost" aria-label={`Remove ${f.file.name}`} onClick={() => onChange(files.filter((_, j) => j !== i))}><X size={14} /></Button>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

/** Files that can't be uploaded, so saving can say so first. */
export const badFiles = (files: PendingFile[]) => files.filter((f) => f.file.size > maxFileBytes || !allowed.includes(f.file.name.slice(f.file.name.lastIndexOf('.')).toLowerCase()))
