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
]

const byHostname: [RegExp, string][] = [
  [/^(desktop|laptop|win|pc)-/i, 'windows'], [/iphone|ipad/i, 'ios'], [/macbook|imac|mac-?mini|mac-?studio/i, 'macos'],
  [/android|galaxy|redmi|pixel|oneplus|huawei-|oppo|vivo/i, 'android'], [/ubuntu/i, 'ubuntu'], [/debian/i, 'debian'],
  [/raspberrypi/i, 'raspberrypi'], [/^npi[0-9a-f]{6}|^hp[0-9a-f]{6}|laserjet|officejet/i, 'hp'], [/^epson/i, 'epson'],
  [/synology|diskstation/i, 'synology'], [/esxi/i, 'vmware'], [/fortigate|^fg[0-9]/i, 'fortinet'], [/mikrotik/i, 'mikrotik'],
  [/chromebook/i, 'chromeos'],
]

/** Device kinds read from UPnP descriptions, and makers read from the MAC or UPnP. Guesses only, never saved. */
export const kinds: Record<string, string> = { tv: 'TV or media player', printer: 'Printer', router: 'Router', camera: 'Camera', media: 'Media server' }

export const makers: Record<string, string> = {
  apple: 'Apple', samsung: 'Samsung', lg: 'LG', sony: 'Sony', xiaomi: 'Xiaomi', huawei: 'Huawei', dell: 'Dell', lenovo: 'Lenovo',
  google: 'Google', tplink: 'TP-Link', asus: 'ASUS', intel: 'Intel', kyocera: 'Kyocera', netgear: 'Netgear', oneplus: 'OnePlus',
  oppo: 'OPPO', vivo: 'vivo', motorola: 'Motorola', nokia: 'Nokia', panasonic: 'Panasonic', toshiba: 'Toshiba', acer: 'Acer',
  msi: 'MSI', roku: 'Roku', playstation: 'PlayStation', hp: 'HP', epson: 'Epson', cisco: 'Cisco', ubiquiti: 'Ubiquiti',
  mikrotik: 'MikroTik', fortinet: 'Fortinet', synology: 'Synology', raspberrypi: 'Raspberry Pi', vmware: 'VMware',
}

const byKind: [RegExp, string][] = [
  [/TV or media player|\bTV\b|smart ?tv|bravia|webos|tizen|android ?tv/i, 'tv'], [/printer|laserjet|officejet|deskjet|ecotank|imageclass/i, 'printer'],
  [/\brouter\b|gateway device/i, 'router'], [/camera|nvr|dvr/i, 'camera'], [/media server/i, 'media'],
]

const byMaker: [RegExp, string][] = [
  [/^apple/i, 'apple'], [/samsung/i, 'samsung'], [/^lg /i, 'lg'], [/^sony/i, 'sony'], [/xiaomi|redmi/i, 'xiaomi'], [/huawei|honor device/i, 'huawei'],
  [/^dell/i, 'dell'], [/lenovo|motorola mobility/i, 'lenovo'], [/^google/i, 'google'], [/tp-?link/i, 'tplink'], [/asustek|^asus/i, 'asus'],
  [/^intel/i, 'intel'], [/kyocera/i, 'kyocera'], [/netgear/i, 'netgear'], [/oneplus/i, 'oneplus'], [/^oppo|guangdong oppo/i, 'oppo'],
  [/^vivo/i, 'vivo'], [/^motorola/i, 'motorola'], [/^nokia/i, 'nokia'], [/panasonic/i, 'panasonic'], [/toshiba/i, 'toshiba'],
  [/^acer/i, 'acer'], [/micro-star|^msi/i, 'msi'], [/^roku/i, 'roku'], [/sony interactive/i, 'playstation'], [/hewlett|^hp /i, 'hp'],
  [/seiko epson|^epson/i, 'epson'], [/^cisco/i, 'cisco'], [/ubiquiti/i, 'ubiquiti'], [/mikrotik|routerboard/i, 'mikrotik'],
  [/fortinet/i, 'fortinet'], [/synology/i, 'synology'], [/raspberry/i, 'raspberrypi'], [/vmware/i, 'vmware'],
]

/**
 * The platform a person recorded, or a likely one from the evidence: the DHCP
 * vendor class, the UPnP device type, the hostname, then the maker of the MAC.
 * A guess is only a hint: it is shown as "likely" and never saved by itself.
 */
export function platformOf(recorded: string | null | undefined, vendor?: string | null, hostname?: string | null): { platform: string; guessed: boolean } | null {
  if (recorded) return { platform: recorded, guessed: false }
  for (const [re, p] of byVendorClass) if (vendor && re.test(vendor)) return { platform: p, guessed: true }
  for (const [re, p] of byKind) if (vendor && re.test(vendor)) return { platform: p, guessed: true }
  for (const [re, p] of byHostname) if (hostname && re.test(hostname)) return { platform: p, guessed: true }
  for (const [re, p] of byMaker) if (vendor && re.test(vendor)) return { platform: p, guessed: true }
  return null
}

/** A readable name for any platform, kind or maker code. */
export const platformName = (code: string) => platforms[code] ?? kinds[code] ?? makers[code] ?? code
