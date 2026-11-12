/**
 * The last menu page someone was on, so a drill-down page (such as a segment)
 * can link back to wherever it was opened from, filters included.
 */
let last = { path: '/discovery', label: 'Discovery' }

export function rememberPage(path: string, label: string) {
  last = { path, label }
}

export const backTo = () => last
