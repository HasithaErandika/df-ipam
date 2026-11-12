import { Notice } from '../../components/ui'
import type { ImportResult } from '../../lib/types'

const box = 'max-h-64 overflow-y-auto rounded-md border border-line'
const th = 'sticky top-0 bg-surface-2 px-3 py-1.5 text-left text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase'

/** What an import adds and what it skips, from a dry run or the real thing. */
export function ImportPreview({ result, keyLabel }: { result: ImportResult; keyLabel: string }) {
  const added = result.dryRun ? 'will be added' : 'added'
  return (
    <div className="space-y-3">
      <Notice tone={result.imported ? 'ok' : 'warn'}>
        <strong>{result.imported}</strong> {result.imported === 1 ? 'row' : 'rows'} {added}, <strong>{result.skipped}</strong> skipped.
        {result.dryRun && ' Nothing has been saved yet.'}
      </Notice>
      {result.preview.length > 0 && (
        <section>
          <h3 className="mb-1.5 text-[12.5px] font-semibold text-ink">{result.dryRun ? 'To be added' : 'Added'}</h3>
          <div className={box}>
            <table className="w-full text-[12.5px]">
              <thead><tr><th className={th}>Row</th><th className={th}>{keyLabel}</th><th className={th}>Name</th></tr></thead>
              <tbody>{result.preview.map((r) => (
                <tr key={r.key} className="border-t border-line">
                  <td className="px-3 py-1.5 text-ink-3 tabular-nums">{r.row}</td>
                  <td className="px-3 py-1.5 font-mono">{r.key}</td>
                  <td className="px-3 py-1.5">{r.label || '—'}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        </section>
      )}
      {result.issues.length > 0 && (
        <section>
          <h3 className="mb-1.5 text-[12.5px] font-semibold text-ink">Skipped</h3>
          <div className={box}>
            <table className="w-full text-[12.5px]">
              <thead><tr><th className={th}>Row</th><th className={th}>Why</th></tr></thead>
              <tbody>{result.issues.map((i, n) => <tr key={n} className="border-t border-line"><td className="px-3 py-1.5 text-ink-3 tabular-nums">{i.row || '—'}</td><td className="px-3 py-1.5">{i.issue}</td></tr>)}</tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  )
}
