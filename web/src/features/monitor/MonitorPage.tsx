import { Activity, AlertTriangle, BellRing, Cable, Gauge, HeartPulse, Inbox, KeyRound, MonitorSmartphone, Network, Radar, ServerCrash, ShieldQuestion, Siren, Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { ErrorState, SkeletonRows } from '../../components/ui'
import { formatAgo, formatCount } from '../../lib/format'
import { AlertsDonut, Issues, Panel, Pools, ServerHealth, ServersDown, Tile, Traffic } from './MonitorPanels'
import { BusiestNetworks, NetworksByKind } from './NetworksPanel'
import { MonitorHeader } from './MonitorHeader'
import { LinksSummary, WaitingFor } from './MorePanels'
import { useClock, useMonitorData } from './useMonitorData'

/**
 * The monitor view, for a TV on the wall: everything that needs attention,
 * in large type, refreshing on its own. Full screen hides the browser too.
 */
export function MonitorPage() {
  const navigate = useNavigate()
  const now = useClock()
  const m = useMonitorData()
  const [full, setFull] = useState(Boolean(document.fullscreenElement))
  useEffect(() => {
    const change = () => setFull(Boolean(document.fullscreenElement))
    document.addEventListener('fullscreenchange', change)
    return () => document.removeEventListener('fullscreenchange', change)
  }, [])
  const toggleFull = () => void (document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen()).catch(() => undefined)
  const d = m.dashboard.data
  const fw = m.firewall.data
  const urgent = d ? d.alerts.filter((a) => a.severity === 'critical' || a.severity === 'high').reduce((n, a) => n + a.count, 0) : 0
  const down = d ? d.servers.reduce((n, s) => n + s.down, 0) : 0
  const tunnelsDown = fw ? fw.tunnels.filter((t) => t.phases > 0 && t.phaseUp === 0).length : 0
  const linesDown = fw ? fw.interfaces.filter((i) => i.link && !i.up).length : 0
  const expiring = m.links.data?.renewals?.filter((r) => r.daysLeft <= 30).length ?? 0

  const segs = m.segments.data ?? []
  const waiting = d ? d.totals.pendingDevices + d.totals.pendingRequests + d.totals.expiringReservations : 0
  const problems = [
    { n: urgent, text: `urgent alert${urgent === 1 ? '' : 's'}` },
    { n: down, text: `server${down === 1 ? '' : 's'} down` },
    { n: linesDown, text: `line${linesDown === 1 ? '' : 's'} down` },
    { n: tunnelsDown, text: `tunnel${tunnelsDown === 1 ? '' : 's'} down` },
  ]
  return (
    <div className="fixed inset-0 z-50 overflow-y-auto bg-bg">
      <div className="mx-auto max-w-[1920px] space-y-4 p-5">
        <MonitorHeader now={now} loading={!d} full={full} onFull={toggleFull} problems={problems}
          onClose={() => { if (document.fullscreenElement) void document.exitFullscreen(); navigate('/') }} />

        {m.dashboard.isError && <ErrorState error={m.dashboard.error} onRetry={() => void m.dashboard.refetch()} />}
        {!d ? <SkeletonRows rows={10} /> : (
          <>
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4 2xl:grid-cols-7">
              <Tile icon={Siren} label="Urgent alerts" value={urgent} state={urgent ? 'danger' : 'ok'} hint={urgent ? 'High or Critical, open' : 'None High or Critical'} />
              <Tile icon={ServerCrash} label="Servers down" value={down} state={down ? 'danger' : 'ok'} hint={`${formatCount(d.servers.reduce((n, s) => n + s.total, 0))} monitored`} />
              <Tile icon={ShieldQuestion} label="Unknown devices" value={formatCount(d.totals.unknownDevices)} state={d.totals.unknownDevices ? 'warn' : 'ok'} hint="Seen, never approved" />
              <Tile icon={MonitorSmartphone} label="Devices to review" value={formatCount(d.totals.pendingDevices)} state={d.totals.pendingDevices ? 'warn' : 'ok'} hint={`${formatCount(d.totals.online)} online now`} />
              {fw && <Tile icon={KeyRound} label="VPN users online" value={formatCount(fw.vpnUsers.length)} state="neutral" hint={`${fw.vpnUsers.filter((u) => u.kind === 'ssl').length} SSL · ${fw.vpnUsers.filter((u) => u.kind === 'ipsec').length} IPsec`} />}
              {m.links.data
                ? <Tile icon={Cable} label="Network links" value={`${m.links.data.totals.active}/${m.links.data.totals.total}`} state={tunnelsDown + linesDown ? 'danger' : 'ok'}
                    hint={tunnelsDown + linesDown ? `${tunnelsDown + linesDown} down on the firewall` : 'In service'} />
                : <Tile icon={Sparkles} label="New in the last hour" value={formatCount(d.totals.newLastHour)} state="neutral" hint="Devices first seen" />}
              <Tile icon={Inbox} label="Waiting for someone" value={formatCount(waiting)} state={waiting ? 'warn' : 'ok'} hint={expiring ? `${expiring} contracts within 30 days` : 'Decisions, requests, holds'} />
            </div>

            <div className="grid gap-4 xl:grid-cols-12">
              {m.canNetwork && <Panel icon={Network} title="Networks" note={`${formatCount(segs.length)} segments`} className="xl:col-span-5"><NetworksByKind segments={segs} /></Panel>}
              <Panel icon={BellRing} title="Open alerts" note="by severity" className="xl:col-span-3"><AlertsDonut d={d} /></Panel>
              <Panel icon={HeartPulse} title="Server health" note="by environment" className={m.canNetwork ? 'xl:col-span-4' : 'xl:col-span-9'}><ServerHealth d={d} /></Panel>
            </div>

            <div className="grid gap-4 xl:grid-cols-12">
              <Panel icon={Activity} title="WAN traffic" note={fw ? `${fw.firewall} · ${formatAgo(fw.at)}` : 'last 15 minutes'} className="xl:col-span-6">
                {fw ? <Traffic points={m.traffic} live={fw} /> : <p className="py-4 text-[13px] text-ink-3">{m.canLinks ? 'No firewall connected. Connect the FortiGate in System > FortiGate to see live traffic.' : 'Live traffic needs the See network links permission.'}</p>}
              </Panel>
              <Panel icon={AlertTriangle} title="Needs attention" note={m.alerts.data ? `${m.alerts.data.meta.total} open` : undefined} className="xl:col-span-3"><Issues alerts={m.alerts.data?.data ?? []} /></Panel>
              <Panel icon={ServerCrash} title="Servers down" className="xl:col-span-3"><ServersDown rows={m.down.data?.data ?? []} /></Panel>
            </div>

            <div className="grid gap-4 xl:grid-cols-12">
              {m.links.data && <Panel icon={Cable} title="Network links" note="by status and type" className="xl:col-span-4"><LinksSummary l={m.links.data} fw={fw} settings={m.linkSettings.data} /></Panel>}
              <Panel icon={Inbox} title="Waiting for someone" className={m.links.data ? 'xl:col-span-4' : 'xl:col-span-6'}><WaitingFor d={d} l={m.links.data} /></Panel>
              <Panel icon={Gauge} title="DHCP pool use" note="fullest first" className={m.links.data ? 'xl:col-span-4' : 'xl:col-span-6'}><Pools d={d} /></Panel>
            </div>
            {m.canNetwork && <Panel icon={Radar} title="Busiest networks" note="alerts first, then devices online"><BusiestNetworks segments={segs} /></Panel>}
          </>
        )}
      </div>
    </div>
  )
}
