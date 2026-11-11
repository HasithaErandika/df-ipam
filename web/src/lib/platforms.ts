/** Operating systems and server services, as stored by the API (api/internal/platforms). */

export const platforms: Record<string, string> = {
  windows: 'Windows', windows_server: 'Windows Server', macos: 'macOS', ios: 'iOS / iPadOS', android: 'Android', chromeos: 'ChromeOS',
  linux: 'Linux', ubuntu: 'Ubuntu', debian: 'Debian', redhat: 'Red Hat', centos: 'CentOS', rocky: 'Rocky Linux', alma: 'AlmaLinux',
  fedora: 'Fedora', freebsd: 'FreeBSD', vmware: 'VMware ESXi', cisco: 'Cisco', fortinet: 'Fortinet', mikrotik: 'MikroTik',
  ubiquiti: 'Ubiquiti', huawei: 'Huawei', hp: 'HP', epson: 'Epson', synology: 'Synology', raspberrypi: 'Raspberry Pi', other: 'Other',
}

export const services: Record<string, string> = {
  apache: 'Apache', nginx: 'Nginx', tomcat: 'Tomcat', iis: 'IIS', docker: 'Docker', postgresql: 'PostgreSQL', mysql: 'MySQL',
  mssql: 'SQL Server', oracle: 'Oracle DB', dns: 'DNS', dhcp: 'DHCP', mail: 'Mail', file: 'File share', active_directory: 'Active Directory',
  backup: 'Backup', other: 'Other',
}

const byVendorClass: [RegExp, string][] = [
  [/^MSFT/i, 'windows'], [/android/i, 'android'], [/ubuntu/i, 'ubuntu'], [/debian/i, 'debian'], [/red ?hat|rhel/i, 'redhat'],
  [/darwin|mac ?os/i, 'macos'], [/iphone|ipad|ios/i, 'ios'], [/^dhcpcd|^udhcp|linux/i, 'linux'], [/cisco/i, 'cisco'],
  [/HP |hewlett/i, 'hp'], [/epson/i, 'epson'],
]

const byHostname: [RegExp, string][] = [
  [/^(desktop|laptop|win|pc)-/i, 'windows'], [/iphone|ipad/i, 'ios'], [/macbook|imac|mac-?mini|mac-?studio/i, 'macos'],
  [/android|galaxy|redmi|pixel|oneplus|huawei-|oppo|vivo/i, 'android'], [/ubuntu/i, 'ubuntu'], [/debian/i, 'debian'],
  [/raspberrypi/i, 'raspberrypi'], [/^npi[0-9a-f]{6}|^hp[0-9a-f]{6}|laserjet|officejet/i, 'hp'], [/^epson/i, 'epson'],
  [/synology|diskstation/i, 'synology'], [/esxi/i, 'vmware'], [/fortigate|^fg[0-9]/i, 'fortinet'], [/mikrotik/i, 'mikrotik'],
  [/chromebook/i, 'chromeos'],
]

/**
 * The platform a person recorded, or a likely one from DHCP and name evidence.
 * A guess is only a hint: it is shown as "likely" and never saved by itself.
 */
export function platformOf(recorded: string | null | undefined, vendor?: string | null, hostname?: string | null): { platform: string; guessed: boolean } | null {
  if (recorded) return { platform: recorded, guessed: false }
  for (const [re, p] of byVendorClass) if (vendor && re.test(vendor)) return { platform: p, guessed: true }
  for (const [re, p] of byHostname) if (hostname && re.test(hostname)) return { platform: p, guessed: true }
  return null
}
