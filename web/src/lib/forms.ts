import type { FieldValues, Path, UseFormSetError } from 'react-hook-form'
import { fieldIssues } from './api'

/**
 * Puts the API's field errors on the matching inputs. It returns whether any
 * matched, so the form can show a general message only when none did.
 */
export function applyServerErrors<T extends FieldValues>(error: unknown, setError: UseFormSetError<T>, fields: readonly string[]): boolean {
  let matched = false
  for (const [field, issue] of Object.entries(fieldIssues(error))) {
    const name = field.split('.').pop() ?? field
    const target = fields.includes(field) ? field : fields.includes(name) ? name : null
    if (target) {
      setError(target as Path<T>, { type: 'server', message: issue })
      matched = true
    }
  }
  return matched
}
