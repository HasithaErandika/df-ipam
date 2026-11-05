import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { ApiError, patchPlain, post } from '../../lib/api'
import { RefSelect as SearchRef, SearchSelect, timezones } from '../SearchSelect'
import { Button, Field, FormError, Input, Modal, Select, useConfirmSave } from '../ui'
import { useChecks } from '../workflow'
import { checkForm, toBody, toForm } from './form'
import type { EntityDef, FieldDef, Row } from './types'

function RefSelect({ field, id, value, onChange, invalid }: { field: FieldDef; id: string; value: string; onChange: (v: string) => void; invalid?: boolean }) {
  return (
    <SearchRef<Row> id={id} aria-invalid={invalid} value={value} onChange={onChange} path={field.ref!} emptyLabel={field.required ? undefined : 'None'}
      toOption={(o) => ({ value: o.id, label: field.refLabel ? field.refLabel(o) : o.name ?? o.email })} />
  )
}

export function EditModal({ def, row, onClose }: { def: EntityDef; row: Row | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const editing = row !== null
  const [form, setForm] = useState(() => toForm(def, row))
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: `${def.singular} saved.`, inlineError: true },
    mutationFn: () => {
      const body = toBody(def, form, editing)
      return editing ? patchPlain(`${def.path}/${row.id}`, body) : post(def.path, body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['catalog', def.key] })
      queryClient.invalidateQueries({ queryKey: ['options', def.path] })
      onClose()
    },
  })
  const checks = useChecks(checkForm(def, form, editing))
  const errors: Record<string, string> = {}
  if (save.error instanceof ApiError) for (const d of save.error.details) errors[d.field] = d.issue
  Object.assign(errors, checks.shown)
  const set = (key: string, v: string | boolean) => setForm((f) => ({ ...f, [key]: v }))
  return (
    <Modal open onClose={onClose} title={`${editing ? 'Edit' : 'New'} ${def.singular}`}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form={`form-${def.key}`} busy={save.isPending}>{editing ? 'Save changes' : `Create ${def.singular}`}</Button></>}>
      <form id={`form-${def.key}`} noValidate onSubmit={(e) => { e.preventDefault(); checks.submit(() => editing ? confirmSave(`this ${def.singular.toLowerCase()}`, () => save.mutate()) : save.mutate()) }} className="space-y-3.5">
        {def.fields.map((f) => {
          const id = `${def.key}-${f.key}`
          const label = `${f.label}${f.required ? '' : ' (optional)'}`
          const disabled = editing && f.createOnly
          if (f.type === 'bool') {
            return (
              <label key={f.key} className="flex items-center gap-2 text-[13px] text-ink">
                <input type="checkbox" checked={Boolean(form[f.key])} onChange={(e) => set(f.key, e.target.checked)} /> {f.label}
              </label>
            )
          }
          return (
            <Field key={f.key} label={label} htmlFor={id} error={errors[f.key]} hint={disabled ? 'Fixed once created.' : f.hint}>
              {f.type === 'ref' ? (
                disabled ? <Input id={id} disabled value={row?.[`${f.key.replace(/Id$/, '')}Name`] ?? String(form[f.key])} />
                  : <RefSelect field={f} id={id} value={String(form[f.key])} onChange={(v) => set(f.key, v)} invalid={Boolean(errors[f.key])} />
              ) : f.type === 'timezone' ? (
                <SearchSelect id={id} value={String(form[f.key])} onChange={(v) => set(f.key, v)} options={timezones()} emptyLabel="Default" />
              ) : f.type === 'select' ? (
                <Select id={id} value={String(form[f.key])} disabled={disabled} onChange={(e) => set(f.key, e.target.value)}>
                  <option value="">{f.required ? 'Select' : 'None'}</option>
                  {Object.entries(f.options ?? {}).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
                </Select>
              ) : (
                <Input id={id} disabled={disabled} aria-invalid={Boolean(errors[f.key])}
                  type={f.type === 'number' ? 'number' : f.type === 'date' ? 'date' : f.type === 'datetime' ? 'datetime-local' : 'text'}
                  min={f.min} max={f.max}
                  className={f.type === 'cidr' || f.type === 'ip' ? 'font-mono' : undefined} spellCheck={f.type === 'text'}
                  value={String(form[f.key])} onChange={(e) => set(f.key, e.target.value)} />
              )}
            </Field>
          )
        })}
        {Object.keys(errors).length === 0 && <FormError error={save.error} />}
      </form>
    </Modal>
  )
}
