/**
 * Windows builds by number, as the SMB handshake reports them ("10.0.22631").
 * Some builds are shared by a desktop release and a Windows Server release.
 */
const builds: Record<number, string> = {
  10240: 'Windows 10 1507', 10586: 'Windows 10 1511', 14393: 'Windows 10 1607 / Server 2016', 15063: 'Windows 10 1703',
  16299: 'Windows 10 1709', 17134: 'Windows 10 1803', 17763: 'Windows 10 1809 / Server 2019', 18362: 'Windows 10 1903',
  18363: 'Windows 10 1909', 19041: 'Windows 10 2004', 19042: 'Windows 10 20H2', 19043: 'Windows 10 21H1', 19044: 'Windows 10 21H2',
  19045: 'Windows 10 22H2', 20348: 'Windows Server 2022', 22000: 'Windows 11 21H2', 22621: 'Windows 11 22H2',
  22631: 'Windows 11 23H2', 26100: 'Windows 11 24H2 / Server 2025', 26200: 'Windows 11 25H2',
}

const older: Record<string, string> = { '6.1': 'Windows 7 / Server 2008 R2', '6.2': 'Windows 8 / Server 2012', '6.3': 'Windows 8.1 / Server 2012 R2' }

/** "10.0.22631" → "Windows 11 23H2 (10.0.22631)". Unknown builds keep the number. */
export function windowsName(version: string): string {
  if (!version) return ''
  const [major, minor, build] = version.split('.')
  const name = major === '10' ? builds[Number(build)] ?? (Number(build) >= 22000 ? 'Windows 11' : 'Windows 10') : older[`${major}.${minor}`]
  return name ? `${name} (${version})` : `Windows ${version}`
}

/** The domain or workgroup a device names, preferring the DNS domain from Windows. */
export const deviceDomain = (r: { windowsDomain: string; workgroup: string }) => r.windowsDomain || r.workgroup

/** How a device is attached to Entra ID, in words. */
export const entraJoin: Record<string, string> = { AzureAd: 'Entra joined', ServerAd: 'Hybrid joined (from the domain)', Workplace: 'Entra registered' }

/** Intune's compliance states, in words. */
export const compliance: Record<string, string> = {
  compliant: 'Compliant', noncompliant: 'Not compliant', inGracePeriod: 'In grace period', conflict: 'Conflicting policies',
  error: 'Error', configManager: 'Managed by Configuration Manager', unknown: 'Unknown',
}
