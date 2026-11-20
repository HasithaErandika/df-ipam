import { Activity, BellRing, Cable, Gauge, HeartPulse, Inbox, KeyRound, MonitorSmartphone, Network, ServerCrash, ShieldQuestion, Siren, Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { ErrorState, SkeletonRows } from '../../components/ui'
import { formatAgo, formatCount } from '../../lib/format'
import { AlertsDonut, Panel, Pools, ServerHealth, Tile, Traffic } from './MonitorPanels'
import { NetworksByKind } from './NetworksPanel'
import { MonitorHeader } from './MonitorHeader'
import { LinksSummary } from './MorePanels'
import { useClock, useMonitorData } from './useMonitorData'

/**
 * The monitor view, for a TV on the wall: summary charts only, sized to one
 * screen with no scrolling, refreshing on its own. Full screen hides the browser too.
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
  // The page behind the monitor must not scroll, or Chrome shows empty space under it.
  useEffect(() => {
    const els = [document.documentElement, document.body]
    const before = els.map((e) => e.style.overflow)
    els.forEach((e) => { e.style.overflow = 'hidden' })
    return () => els.forEach((e, i) => { e.style.overflow = before[i] })
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
  const plural = (n: number, one: string, many: string) => (n === 1 ? one : many)
  const problems = [
    { n: urgent, text: plural(urgent, 'Urgent alert', 'Urgent alerts') },
    { n: down, text: plural(down, 'Server down', 'Servers down') },
    { n: linesDown, text: plural(linesDown, 'Line down', 'Lines down') },
    { n: tunnelsDown, text: plural(tunnelsDown, 'Tunnel down', 'Tunnels down') },
  ]
  return (
    <div className="fixed inset-0 z-50 h-dvh overflow-hidden bg-bg max-xl:overflow-y-auto">
      <div className="mx-auto flex h-full max-w-[1920px] flex-col gap-3 p-4 max-xl:h-auto">
        <MonitorHeader now={now} loading={!d} full={full} onFull={toggleFull} problems={problems}
          onClose={() => { if (document.fullscreenElement) void document.exitFullscreen(); navigate('/') }} />

        {m.dashboard.isError && <ErrorState error={m.dashboard.error} onRetry={() => void m.dashboard.refetch()} />}
        {!d ? <SkeletonRows rows={10} /> : (
          <>
            <div className="grid shrink-0 grid-cols-2 gap-3 md:grid-cols-4 2xl:grid-cols-7">
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

            <div className="grid min-h-0 flex-1 gap-3 xl:grid-cols-12">
              {m.canNetwork && <Panel icon={Network} title="Networks" note={`${formatCount(segs.length)} segments`} className="xl:col-span-5"><NetworksByKind segments={segs} /></Panel>}
              <Panel icon={BellRing} title="Open alerts" note="by severity" className="xl:col-span-3"><AlertsDonut d={d} /></Panel>
              <Panel icon={HeartPulse} title="Server health" note="by environment" className={m.canNetwork ? 'xl:col-span-4' : 'xl:col-span-9'}><ServerHealth d={d} /></Panel>
            </div>

            <div className="grid min-h-0 flex-1 gap-3 xl:grid-cols-12">
              <Panel icon={Activity} title="WAN traffic" note={fw ? `${fw.firewall} · ${formatAgo(fw.at)}` : 'last 15 minutes'} className={m.links.data ? 'xl:col-span-6' : 'xl:col-span-8'}>
                {fw ? <Traffic points={m.traffic} live={fw} /> : <p className="py-4 text-[13px] text-ink-3">{m.canLinks ? 'No firewall connected. Connect the FortiGate in System > FortiGate to see live traffic.' : 'Live traffic needs the See network links permission.'}</p>}
              </Panel>
              {m.links.data && <Panel icon={Cable} title="Network links" note="by status and type" className="xl:col-span-3"><LinksSummary l={m.links.data} fw={fw} settings={m.linkSettings.data} /></Panel>}
              <Panel icon={Gauge} title="DHCP pool use" note="fullest first" className={m.links.data ? 'xl:col-span-3' : 'xl:col-span-4'}><Pools d={d} /></Panel>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
