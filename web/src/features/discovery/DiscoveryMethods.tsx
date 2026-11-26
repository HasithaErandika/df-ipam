import { Card } from '../../components/ui'

interface Method { name: string; finds: string; traffic: string; who: string; when: string }

// Kept in step with api/internal/scanner, api/internal/integrations/fortigate and api/internal/integrations/intune.
const methods: Method[] = [
  { name: 'Ping (ICMP echo)', finds: 'Which addresses are in use', traffic: 'ICMP echo request and reply', who: 'Every address in each floor and server segment, at most 30 a second by default, shared with server checks', when: 'Every scan' },
  { name: 'ARP table', finds: 'MAC address of each host that answered', traffic: 'None extra: read from the server’s own neighbour table after the ping', who: 'Hosts on networks the server is directly attached to', when: 'Every scan' },
  { name: 'Reverse DNS', finds: 'Registered hostname', traffic: 'DNS PTR query, UDP and TCP 53, to the server’s DNS resolver', who: 'Each host that answered', when: 'Every scan' },
  { name: 'NetBIOS name', finds: 'Windows computer name, MAC, and workgroup or domain', traffic: 'UDP 137, one status query', who: 'Each host that answered', when: 'Every scan' },
  { name: 'mDNS name', finds: 'Name of Apple devices, many phones, printers and Linux machines', traffic: 'UDP 5353, one unicast query (no multicast)', who: 'Each host that answered, if it has no name yet', when: 'Every scan' },
  { name: 'SSDP / UPnP description', finds: 'Friendly name, maker, model and type of TVs, media players, printers and routers', traffic: 'UDP 1900 unicast search, then HTTP GET of the description on the same host', who: 'Each host that answered', when: 'Every scan' },
  { name: 'Windows details (SMB)', finds: 'Windows version and build, full DNS name and domain', traffic: 'TCP 445: SMB2 negotiate and an NTLM sign-in request with no credentials, closed at the challenge. Nobody signs in', who: 'Hosts that answered NetBIOS or have port 445 open', when: 'Every 6 hours per host' },
  { name: 'MAC vendor', finds: 'The maker of the network card', traffic: 'None: looked up in a built-in copy of the IEEE register', who: 'Hosts with a real (not randomised) MAC', when: 'Every scan' },
  { name: 'Server check', finds: 'Up or Down', traffic: 'ICMP echo, then a TCP connect to the configured port (no data sent)', who: 'Register entries marked Monitor', when: 'Server check interval' },
  { name: 'FortiGate (optional)', finds: 'ARP table, DHCP leases with hostname and vendor class, VPN sessions with their user, VPN kind and origin', traffic: 'HTTPS GET to the firewall’s REST API with a read-only token', who: 'The firewall only', when: 'Its own interval, when switched on' },
  { name: 'Microsoft Entra ID (with Intune)', finds: 'Registered owner of devices joined to or registered in Entra ID, matched by computer name', traffic: 'HTTPS GET to Microsoft Graph with the sign-in app’s own token. Nothing is sent to the devices', who: 'Microsoft Graph only', when: 'With each Intune read' },
  { name: 'Microsoft Intune (optional)', finds: 'Primary user, serial, maker and model, OS, compliance, disk encryption', traffic: 'HTTPS GET to Microsoft Graph with the sign-in app’s own token. Nothing is sent to the devices', who: 'Microsoft Graph only', when: 'Its own interval, when switched on' },
]

/**
 * What discovery sends, where and when, for whoever approves it on the network.
 * Everything is read-only: nothing logs in to a device, changes a setting or
 * inspects traffic.
 */
export function DiscoveryMethods() {
  return (
    <Card title="How discovery works" subtitle="Every method is read-only and standard. Allow these from the DF-Atlas server to the scanned segments.">
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr>{['Method', 'What it finds', 'Network traffic', 'Sent to', 'When'].map((h) => (
              <th key={h} scope="col" className="border-b border-line bg-surface-2 px-3 py-2 text-left text-[11px] font-semibold tracking-[.04em] whitespace-nowrap text-ink-3 uppercase">{h}</th>
            ))}</tr>
          </thead>
          <tbody>
            {methods.map((m) => (
              <tr key={m.name} className="align-top">
                <td className="border-b border-line px-3 py-2 font-medium whitespace-nowrap text-ink">{m.name}</td>
                <td className="border-b border-line px-3 py-2 text-ink-2">{m.finds}</td>
                <td className="border-b border-line px-3 py-2 font-mono text-[12px] text-ink-2">{m.traffic}</td>
                <td className="border-b border-line px-3 py-2 text-ink-2">{m.who}</td>
                <td className="border-b border-line px-3 py-2 whitespace-nowrap text-ink-3">{m.when}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="grid gap-3 border-t border-line p-4 text-[12.5px] text-ink-2 md:grid-cols-3">
        <p><strong className="text-ink">Firewall rule.</strong> From the DF-Atlas server to each segment: ICMP echo, UDP 137, 1900 and 5353, and TCP to UPnP description ports and server check ports. Replies come back to the server.</p>
        <p><strong className="text-ink">MAC addresses</strong> need the server on the same network as the devices: a switch trunk port with one VLAN per floor, or the FortiGate connection. Phones and laptops on Wi-Fi often use randomised MACs, shown as Private MAC.</p>
        <p><strong className="text-ink">Never done:</strong> port scanning, logging in to devices, SNMP guessing, packet capture, or blocking anything. Segments over 4,096 addresses are not swept.</p>
      </div>
    </Card>
  )
}
