import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../App'
import { useState, useRef, useEffect } from 'react'
import {
  FiLayout,
  FiUsers,
  FiSmartphone,
  FiBookOpen,
  FiLogOut,
  FiChevronDown,
  FiMenu,
  FiX,
  FiSun,
  FiMoon,
  FiGlobe,
  FiHome,
  FiArrowRight,
} from 'react-icons/fi'
import { useTheme } from '../lib/theme'
import { useI18n } from '../lib/i18n'

export default function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout } = useAuth()
  const { resolved, toggle } = useTheme()
  const { t, lang, setLang } = useI18n()
  const loc = useLocation()
  const navigate = useNavigate()
  const [menuOpen, setMenuOpen] = useState(false)
  const [userMenu, setUserMenu] = useState(false)
  const userMenuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const onClick = (e: MouseEvent) => {
      if (userMenuRef.current && !userMenuRef.current.contains(e.target as Node)) setUserMenu(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { setMenuOpen(false); setUserMenu(false) }
    }
    window.addEventListener('mousedown', onClick)
    window.addEventListener('keydown', onKey)
    return () => { window.removeEventListener('mousedown', onClick); window.removeEventListener('keydown', onKey) }
  }, [])
  useEffect(() => { setMenuOpen(false); setUserMenu(false) }, [loc.pathname])

  const adminNav = [
    { to: '/admin', label: t('nav.overview'), icon: FiLayout },
    { to: '/admin/users', label: t('nav.users'), icon: FiUsers },
    { to: '/admin/devices', label: t('nav.devices'), icon: FiSmartphone },
    { to: '/admin/domains', label: t('nav.domains'), icon: FiGlobe },
  ]
  const userNav = [{ to: '/dashboard', label: t('nav.dashboard'), icon: FiLayout }]
  const baseNav = user?.role === 'admin' ? adminNav : userNav
  const nav = [...baseNav, { to: '/guide', label: t('nav.guide'), icon: FiBookOpen }]

  const handleLogout = async () => { await logout(); navigate('/login', { replace: true }) }

  const crumbs = loc.pathname.split('/').filter(Boolean)
  const breadcrumbLabel = (seg: string) => {
    const map: Record<string, string> = {
      admin: t('nav.overview'), users: t('nav.users'), devices: t('nav.devices'),
      dashboard: t('nav.dashboard'), guide: t('nav.guide'), domains: 'Domains',
    }
    return map[seg] || seg
  }

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-40 border-b bg-[var(--color-surface)]/80 backdrop-blur-xl">
        <div className="mx-auto flex h-[60px] max-w-[1120px] items-center gap-4 px-4 sm:px-6">
          <Link to={user?.role === 'admin' ? '/admin' : '/dashboard'} className="flex shrink-0 items-center gap-2.5 no-underline" aria-label="Kairo home">
            <span className="grid h-8 w-8 place-items-center rounded-lg bg-[var(--color-ink)] text-[var(--color-bg)] text-[11px] font-bold">k.</span>
            <span className="hidden sm:flex flex-col leading-none">
              <span className="mono text-[14px] font-bold tracking-[-0.03em] text-[var(--color-ink)]">kairo</span>
              <span className="text-[10px] font-medium tracking-[0.08em] text-[var(--color-ink-4)] uppercase">Split Gateway</span>
            </span>
          </Link>

          <nav className="hidden lg:flex items-center gap-1 flex-1" aria-label="Primary">
            {nav.map(({ to, label, icon: Icon }) => {
              const active = loc.pathname === to || (to !== '/admin' && loc.pathname.startsWith(to + '/'))
              return (
                <Link
                  key={to} to={to} aria-current={active ? 'page' : undefined}
                  className={`inline-flex items-center gap-1.5 rounded-lg px-3.5 py-1.5 text-[13px] font-medium no-underline transition ${active ? 'bg-[var(--color-ink)] text-[var(--color-bg)]' : 'text-[var(--color-ink-3)] hover:bg-[var(--color-raised)] hover:text-[var(--color-ink)]'}`}
                >
                  <Icon size={14} /> {label}
                </Link>
              )
            })}
          </nav>

          <div className="hidden lg:flex items-center gap-1.5 shrink-0">
            <div className="flex items-center rounded-lg border bg-[var(--color-surface)] p-0.5">
              <button onClick={() => setLang('en')} className={`rounded-md px-2.5 py-1 text-xs font-medium transition ${lang === 'en' ? 'bg-[var(--color-ink)] text-[var(--color-bg)]' : 'text-[var(--color-ink-3)] hover:text-[var(--color-ink)]'}`}>EN</button>
              <button onClick={() => setLang('fa')} className={`rounded-md px-2.5 py-1 text-xs font-medium transition ${lang === 'fa' ? 'bg-[var(--color-ink)] text-[var(--color-bg)]' : 'text-[var(--color-ink-3)] hover:text-[var(--color-ink)]'}`}>FA</button>
            </div>
            <button onClick={toggle} aria-label="Toggle theme" className="grid h-8 w-8 place-items-center rounded-lg border bg-[var(--color-surface)] text-[var(--color-ink-2)] hover:bg-[var(--color-raised)]">
              {resolved === 'dark' ? <FiSun size={14} /> : <FiMoon size={14} />}
            </button>

            <div className="relative ms-1" ref={userMenuRef}>
              <button onClick={() => setUserMenu((v) => !v)} aria-haspopup="menu" aria-expanded={userMenu} className="flex items-center gap-2 rounded-lg border bg-[var(--color-surface)] pl-1 pr-3 py-1 hover:border-[var(--color-border-strong)] transition text-left">
                <span className="grid h-7 w-7 place-items-center rounded-md bg-[var(--color-brand-soft)] text-[11px] font-bold text-[var(--color-brand)]">{(user?.username?.[0] || '?').toUpperCase()}</span>
                <span className="hidden sm:flex flex-col leading-none">
                  <span className="text-[12.5px] font-semibold tracking-tight text-[var(--color-ink)]">{user?.username}</span>
                  <span className="text-[10.5px] capitalize text-[var(--color-ink-4)]">{user?.role}</span>
                </span>
                <FiChevronDown size={12} className={`text-[var(--color-ink-4)] transition ${userMenu ? 'rotate-180' : ''}`} />
              </button>
              {userMenu && (
                <div role="menu" className="absolute end-0 mt-2 w-64 rounded-xl border bg-[var(--color-surface)] p-1.5 shadow-medium animate-scale">
                  <div className="px-3 py-2.5">
                    <div className="text-sm font-semibold leading-none">{user?.username}</div>
                    <div className="mono mt-1 truncate text-xs text-[var(--color-ink-3)]">{user?.api_key?.slice(0, 20)}</div>
                  </div>
                  <div className="my-1 h-px bg-[var(--color-border)]" />
                  <Link to="/guide" role="menuitem" className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-[var(--color-ink-2)] hover:bg-[var(--color-raised)] no-underline"><FiBookOpen size={14} /> {t('nav.guide')}</Link>
                  <Link to={user?.role === 'admin' ? '/admin' : '/dashboard'} role="menuitem" className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-[var(--color-ink-2)] hover:bg-[var(--color-raised)] no-underline"><FiLayout size={14} /> {t('nav.dashboard')}</Link>
                  <div className="my-1 h-px bg-[var(--color-border)]" />
                  <button role="menuitem" onClick={handleLogout} className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-[var(--color-rose)] hover:bg-[var(--color-rose-soft)]"><FiLogOut size={14} /> {t('nav.signOut')}</button>
                </div>
              )}
            </div>
          </div>

          <button onClick={() => setMenuOpen(!menuOpen)} aria-label={menuOpen ? 'Close menu' : 'Open menu'} aria-expanded={menuOpen} className="lg:hidden ms-auto grid h-9 w-9 place-items-center rounded-lg border bg-[var(--color-surface)] text-[var(--color-ink-2)]">
            {menuOpen ? <FiX size={18} /> : <FiMenu size={18} />}
          </button>
        </div>

        {menuOpen && (
          <div className="lg:hidden border-t bg-[var(--color-surface)] px-4 py-3 animate-in">
            <nav className="flex flex-col gap-1" aria-label="Mobile">
              {nav.map(({ to, label, icon: Icon }) => {
                const active = loc.pathname === to
                return <Link key={to} to={to} onClick={() => setMenuOpen(false)} className={`inline-flex items-center gap-2 rounded-lg px-3 py-2.5 text-[14px] font-medium no-underline ${active ? 'bg-[var(--color-ink)] text-[var(--color-bg)]' : 'text-[var(--color-ink-2)] hover:bg-[var(--color-raised)]'}`}><Icon size={16} /> {label}</Link>
              })}
            </nav>
            <div className="mt-3 grid grid-cols-2 gap-2">
              <button onClick={toggle} className="inline-flex items-center justify-center gap-1.5 rounded-lg border bg-[var(--color-raised)] px-3 py-2 text-sm">{resolved === 'dark' ? <FiSun size={14} /> : <FiMoon size={14} />} {resolved === 'dark' ? 'Light' : 'Dark'}</button>
              <button onClick={() => setLang(lang === 'fa' ? 'en' : 'fa')} className="inline-flex items-center justify-center gap-1.5 rounded-lg border bg-[var(--color-raised)] px-3 py-2 text-sm"><FiGlobe size={14} /> {lang === 'fa' ? 'English' : 'فارسی'}</button>
            </div>
            <div className="mt-3 flex items-center justify-between rounded-xl border bg-[var(--color-raised)] px-3 py-2.5">
              <div className="flex items-center gap-2">
                <span className="grid h-8 w-8 place-items-center rounded-md bg-[var(--color-surface)] border text-xs font-bold">{(user?.username?.[0] || '?').toUpperCase()}</span>
                <span className="text-sm font-medium">{user?.username}</span>
              </div>
              <button onClick={handleLogout} className="btn btn-ghost btn-sm"><FiLogOut size={14} /> {t('nav.signOut')}</button>
            </div>
          </div>
        )}
      </header>

      <div className="mx-auto max-w-[1120px] px-4 sm:px-6 pt-5">
        <nav aria-label="Breadcrumb" className="flex items-center gap-1.5 text-xs">
          <Link to={user?.role === 'admin' ? '/admin' : '/dashboard'} className="inline-flex items-center gap-1 text-[var(--color-ink-4)] hover:text-[var(--color-ink-2)] no-underline"><FiHome size={12} /> Kairo</Link>
          {crumbs.map((seg, i) => (
            <span key={i} className="inline-flex items-center gap-1.5">
              <FiArrowRight size={10} className="text-[var(--color-ink-4)] opacity-60" />
              <span className={i === crumbs.length - 1 ? 'font-medium text-[var(--color-ink-2)] capitalize' : 'text-[var(--color-ink-4)] capitalize'}>{breadcrumbLabel(seg)}</span>
            </span>
          ))}
        </nav>
      </div>

      <main className="mx-auto max-w-[1120px] px-4 sm:px-6 pb-10 pt-4">
        <div className="animate-in">{children}</div>
      </main>

      <footer className="mx-auto max-w-[1120px] px-4 sm:px-6 pb-6">
        <div className="flex items-center justify-between gap-3 border-t pt-4 text-xs text-[var(--color-ink-4)]">
          <span className="mono">Kairo</span>
          <Link to="/guide" className="inline-flex items-center gap-1 text-[var(--color-ink-3)] no-underline hover:text-[var(--color-ink)]">Guide <FiArrowRight size={10} /></Link>
        </div>
      </footer>
    </div>
  )
}
