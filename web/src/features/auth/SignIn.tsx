import { useEffect, useState } from 'react'
import { LogoMark } from '../../components/LogoMark'
import { SubnetMap } from '../../components/SubnetMap'
import { Button, Notice } from '../../components/ui'
import { authStatus, signInWithMicrosoft, type AuthStatus } from '../../lib/auth'
import { takeSignOutReason } from '../../lib/idle'

/** Why Microsoft sign-in sent the person back, from /?signin_error=. */
const refused: Record<string, string> = {
  domain: 'Use your @dialogfinance.lk Microsoft account. Other accounts, including guests, can\'t sign in.',
  tenant: 'That Microsoft account belongs to another organisation. Use your Dialog Finance account.',
  disabled: 'Your DF-IPAM access has been switched off. Ask your IPAM administrator.',
  expired: 'The sign-in took too long or was opened in another tab. Try again.',
  cancelled: 'Sign-in was cancelled.',
  off: 'Microsoft sign-in isn\'t set up on this server.',
  failed: 'Microsoft sign-in didn\'t complete. Try again, and tell your IPAM administrator if it keeps happening.',
}

function takeRefusal(): string | null {
  const params = new URLSearchParams(window.location.search)
  const code = params.get('signin_error')
  if (!code) return null
  params.delete('signin_error')
  window.history.replaceState(null, '', window.location.pathname + (params.size ? `?${params}` : ''))
  return refused[code] ?? refused.failed
}

function MicrosoftMark() {
  return (
    <svg width="16" height="16" viewBox="0 0 21 21" aria-hidden>
      <rect x="1" y="1" width="9" height="9" fill="#f25022" /><rect x="11" y="1" width="9" height="9" fill="#7fba00" />
      <rect x="1" y="11" width="9" height="9" fill="#00a4ef" /><rect x="11" y="11" width="9" height="9" fill="#ffb900" />
    </svg>
  )
}

export function SignIn() {
  const [refusal] = useState(takeRefusal)
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [leaving, setLeaving] = useState(false)
  useEffect(() => { void authStatus().then(setStatus) }, [])
  const [reason] = useState(takeSignOutReason)

  const msReady = status?.microsoft === true
  const msOff = status !== null && !status.microsoft

  return (
    <div className="flex h-full">
      <div className="relative hidden flex-1 overflow-hidden bg-map lg:block">
        <SubnetMap />
        <div className="absolute inset-0 bg-gradient-to-t from-map via-map/70 to-map/10" />
        <div className="absolute inset-x-0 bottom-0 p-12">
          <p className="font-mono text-[13px] text-map-accent">192.168.0.0/16 · 172.16.0.0/12 · 10.0.0.0/8</p>
          <h2 className="mt-3 max-w-md text-3xl leading-tight font-semibold tracking-tight text-map-ink">
            Every network, server and device at Dialog Finance, in one record.
          </h2>
          <p className="mt-3 max-w-md text-[14px] text-map-ink/70">Discovery, the IP register, server checks and alerts. Read-only towards the network.</p>
        </div>
      </div>
      <main className="flex w-full flex-col bg-surface px-6 lg:w-[480px] lg:shrink-0">
        <div className="m-auto w-full max-w-sm py-10">
          <div className="mb-8 flex items-center gap-3">
            <LogoMark size={36} />
            <div className="leading-tight"><div className="text-[16px] font-semibold text-ink">DF-IPAM</div><div className="text-[12px] text-ink-3">Dialog Finance · IP Address Management</div></div>
          </div>
          <h1 className="text-[22px] font-semibold tracking-tight text-ink">Sign in</h1>
          <p className="mt-1 text-ink-2">Use your Dialog Finance Microsoft account.</p>
          {reason && <div className="mt-5"><Notice tone="warn">{reason}</Notice></div>}
          {refusal && <div className="mt-5"><Notice tone="danger">{refusal}</Notice></div>}

          <Button variant="primary" className="mt-7 h-11 w-full gap-2.5 text-[14px]" busy={leaving || status === null} disabled={msOff}
            onClick={() => { setLeaving(true); signInWithMicrosoft() }}>
            {!leaving && status !== null && <MicrosoftMark />}Sign in with Microsoft
          </Button>
          {msReady && <p className="mt-2 text-center text-[12.5px] text-ink-3">Only <strong className="font-medium text-ink-2">@dialogfinance.lk</strong> accounts can sign in.</p>}
          {msOff && <div className="mt-4"><Notice tone="warn">Sign-in isn't available: the server can't be reached, or Microsoft sign-in isn't set up. An administrator adds the Entra ID settings to <code className="font-mono">api/.env</code>.</Notice></div>}

          <div className="mt-8 rounded-md border border-line bg-surface-2 px-3.5 py-3 text-[12.5px] leading-relaxed text-ink-2">
            <strong className="font-medium text-ink">First time here?</strong> Sign in, and you'll see "Waiting for access" until your IPAM administrator gives you a role.
          </div>
        </div>
        <p className="pb-5 text-center text-[11.5px] text-ink-3">DF-IPAM Lite v{__APP_VERSION__}{__APP_COMMIT__ ? ` · ${__APP_COMMIT__}` : ''}</p>
      </main>
    </div>
  )
}
