import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Play } from 'lucide-react'
import { Button, Card, FormError } from '../../components/ui'
import { get, post } from '../../lib/api'
import { formatAgo, formatDateTime } from '../../lib/format'
import { useCan } from '../../lib/session'
import type { LastRun, RunOutcome } from '../../lib/types'

const plural = (n: number) => `${n} ${n === 1 ? 'finding' : 'findings'}`

/** Says what a run changed. A repeat run with no change says so, not the same total. */
export function runMessage(r: RunOutcome): string {
  if (r.new === 0 && r.resolved === 0) return `Nothing changed. ${plural(r.open)} still need review.`
  return `${r.new} new, ${r.resolved} resolved. ${plural(r.open)} need review.`
}

function lastRunText(last: LastRun | undefined): string {
  if (!last?.ranAt) return 'Not run yet. It runs by itself every hour.'
  const who = last.ranBy ? `by ${last.ranBy}` : 'automatically'
  return `Last run ${formatAgo(last.ranAt)}, ${who}. It runs by itself every hour.`
}

/** The queue's headline: how much needs review, when it was checked, and Run now. */
export function RunPanel({ needsReview }: { needsReview: number | undefined }) {
  const queryClient = useQueryClient()
  const canRun = useCan('discovery:run')
  const last = useQuery({ queryKey: ['findings', 'last-run'], queryFn: () => get<LastRun>('/reconciliation/last-run') })
  const run = useMutation({ meta: { success: (_: unknown, r: RunOutcome) => runMessage(r), inlineError: true },
    mutationFn: () => post<RunOutcome>('/reconciliation/run'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['findings'] })
      queryClient.invalidateQueries({ queryKey: ['summary'] })
    },
  })
  return (
    <Card className="mb-3">
      <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
        <div>
          <p className="text-[15px] font-semibold text-ink">
            {needsReview === undefined ? 'Findings' : needsReview === 0 ? 'Nothing needs review' : `${plural(needsReview)} ${needsReview === 1 ? 'needs' : 'need'} review`}
          </p>
          <p className="text-[12.5px] text-ink-2" title={last.data?.ranAt ? formatDateTime(last.data.ranAt) : undefined}>{lastRunText(last.data)}</p>
        </div>
        {canRun && <Button variant="primary" busy={run.isPending} onClick={() => run.mutate()}><Play size={14} aria-hidden /> {run.isPending ? 'Comparing' : 'Run now'}</Button>}
      </div>
      {run.error ? <div className="border-t border-line p-3"><FormError error={run.error} /></div> : null}
    </Card>
  )
}
