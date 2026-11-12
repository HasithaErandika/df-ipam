import { PageHeader } from '../../components/ui'
import { AuditTab } from './AuditTab'

export function AuditPage() {
  return (
    <>
      <PageHeader title="Audit log" description="Every change, who made it and when, with what it was before and after. Nothing here can be edited." />
      <AuditTab />
    </>
  )
}
