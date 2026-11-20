import { Details, StateBadge } from '../../components/ui'
import { formatAgo, formatDateTime } from '../../lib/format'
import type { Finding } from '../../lib/types'
import { compliance, entraJoin, windowsName } from './windowsInfo'

/** Whether the device has told us anything about Windows or is known to Intune. */
export const hasManagement = (r: Finding) => Boolean(r.workgroup || r.windowsDomain || r.windowsFqdn || r.osVersion || r.intuneSeenAt || r.entraSeenAt)

const Sub = ({ children }: { children: string }) => <h4 className="mb-1 text-[12px] font-medium text-ink-2">{children}</h4>

/** What Windows says about itself on the network, and Intune's and Entra ID's records of the device. */
export function ManagementSection({ r }: { r: Finding }) {
  const comply = r.intuneCompliance
  const windows = Boolean(r.workgroup || r.windowsDomain || r.windowsFqdn || r.osVersion)
  return (
    <div className="space-y-3">
      {windows && (
        <div>
          <Sub>On the network</Sub>
          <Details items={[
            ['Domain or workgroup', r.windowsDomain || r.workgroup || null],
            ...(r.windowsDomain && r.workgroup && r.workgroup !== r.windowsDomain ? [['NetBIOS domain', r.workgroup] as [string, string]] : []),
            ['Full name', r.windowsFqdn || null],
            ['Windows version', r.osVersion ? <span title="From the file-sharing (SMB) handshake">{windowsName(r.osVersion)}</span> : null],
          ]} />
        </div>
      )}
      <div>
        <Sub>Microsoft Intune</Sub>
        {r.intuneSeenAt
          ? <Details items={[
              ['User', r.intuneUser ? <span>{r.intuneUserName ? `${r.intuneUserName} · ` : ''}<span className="font-mono">{r.intuneUser}</span></span> : <span className="text-ink-3">No primary user (shared device)</span>],
              ['Intune name', r.intuneDevice || null],
              ['Maker and model', [r.intuneMaker, r.intuneModel].filter(Boolean).join(' ') || null],
              ['Serial number', r.intuneSerial ? <span className="font-mono">{r.intuneSerial}</span> : null],
              ['Operating system', r.intuneOs || null],
              ['Compliance', comply ? <StateBadge tone={comply === 'compliant' ? 'ok' : comply === 'noncompliant' ? 'danger' : 'neutral'}>{compliance[comply] ?? comply}</StateBadge> : null],
              ['Disk encryption', r.intuneEncrypted == null ? null : <StateBadge tone={r.intuneEncrypted ? 'ok' : 'warn'}>{r.intuneEncrypted ? 'Encrypted' : 'Not encrypted'}</StateBadge>],
              ['Last Intune check-in', r.intuneLastSync ? <span title={formatDateTime(r.intuneLastSync)}>{formatAgo(r.intuneLastSync)}</span> : null],
            ]} />
          : <p className="text-[12.5px] text-ink-3">Not in Intune, or Intune isn't connected. A company PC should be there; a personal or unknown device won't be.</p>}
      </div>
      {r.entraSeenAt && (
        <div>
          <Sub>Microsoft Entra ID</Sub>
          <Details items={[
            ['Owner', <span key="o">{r.entraUserName ? `${r.entraUserName} · ` : ''}<span className="font-mono">{r.entraUser}</span></span>],
            ['Entra name', r.entraDevice ? <span title="Matched to this device by computer name">{r.entraDevice}</span> : null],
            ['Joined as', r.entraJoin ? entraJoin[r.entraJoin] ?? r.entraJoin : null],
            ['Last Entra sign-in', r.entraLastSignIn ? <span title={formatDateTime(r.entraLastSignIn)}>{formatAgo(r.entraLastSignIn)}</span> : null],
          ]} />
        </div>
      )}
    </div>
  )
}
