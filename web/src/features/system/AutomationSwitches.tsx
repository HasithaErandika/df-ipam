import { useMutation } from '@tanstack/react-query'
import { Card, Notice, Switch, useConfirm } from '../../components/ui'
import { put } from '../../lib/api'
import type { SystemSettings } from '../../lib/types'

interface Automation { key: string; label: string; hint: string; on: string; off: string }

const automations: Automation[] = [
  { key: 'autoScanDevices', label: 'Scan device segments automatically', hint: 'Floor (DHCP) segments, on the scan interval below.',
    on: 'Floor segments are scanned on schedule again.', off: 'Floor segments are scanned only when someone presses Scan now. Their alerts stay open until scanning is back on.' },
  { key: 'autoScanServers', label: 'Scan server segments automatically', hint: 'Server segments, on the scan interval below.',
    on: 'Server segments are scanned on schedule again.', off: 'Server segments are scanned only when someone presses Scan now. Their alerts stay open until scanning is back on.' },
  { key: 'serverChecks', label: 'Check monitored servers automatically', hint: 'Up, Down and Slow, on the server check interval below.',
    on: 'Monitored servers are checked on schedule again.', off: 'Servers keep their last Up, Down or Slow state, and nobody hears about an outage, until checks are back on.' },
]

/** On/off switches for the scheduled work. Scan now works whatever they say. */
export function AutomationSwitches({ settings, onSaved }: { settings: SystemSettings; onSaved: () => void }) {
  const confirm = useConfirm()
  const save = useMutation({
    mutationFn: ({ key, on }: { key: string; on: boolean }) => put('/settings', { switches: { [key]: on } }),
    meta: { success: ({ on }: { on: boolean }) => (on ? 'Switched on.' : 'Switched off.') }, onSuccess: onSaved,
  })
  const anyOff = automations.some((a) => settings.switches[a.key] === false)
  return (
    <Card title="Automatic scanning" subtitle="What runs on its own. Scan now on the Discovery page works whatever these say.">
      <div className="space-y-4 p-4">
        {automations.map((a) => (
          <Switch key={a.key} id={`set-${a.key}`} checked={settings.switches[a.key] !== false} disabled={save.isPending} label={a.label} hint={a.hint}
            onChange={async (on) => {
              if (await confirm({ title: `${on ? 'Switch on' : 'Switch off'}: ${a.label.toLowerCase()}?`, tone: 'primary',
                confirmLabel: on ? 'Switch on' : 'Switch off', body: on ? a.on : a.off })) save.mutate({ key: a.key, on })
            }} />
        ))}
        {anyOff && <Notice tone="warn">Some scheduled work is off. Findings and server states on this site may be out of date.</Notice>}
      </div>
    </Card>
  )
}
