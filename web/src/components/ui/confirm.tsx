import { createContext, type ReactNode, useCallback, useContext, useRef, useState } from 'react'
import { ConfirmDialog } from './dialogs'

interface Request {
  title: string
  body: string
  confirmLabel: string
  tone?: 'danger' | 'primary'
}

type Confirm = (request: Request) => Promise<boolean>

const ConfirmContext = createContext<Confirm>(() => Promise.resolve(false))

/** Holds the one dialog behind useConfirm. */
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [request, setRequest] = useState<Request | null>(null)
  const answer = useRef<(yes: boolean) => void>(() => {})
  const confirm = useCallback<Confirm>((next) => new Promise((resolve) => {
    answer.current = resolve
    setRequest(next)
  }), [])
  const close = (yes: boolean) => { setRequest(null); answer.current(yes) }
  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <ConfirmDialog open={request !== null} title={request?.title ?? ''} body={request?.body ?? ''} confirmLabel={request?.confirmLabel ?? ''}
        tone={request?.tone} onConfirm={() => close(true)} onCancel={() => close(false)} />
    </ConfirmContext.Provider>
  )
}

/** Asks a question in a dialog, and resolves to the answer. */
export const useConfirm = () => useContext(ConfirmContext)

/** Asks before saving an edit, then runs it. what names the record being changed. */
export function useConfirmSave() {
  const confirm = useConfirm()
  return async (what: string, run: () => void, body = 'The change applies straight away and is recorded in the audit log.') => {
    if (await confirm({ title: `Save changes to ${what}?`, body, confirmLabel: 'Save changes', tone: 'primary' })) run()
  }
}
