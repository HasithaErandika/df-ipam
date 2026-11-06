import { AlertTriangle, Inbox } from 'lucide-react'
import { Component, type ReactNode } from 'react'
import { ApiError, errorMessage, isStaleBuild } from '../../lib/api'
import { Button } from './controls'
import { Mono } from './layout'

/** Catches a crash in a page, so people get a way forward instead of a blank screen. */
export class ErrorBoundary extends Component<{ children: ReactNode; resetKey?: string }, { failed: boolean; stale: boolean; key?: string }> {
  state = { failed: false, stale: false, key: this.props.resetKey }

  static getDerivedStateFromError(error: unknown) { return { failed: true, stale: isStaleBuild(error) } }

  // Moving to another page clears the failure.
  static getDerivedStateFromProps(props: { resetKey?: string }, state: { key?: string }) {
    return props.resetKey === state.key ? null : { failed: false, stale: false, key: props.resetKey }
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <div role="alert" className="flex flex-col items-center p-10 text-center">
        <AlertTriangle size={26} className="mb-3 text-danger" aria-hidden />
        <h1 className="text-base font-semibold text-ink">{this.state.stale ? 'A new version is available' : 'This page ran into a problem'}</h1>
        <p className="mt-1 text-ink-2">{this.state.stale ? 'Reload to get the latest version of DF-IPAM.' : 'Your data is safe. Reload the page, or go to another one.'}</p>
        <div className="mt-4"><Button variant="primary" onClick={() => window.location.reload()}>Reload</Button></div>
      </div>
    )
  }
}

export function SkeletonRows({ rows = 6 }: { rows?: number }) {
  return (
    <div className="space-y-2 p-4" aria-label="Loading" role="status">
      {Array.from({ length: rows }, (_, i) => <div key={i} className="h-6 animate-pulse rounded bg-track" />)}
    </div>
  )
}

export function EmptyState({ title, body, action }: { title: string; body?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center px-6 py-14 text-center">
      <Inbox size={26} className="mb-2.5 text-ink-3" aria-hidden />
      <p className="font-medium text-ink">{title}</p>
      {body && <p className="mt-1 max-w-md text-ink-2">{body}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const api = error instanceof ApiError ? error : undefined
  return (
    <div role="alert" className="flex flex-col items-center px-6 py-14 text-center">
      <AlertTriangle size={26} className="mb-2.5 text-danger" aria-hidden />
      <p className="font-medium text-ink">{errorMessage(error)}</p>
      {api?.correlationId && <p className="mt-1 text-[13px] text-ink-3">Reference: <Mono>{api.correlationId}</Mono></p>}
      {onRetry && <Button className="mt-4" onClick={onRetry}>Try again</Button>}
    </div>
  )
}

export function FormError({ error }: { error: unknown }) {
  if (!error) return null
  return (
    <div role="alert" className="flex gap-2 rounded-md bg-danger-soft px-3 py-2 text-danger">
      <AlertTriangle size={16} className="mt-0.5 shrink-0" aria-hidden />
      <span>{errorMessage(error)}</span>
    </div>
  )
}
