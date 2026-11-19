import { Cctv, Database, Folder, Globe, HardDrive, Laptop, Mail, Monitor, Network, Printer, Router, Server, Shield, Smartphone, Tv, Wifi, type LucideIcon } from 'lucide-react'
import {
  siAlmalinux, siAndroid, siApache, siApachetomcat, siApple, siCentos, siCisco, siDebian, siDocker, siEpson, siFedora, siFortinet,
  siFreebsd, siHp, siHuawei, siIos, siLinux, siMacos, siMikrotik, siMysql, siNginx, siPostgresql, siRaspberrypi, siRedhat,
  siRockylinux, siSynology, siUbiquiti, siUbuntu, siVmware, siSamsung, siLg, siSony, siXiaomi, siDell, siLenovo, siGoogle, siTplink, siAsus,
  siIntel, siKyocera, siNetgear, siOneplus, siOppo, siVivo, siMotorola, siNokia, siPanasonic, siToshiba, siAcer, siMsi, siRoku, siPlaystation,
  type SimpleIcon,
} from 'simple-icons'
import { deviceIcon, platformName, platforms, services, type IconEvidence } from '../lib/platforms'

// Brand marks from Simple Icons (CC0). Simple Icons carries no Microsoft marks, so
// Windows is drawn as a plain four-pane window in Windows blue.
const brand: Record<string, SimpleIcon> = {
  macos: siMacos, ios: siIos, android: siAndroid, linux: siLinux, ubuntu: siUbuntu, debian: siDebian, redhat: siRedhat, centos: siCentos,
  rocky: siRockylinux, alma: siAlmalinux, fedora: siFedora, freebsd: siFreebsd, vmware: siVmware, cisco: siCisco, fortinet: siFortinet,
  mikrotik: siMikrotik, ubiquiti: siUbiquiti, huawei: siHuawei, hp: siHp, epson: siEpson, synology: siSynology, raspberrypi: siRaspberrypi,
  apache: siApache, nginx: siNginx, tomcat: siApachetomcat, docker: siDocker, postgresql: siPostgresql, mysql: siMysql, apple: siApple,
  samsung: siSamsung, lg: siLg, sony: siSony, xiaomi: siXiaomi, dell: siDell, lenovo: siLenovo, google: siGoogle, tplink: siTplink, asus: siAsus,
  intel: siIntel, kyocera: siKyocera, netgear: siNetgear, oneplus: siOneplus, oppo: siOppo, vivo: siVivo, motorola: siMotorola, nokia: siNokia,
  panasonic: siPanasonic, toshiba: siToshiba, acer: siAcer, msi: siMsi, roku: siRoku, playstation: siPlaystation,
}

const generic: Record<string, LucideIcon> = {
  chromeos: Laptop, other: Monitor, iis: Globe, mssql: Database, oracle: Database, dns: Network, dhcp: Network, mail: Mail,
  file: Folder, active_directory: Shield, backup: HardDrive, tv: Tv, printer: Printer, router: Router, camera: Cctv, media: Tv, network: Network, accesspoint: Wifi,
}

/** A fallback by device or IP type when nothing names the platform. */
const byType: Record<string, LucideIcon> = {
  pc: Monitor, user_pc: Monitor, laptop: Laptop, printer: Printer, phone: Smartphone, access_point: Wifi, network: Network, server: Server, cctv: Monitor,
}

function Windows({ size, server }: { size: number; server?: boolean }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden>
      <g fill={server ? '#00508a' : '#0078d4'}>
        <rect x="2" y="2" width="9.5" height="9.5" rx="1" /><rect x="12.5" y="2" width="9.5" height="9.5" rx="1" />
        <rect x="2" y="12.5" width="9.5" height="9.5" rx="1" /><rect x="12.5" y="12.5" width="9.5" height="9.5" rx="1" />
      </g>
    </svg>
  )
}

function Glyph({ code, size, fallback }: { code: string; size: number; fallback?: string }) {
  if (code === 'windows' || code === 'windows_server') return <Windows size={size} server={code === 'windows_server'} />
  const icon = brand[code]
  if (icon) {
    return <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden><path d={icon.path} fill={`#${icon.hex}`} /></svg>
  }
  const Generic = generic[code] ?? (fallback ? byType[fallback] : undefined) ?? Monitor
  return <Generic size={size} strokeWidth={1.75} className="text-ink-3" aria-hidden />
}

/**
 * The platform's mark. A guessed platform shows a little faded with "likely" in its
 * label, so evidence is never mistaken for what a person recorded.
 */
export function PlatformIcon({ platform, guessed, reason, size = 16, fallbackType }: { platform: string | null; guessed?: boolean; reason?: string; size?: number; fallbackType?: string }) {
  const name = platform ? platformName(platform) : 'Platform not known'
  const title = reason ?? (platform && guessed ? `Likely ${name}` : name)
  return (
    <span role="img" aria-label={title} title={title} className={`inline-grid shrink-0 place-items-center ${guessed ? 'opacity-60' : ''}`} style={{ width: size, height: size }}>
      <Glyph code={platform ?? ''} size={size} fallback={fallbackType} />
    </span>
  )
}

/** A server's services as a row of small marks with names on hover. */
export function ServiceIcons({ list, max = 4 }: { list: string[]; max?: number }) {
  if (list.length === 0) return null
  const shown = list.slice(0, max)
  return (
    <span className="inline-flex items-center gap-1">
      {shown.map((s) => (
        <span key={s} role="img" aria-label={services[s] ?? s} title={services[s] ?? s} className="inline-grid size-5 place-items-center rounded-[4px] border border-line bg-surface">
          <Glyph code={s} size={12} />
        </span>
      ))}
      {list.length > max && <span className="text-[11px] text-ink-3" title={list.slice(max).map((s) => services[s] ?? s).join(', ')}>+{list.length - max}</span>}
    </span>
  )
}

/**
 * A name with the device's icon in front: what a person recorded, the router
 * mark for a gateway, or a likely one from the evidence (see deviceIcon), with
 * the reason on hover. type is the fallback when nothing names it.
 */
export function WithPlatform({ evidence, type, children }: { evidence: IconEvidence; type?: string; children: React.ReactNode }) {
  const icon = deviceIcon(evidence)
  const gateway = icon?.code === 'router' && evidence.gateway && !evidence.recorded
  return (
    <div className="flex min-w-0 items-start gap-2.5">
      <span className={`mt-0.5 grid size-7 shrink-0 place-items-center rounded-[5px] border ${gateway ? 'border-[var(--addr-gateway)] bg-[var(--addr-gateway)] text-white' : 'border-line bg-surface'}`}>
        {gateway
          ? <span role="img" aria-label={icon.reason} title={icon.reason}><Router size={15} strokeWidth={1.9} aria-hidden /></span>
          : <PlatformIcon platform={icon?.code ?? null} guessed={icon?.guessed} reason={icon?.reason} size={16} fallbackType={type} />}
      </span>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  )
}

/** For pickers: a platform's mark next to its name. */
export function PlatformLabel({ code }: { code: string }) {
  return <span className="inline-flex items-center gap-1.5"><Glyph code={code} size={14} />{platforms[code] ?? services[code] ?? code}</span>
}
