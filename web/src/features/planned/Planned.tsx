import { CheckCircle2 } from 'lucide-react'
import { Badge, Card, PageHeader } from '../../components/ui'
import type { NavItem } from '../../app/nav'

/** Shown for pages that aren't built yet. It never displays sample data. */
export function Planned({ item }: { item: NavItem }) {
  return (
    <>
      <PageHeader title={item.label} description={item.summary} actions={<Badge tone="accent">Planned: Phase {item.phase}</Badge>} />
      <Card title="What this page will do" className="max-w-2xl">
        <ul className="space-y-2 p-4">
          {item.features?.map((f) => (
            <li key={f} className="flex gap-2 text-ink-2"><CheckCircle2 size={16} className="mt-0.5 shrink-0 text-ink-3" aria-hidden />{f}</li>
          ))}
        </ul>
      </Card>
    </>
  )
}
