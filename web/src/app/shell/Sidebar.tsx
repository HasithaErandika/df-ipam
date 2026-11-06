import { PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import { Link, NavLink } from 'react-router'
import { navFor } from '../nav'
import { useMe } from '../../lib/session'

function LogoMark() {
  return (
    <svg width="26" height="26" viewBox="0 0 32 32" aria-hidden className="shrink-0">
      <rect width="32" height="32" rx="7" fill="#17305f" />
      <rect x="7" y="7" width="8" height="8" rx="1.5" fill="#6c9bf2" />
      <rect x="17" y="17" width="8" height="8" rx="1.5" fill="#6c9bf2" />
      <rect x="17" y="7" width="8" height="8" rx="1.5" fill="#fff" opacity=".55" />
      <rect x="7" y="17" width="8" height="8" rx="1.5" fill="#fff" opacity=".55" />
    </svg>
  )
}

export function Sidebar({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  const { permissions } = useMe()
  const groups = navFor(permissions)
  return (
    <aside className={`flex shrink-0 flex-col border-r border-side-border bg-side transition-[width] duration-150 ${collapsed ? 'w-[60px]' : 'w-[248px]'}`}>
      <div className={`flex h-[60px] shrink-0 items-center border-b border-side-border ${collapsed ? 'justify-center' : 'justify-between pr-2 pl-4'}`}>
        <Link to="/" aria-label="DF-IPAM home" className="flex min-w-0 items-center gap-2.5">
          <LogoMark />
          {!collapsed && (
            <span className="min-w-0 leading-[1.15]">
              <span className="block text-[15px] font-semibold tracking-tight text-side-ink">DF-IPAM</span>
              <span className="block truncate text-[11px] text-side-ink-3">Dialog Finance</span>
            </span>
          )}
        </Link>
        {!collapsed && (
          <button type="button" onClick={onToggle} aria-label="Collapse sidebar" className="grid size-7 cursor-pointer place-items-center rounded text-side-ink-3 hover:bg-side-hover hover:text-side-ink">
            <PanelLeftClose size={16} />
          </button>
        )}
      </div>

      <nav aria-label="Main" className="flex-1 overflow-y-auto py-2">
        {collapsed && (
          <button type="button" onClick={onToggle} aria-label="Expand sidebar" className="mx-auto mb-1 grid size-8 cursor-pointer place-items-center rounded text-side-ink-3 hover:bg-side-hover hover:text-side-ink">
            <PanelLeftOpen size={16} />
          </button>
        )}
        {groups.map(({ group, items }) => (
          <div key={group} className="mb-2.5">
            {collapsed ? <div className="mx-4 my-2 h-px bg-side-border" />
              : group && <div className="px-4 pt-1.5 pb-1 text-[10.5px] font-semibold tracking-[.08em] text-side-ink-3 uppercase">{group}</div>}
            {items.map(({ to, label, icon: Icon, phase }) => (
              <NavLink
                key={to}
                to={to}
                end={to === '/'}
                title={collapsed ? label : undefined}
                aria-label={label}
                className={({ isActive }) =>
                  `relative flex h-[30px] items-center gap-2.5 text-[13px] transition-colors ${collapsed ? 'justify-center' : 'pr-3 pl-4'} ${
                    isActive
                      ? 'bg-side-active font-medium text-side-active-ink before:absolute before:inset-y-0 before:left-0 before:w-[3px] before:bg-accent'
                      : `hover:bg-side-hover hover:text-side-ink ${phase ? 'text-side-ink-3' : 'text-side-ink-2'}`
                  }`
                }
              >
                <Icon size={15} strokeWidth={1.75} className="shrink-0" aria-hidden />
                {!collapsed && <span className="min-w-0 flex-1 truncate">{label}</span>}
                {!collapsed && phase && <span className="text-[10px] text-side-ink-3">Planned</span>}
              </NavLink>
            ))}
          </div>
        ))}
      </nav>

    </aside>
  )
}
