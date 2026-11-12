import { Link } from 'react-router'
import { Field, Input, Select } from '../../components/ui'
import { useDepartments } from '../../lib/queries'

/**
 * The department, picked from the list set up in Network setup. A name saved before
 * the list existed stays selectable. With no list yet, it is typed, and the hint
 * says where to set one up.
 */
export function DepartmentSelect({ id, value, onChange, error }: { id: string; value: string; onChange: (v: string) => void; error?: string }) {
  const q = useDepartments()
  const list = q.data ?? []
  if (q.isSuccess && list.length === 0) {
    return (
      <Field label="Department" htmlFor={id} error={error} hint={<>Set up a list in <Link className="text-accent hover:underline" to="/network-setup/departments">Network setup → Departments</Link> to pick from.</>}>
        <Input id={id} value={value} maxLength={120} onChange={(e) => onChange(e.target.value)} />
      </Field>
    )
  }
  const listed = list.some((d) => d.name.toLowerCase() === value.trim().toLowerCase())
  return (
    <Field label="Department" htmlFor={id} error={error}>
      <Select id={id} value={listed ? list.find((d) => d.name.toLowerCase() === value.trim().toLowerCase())!.name : value} disabled={q.isPending}
        onChange={(e) => onChange(e.target.value)}>
        <option value="">{q.isPending ? 'Loading…' : 'Not set'}</option>
        {list.map((d) => <option key={d.id} value={d.name}>{d.name}</option>)}
        {value && !listed && <option value={value}>{value} (not in the list)</option>}
      </Select>
    </Field>
  )
}
