import { PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import { Link, NavLink } from 'react-router'
import { LogoMark } from '../../components/LogoMark'
import { cx } from '../../components/ui/cx'
import { useMe } from '../../lib/session'
import { navFor } from '../nav'
import { useNavBadges } from './navBadges'

const badgeTone = { alerts: 'bg-danger text-white', devices: 'bg-warn text-white', requests: 'bg-accent text-white' }

export function Sidebar({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  const me = useMe()
  const groups = navFor(me.permissions)
  const counts = useNavBadges()
  return (
    <aside className={cx('flex shrink-0 flex-col border-r border-side-border bg-side transition-[width] duration-150', collapsed ? 'w-[64px]' : 'w-[252px]')}>
      <div className={cx('flex h-[60px] shrink-0 items-center border-b border-side-border', collapsed ? 'justify-center' : 'justify-between gap-2 pr-2 pl-4')}>
        {!collapsed && (
          <Link to="/" aria-label="DF-Atlas home" className="flex min-w-0 items-center gap-2.5">
            <LogoMark />
            <span className="min-w-0 leading-[1.15]">
              <span className="block text-[15px] font-semibold tracking-tight text-side-ink">DF-Atlas</span>
              <span className="block truncate text-[11px] text-side-ink-3">Dialog Finance · Network and IPAM</span>
            </span>
          </Link>
        )}
        <button type="button" onClick={onToggle} aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-md text-side-ink-3 hover:bg-side-hover hover:text-side-ink">
          {collapsed ? <PanelLeftOpen size={17} aria-hidden /> : <PanelLeftClose size={17} aria-hidden />}
        </button>
      </div>

      <nav aria-label="Main" className="flex-1 overflow-y-auto px-2.5 py-3">
        {groups.map(({ group, items }, gi) => (
          <div key={group} className={gi > 0 ? 'mt-4' : undefined}>
            {collapsed ? (gi > 0 && <div className="mx-2 mb-2 h-px bg-side-border" />)
              : <div className="px-2.5 pb-1.5 text-[10.5px] font-semibold tracking-[.09em] text-side-ink-3 uppercase">{group}</div>}
            <ul className="space-y-0.5">
              {items.map(({ to, label, icon: Icon, badge, end }) => {
                const count = badge ? counts[badge] : 0
                return (
                  <li key={to}>
                    <NavLink to={to} end={to === '/' || end} title={collapsed ? (count ? `${label} (${count})` : label) : undefined} aria-label={count ? `${label}, ${count}` : label}
                      className={({ isActive }) => cx('group relative flex h-9 items-center gap-3 rounded-md text-[13.5px] transition-colors',
                        collapsed ? 'justify-center' : 'px-2.5',
                        isActive ? 'bg-side-active font-semibold text-side-active-ink shadow-[inset_0_0_0_1px_rgba(27,71,166,.12)]' : 'text-side-ink-2 hover:bg-side-hover hover:text-side-ink')}>
                      {({ isActive }) => (
                        <>
                          {isActive && !collapsed && <span aria-hidden className="absolute inset-y-2 left-0 w-[3px] rounded-r bg-accent" />}
                          <Icon size={17} strokeWidth={isActive ? 2.1 : 1.75} className={cx('shrink-0', isActive ? 'text-accent' : 'text-side-ink-3 group-hover:text-side-ink-2')} aria-hidden />
                          {!collapsed && <span className="min-w-0 flex-1 truncate">{label}</span>}
                          {badge && count > 0 && (collapsed
                            ? <span aria-hidden className={cx('absolute top-1.5 right-2 size-2 rounded-full', badgeTone[badge].split(' ')[0])} />
                            : <span className={cx('min-w-5 rounded-full px-1.5 text-center text-[11px] leading-[18px] font-semibold tabular-nums', badgeTone[badge])}>{count > 99 ? '99+' : count}</span>)}
                        </>
                      )}
                    </NavLink>
                  </li>
                )
              })}
            </ul>
          </div>
        ))}
      </nav>

      <div className={cx('shrink-0 border-t border-side-border py-2.5 text-[11px] leading-snug text-side-ink-3', collapsed ? 'text-center' : 'px-4')}
        title={`DF-Atlas ${__APP_VERSION__}${__APP_COMMIT__ ? ` (${__APP_COMMIT__})` : ''}`}>
        {collapsed ? <span className="tabular-nums">v{__APP_VERSION__}</span> : (
          <>
            <div><span className="font-medium text-side-ink-2">DF-Atlas</span> <span className="tabular-nums">v{__APP_VERSION__}</span></div>
            {__APP_COMMIT__ && <div className="tabular-nums">Build {__APP_COMMIT__}</div>}
          </>
        )}
      </div>
    </aside>
  )
}
