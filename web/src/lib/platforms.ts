/** Operating systems and server services, as stored by the API (api/internal/platforms). */

export const platforms: Record<string, string> = {
  windows: 'Windows', windows_server: 'Windows Server', macos: 'macOS', ios: 'iOS / iPadOS', android: 'Android', chromeos: 'ChromeOS',
  linux: 'Linux', ubuntu: 'Ubuntu', debian: 'Debian', redhat: 'Red Hat', centos: 'CentOS', rocky: 'Rocky Linux', alma: 'AlmaLinux',
  fedora: 'Fedora', freebsd: 'FreeBSD', vmware: 'VMware ESXi', cisco: 'Cisco', fortinet: 'Fortinet', mikrotik: 'MikroTik',
  ubiquiti: 'Ubiquiti', huawei: 'Huawei', dell: 'Dell', hp: 'HP', epson: 'Epson', synology: 'Synology', raspberrypi: 'Raspberry Pi', other: 'Other',
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
  [/raspberrypi/i, 'raspberrypi'], [/^npi[0-9a-f]{6}|^hp[0-9a-f]{6}|^hp(?:[-_ ]|$)|laserjet|officejet/i, 'hp'], [/^dell(?:[-_ ]|$)/i, 'dell'], [/^epson/i, 'epson'],
  [/synology|diskstation/i, 'synology'], [/esxi/i, 'vmware'], [/fortigate|^fg[0-9]/i, 'fortinet'], [/mikrotik/i, 'mikrotik'],
  [/chromebook/i, 'chromeos'],
]

/** Device kinds read from UPnP descriptions, and makers read from the MAC or UPnP. Guesses only, never saved. */
export const kinds: Record<string, string> = {
  tv: 'TV or media player', printer: 'Printer', router: 'Router', camera: 'Camera', media: 'Media server', network: 'Network device',
  accesspoint: 'Wi-Fi access point',
}

export const makers: Record<string, string> = {
  apple: 'Apple', samsung: 'Samsung', lg: 'LG', sony: 'Sony', xiaomi: 'Xiaomi', huawei: 'Huawei', dell: 'Dell', lenovo: 'Lenovo',
  google: 'Google', tplink: 'TP-Link', asus: 'ASUS', intel: 'Intel', kyocera: 'Kyocera', netgear: 'Netgear', oneplus: 'OnePlus',
  oppo: 'OPPO', vivo: 'vivo', motorola: 'Motorola', nokia: 'Nokia', panasonic: 'Panasonic', toshiba: 'Toshiba', acer: 'Acer',
  msi: 'MSI', roku: 'Roku', playstation: 'PlayStation', hp: 'HP', epson: 'Epson', cisco: 'Cisco', ubiquiti: 'Ubiquiti',
  mikrotik: 'MikroTik', fortinet: 'Fortinet', synology: 'Synology', raspberrypi: 'Raspberry Pi', vmware: 'VMware',
}

const byKind: [RegExp, string][] = [
  [/TV or media player|\bTV\b|smart ?tv|bravia|webos|tizen|android ?tv/i, 'tv'], [/printer|laserjet|officejet|deskjet|ecotank|imageclass/i, 'printer'],
  [/\brouter\b|gateway device/i, 'router'], [/access point/i, 'accesspoint'], [/camera|nvr|dvr/i, 'camera'], [/media server/i, 'media'],
]

/**
 * Network card makers that build whole devices, not cards for others' PCs, so
 * the card's maker is the device's: camera makers, and network gear makers.
 * HUAWEI TECHNOLOGIES is the enterprise side (access points, switches); Huawei's
 * phones register as "Huawei Device", and hide their MAC on Wi-Fi anyway. Plain
 * Hewlett Packard also makes PCs and printers, so it names only the maker, and
 * comes after the evidence that finds those.
 */
const byCard: [RegExp, string, string][] = [
  [/hikvision|dahua|uniview|axis communications|hanwha|vivotek/i, 'camera', 'Camera'],
  [/^huawei technologies|huawei symantec/i, 'huawei', 'Network device'],
  [/aruba|hewlett packard enterprise|h3c|ruckus|juniper/i, 'network', 'Network device'],
  [/^cisco/i, 'cisco', 'Network device'], [/ubiquiti/i, 'ubiquiti', 'Network device'],
  [/mikrotik|routerboard/i, 'mikrotik', 'Network device'], [/fortinet/i, 'fortinet', 'Network device'],
  [/^hewlett packard$|^hp inc/i, 'hp', ''],
]

/** What a network card's maker says about the whole device: an icon code, and a kind when it builds only one kind. */
export function cardDevice(card: string | null | undefined): { code: string; kind: string; maker: string } | null {
  const hit = card ? byCard.find(([re]) => re.test(card)) : undefined
  if (!hit) return null
  // A brand the app knows reads as its own name (Huawei, HP); otherwise the register's name for the maker.
  return { code: hit[1], kind: hit[2], maker: makers[hit[1]] ?? platforms[hit[1]] ?? card! }
}

const byMaker: [RegExp, string][] = [
  [/^apple/i, 'apple'], [/samsung/i, 'samsung'], [/^lg /i, 'lg'], [/^sony/i, 'sony'], [/xiaomi|redmi/i, 'xiaomi'], [/huawei|honor device/i, 'huawei'],
  [/^dell/i, 'dell'], [/lenovo|motorola mobility/i, 'lenovo'], [/^google/i, 'google'], [/tp-?link/i, 'tplink'], [/asustek|^asus/i, 'asus'],
  [/^intel/i, 'intel'], [/kyocera/i, 'kyocera'], [/netgear/i, 'netgear'], [/oneplus/i, 'oneplus'], [/^oppo|guangdong oppo/i, 'oppo'],
  [/^vivo/i, 'vivo'], [/^motorola/i, 'motorola'], [/^nokia/i, 'nokia'], [/panasonic/i, 'panasonic'], [/toshiba/i, 'toshiba'],
  [/^acer/i, 'acer'], [/micro-star|^msi/i, 'msi'], [/^roku/i, 'roku'], [/sony interactive/i, 'playstation'], [/hewlett|^hp(?:$|[-_ ])/i, 'hp'],
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

const byModel: [RegExp, string][] = [[/^(macbook|imac|macmini|macpro|mac\d)/i, 'macos'], [/^(iphone|ipad|ipod)/i, 'ios'], [/^appletv/i, 'ios']]

/** What the device evidence a form can offer as a likely platform. */
export interface PlatformEvidence { vendor?: string | null; hostname?: string | null; model?: string | null; osHint?: string | null }

/**
 * A likely platform for a form to offer, never a maker or device kind: those are
 * not platforms, and a form holding one could not be saved. Only a suggestion;
 * a person chooses it.
 */
export function likelyPlatform(e: PlatformEvidence): string | null {
  for (const [re, p] of byModel) if (e.model && re.test(e.model)) return p
  const guess = platformOf(null, e.vendor, e.hostname)?.platform
  if (guess && guess in platforms) return guess
  if (e.osHint?.startsWith('Windows')) return 'windows'
  return null
}

/** The device's maker read from its own name (ASUS-Q533MJ is an ASUS), as a maker code. */
export function makerFromName(hostname: string | null | undefined): string | null {
  if (!hostname) return null
  const first = hostname.split(/[-_ .]/)[0]
  for (const [re, m] of byMaker) if (re.test(first) || re.test(hostname)) return m
  return null
}

/** A recorded platform the form can show, or "" when it is not one of the listed ones. */
export const knownPlatform = (p: string | null | undefined) => (p && p in platforms ? p : '')

/** A readable name for any platform, kind or maker code. */
export const platformName = (code: string) => platforms[code] ?? kinds[code] ?? makers[code] ?? code

/** Everything known about a device that can say what icon it gets. */
export interface IconEvidence {
  recorded?: string | null
  gateway?: boolean
  kind?: string | null
  model?: string | null
  dhcpClass?: string | null
  hostname?: string | null
  openPorts?: number[]
  osHint?: string | null
  maker?: string | null
  /** The network card's maker, from the MAC. Used only for makers that build whole devices (see cardDevice). */
  card?: string | null
}

/** An icon and why it was chosen. A guess shows faded, with its reason. */
export interface IconChoice { code: string; guessed: boolean; reason: string }

const first = (table: [RegExp, string][], value: string | null | undefined) => (value ? table.find(([re]) => re.test(value))?.[1] : undefined)

/**
 * The icon for a device, from the most reliable evidence down: what a person
 * recorded; the gateway; what the device says it is; its model, DHCP vendor
 * class or name; its open ports; the ping TTL where it is specific (128 is
 * Windows, 255 network gear; 64 could be Linux, Android or Apple, so it picks
 * nothing); then its maker. The network card's maker is never used: it names
 * the card, not the device.
 */
export function deviceIcon(e: IconEvidence): IconChoice | null {
  if (e.recorded) return { code: e.recorded, guessed: false, reason: platformName(e.recorded) }
  if (e.gateway) return { code: 'router', guessed: false, reason: 'Gateway: the router for this network' }
  const card = cardDevice(e.card)
  const steps: [string | undefined, string][] = [
    [first(byKind, e.kind), 'it says what it is'],
    [first(byModel, e.model), 'its model'],
    [first(byVendorClass, e.dhcpClass), 'its DHCP vendor class'],
    [first(byHostname, e.hostname), 'its name'],
    [portsIcon(e.openPorts), 'its open ports'],
    [card?.code === 'camera' ? 'camera' : undefined, 'its network card: a camera maker'],
    [ttlIcon(e.osHint), 'its ping reply'],
    [first(byMaker, e.maker) ?? makerFromName(e.hostname) ?? undefined, 'its maker'],
    [card?.code, 'its network card: this maker builds whole devices'],
  ]
  for (const [code, from] of steps) if (code) return { code, guessed: true, reason: `Likely ${platformName(code)}, from ${from}` }
  return null
}

/** Ports that name a kind of device (api/internal/scanner/ports.go). Windows services win over the rest, which a PC might also open. */
export const devicePorts = { printer: [9100, 515, 631], windows: [3389, 135, 5985], camera: [554, 8000, 37777], tv: [8008, 8009], ios: [62078] }

function portsIcon(ports: number[] = []): string | undefined {
  const has = (list: number[]) => ports.some((p) => list.includes(p))
  if (has(devicePorts.printer)) return 'printer'
  if (has(devicePorts.windows)) return 'windows'
  if (has(devicePorts.ios)) return 'ios'
  if (has(devicePorts.camera)) return 'camera'
  if (has(devicePorts.tv)) return 'tv'
  return undefined
}

function ttlIcon(hint: string | null | undefined): string | undefined {
  if (hint?.startsWith('Windows')) return 'windows'
  if (hint?.startsWith('Network device')) return 'network'
  return undefined
}
