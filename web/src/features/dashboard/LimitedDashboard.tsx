import { Link } from 'react-router'
import { Card, PageHeader } from '../../components/ui'
import { useCan } from '../../lib/session'

/** A role without inventory access, such as Business Viewer. Scoped inventory isn't built yet, and nothing stands in for it. */
export function LimitedDashboard() {
  const canReport = useCan('reports:export') || useCan('reports:export_scoped')
  const link = 'block rounded-md border border-line p-3 transition-colors hover:border-accent'
  return (
    <>
      <PageHeader title="Dashboard" description="What your role gives you." />
      <Card title="Your access">
        <div className="grid gap-2.5 p-3 sm:grid-cols-2">
          {canReport && <Link to="/reports" className={link}><span className="block font-medium text-ink">Reports</span><span className="block text-[12.5px] text-ink-2">Run and download the reports open to your role.</span></Link>}
          <Link to="/alerts" className={link}><span className="block font-medium text-ink">Alerts and Notifications</span><span className="block text-[12.5px] text-ink-2">Your alerts and how you receive them.</span></Link>
        </div>
        <p className="border-t border-line px-4 py-2 text-[12.5px] text-ink-3">Inventory views limited to your business services aren't available yet.</p>
      </Card>
    </>
  )
}
