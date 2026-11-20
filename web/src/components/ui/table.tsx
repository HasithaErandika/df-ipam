import type { UseQueryResult } from '@tanstack/react-query'
import { type ReactNode } from 'react'
import type { PageMeta } from '../../lib/api'
import type { Paging } from '../../lib/paging'
import { cx } from './cx'
import { EmptyState, ErrorState, SkeletonRows } from './feedback'
import { Pagination } from './Pagination'

export interface Column<T> {
  key: string
  header: ReactNode
  cell: (row: T) => ReactNode
  className?: string
  /** Column width in a fixed table, as a class such as `w-32` or `w-[18%]`. Columns without one share what is left. */
  width?: string
  /** Hidden below this width, for columns that matter less on small screens. */
  hideBelow?: 'md' | 'lg' | 'xl'
}

const hide = { md: 'max-md:hidden', lg: 'max-lg:hidden', xl: 'max-xl:hidden' }

/**
 * A table with its loading, error and empty states, and paging. Every list in the
 * app uses it, so they all behave the same way.
 */
export function DataTable<T>({ columns, rows, query, rowKey, empty, meta, paging, rowClassName, filtered, pageSizes, fixed }: {
  columns: Column<T>[]
  rows: T[]
  query: Pick<UseQueryResult, 'isPending' | 'isError' | 'error' | 'refetch' | 'isFetching' | 'isPlaceholderData'>
  rowKey: (row: T) => string
  empty: { title: string; body?: string; action?: ReactNode }
  meta?: PageMeta
  paging?: Paging
  rowClassName?: (row: T) => string | undefined
  /** When filters are on, an empty result says so instead of "nothing yet". */
  filtered?: { active: number; clear: () => void }
  /** Rows-per-page choices, when a list needs its own. */
  pageSizes?: number[]
  /** Fit the table to its container: set column widths, cut long text, never scroll sideways. */
  fixed?: boolean
}) {
  if (query.isPending) return <SkeletonRows rows={6} />
  if (query.isError) return <ErrorState error={query.error} onRetry={() => void query.refetch()} />
  if (rows.length === 0) {
    return filtered && filtered.active > 0
      ? <EmptyState title="Nothing matches these filters" body="Change or clear the filters to see more."
          action={<button type="button" className="cursor-pointer text-[13px] font-medium text-accent hover:underline" onClick={filtered.clear}>Clear filters</button>} />
      : <EmptyState {...empty} />
  }
  return (
    <div className={cx('relative', query.isPlaceholderData && 'opacity-60 transition-opacity')} aria-busy={query.isFetching}>
      <div className={fixed ? undefined : 'overflow-x-auto'}>
        <table className={cx('w-full border-collapse', fixed && 'table-fixed')}>
          <thead className="sticky top-0 z-[1]">
            <tr>
              {columns.map((c) => (
                <th key={c.key} scope="col" className={cx('border-b border-line bg-surface-2 px-3 py-2 text-left text-[11px] font-semibold tracking-[.04em] whitespace-nowrap text-ink-3 uppercase', fixed && 'truncate', fixed && c.width, c.hideBelow && hide[c.hideBelow], c.className)}>
                  {c.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={rowKey(row)} className={cx('group hover:bg-surface-2/70', rowClassName?.(row))}>
                {columns.map((c) => (
                  <td key={c.key} className={cx('border-b border-line px-3 py-2 align-middle text-[13px]', fixed && 'overflow-hidden break-words', c.hideBelow && hide[c.hideBelow], c.className)}>{c.cell(row)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {meta && paging && <Pagination meta={meta} onPage={paging.setPage} onPageSize={paging.setPageSize} pageSizes={pageSizes} />}
    </div>
  )
}

/** A muted dash for an empty cell. */
export const none = <span className="text-ink-3">—</span>

/** Two lines in one cell: a main value and a quieter one under it. */
export function Stack({ main, sub, mono }: { main: ReactNode; sub?: ReactNode; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <div className={cx('truncate text-ink', mono && 'font-mono text-[12.5px]')}>{main}</div>
      {sub ? <div className="truncate text-[12px] text-ink-3">{sub}</div> : null}
    </div>
  )
}
