import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-react'
import { type PageMeta } from '../../lib/api'
import { pageWindow } from '../../lib/format'

const pageSizes = [25, 50, 100, 200]

const pageButton = 'grid h-7 min-w-7 cursor-pointer place-items-center rounded-[4px] px-1 text-[12.5px] tabular-nums text-ink-2 hover:bg-surface-2 hover:text-ink disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent'

export function Pagination({ meta, onPage, onPageSize }:
  { meta: PageMeta; onPage: (page: number) => void; onPageSize?: (size: number) => void }) {
  const { page, pageSize, total, totalPages, hasNext, hasPrev } = meta
  const first = total === 0 ? 0 : (page - 1) * pageSize + 1
  return (
    <nav aria-label="Pagination" className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-t border-line px-3 py-1.5 text-[12.5px] text-ink-2">
      <div className="flex items-center gap-4">
        <span className="tabular-nums">{first.toLocaleString('en-GB')}–{Math.min(page * pageSize, total).toLocaleString('en-GB')} of {total.toLocaleString('en-GB')}</span>
        {onPageSize && (
          <label className="flex items-center gap-1.5">
            Rows
            <select aria-label="Rows per page" value={pageSize} onChange={(e) => onPageSize(Number(e.target.value))}
              className="h-7 cursor-pointer rounded-[4px] border border-line bg-surface px-1 text-[12.5px] text-ink focus:border-accent focus:outline-none">
              {pageSizes.map((size) => <option key={size} value={size}>{size}</option>)}
            </select>
          </label>
        )}
      </div>
      <div className="flex items-center gap-0.5">
        <button type="button" className={pageButton} aria-label="First page" disabled={!hasPrev} onClick={() => onPage(1)}><ChevronsLeft size={15} /></button>
        <button type="button" className={pageButton} aria-label="Previous page" disabled={!hasPrev} onClick={() => onPage(page - 1)}><ChevronLeft size={15} /></button>
        {pageWindow(page, totalPages).map((p, i) => p === null
          ? <span key={`gap-${i}`} aria-hidden className="w-6 text-center text-ink-3">…</span>
          : (
            <button key={p} type="button" aria-label={`Page ${p}`} aria-current={p === page ? 'page' : undefined} onClick={() => onPage(p)}
              className={p === page ? `${pageButton} bg-accent font-medium text-white hover:bg-accent hover:text-white` : pageButton}>{p}</button>
          ))}
        <button type="button" className={pageButton} aria-label="Next page" disabled={!hasNext} onClick={() => onPage(page + 1)}><ChevronRight size={15} /></button>
        <button type="button" className={pageButton} aria-label="Last page" disabled={!hasNext} onClick={() => onPage(totalPages)}><ChevronsRight size={15} /></button>
      </div>
    </nav>
  )
}
