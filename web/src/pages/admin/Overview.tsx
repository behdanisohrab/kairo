import { useState, useEffect, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import type { UserData, DeviceData } from '../../api'
import { useToast } from '../../components/ui/Toast'
import { useI18n } from '../../lib/i18n'
import { FiUsers, FiSmartphone, FiActivity, FiArrowRight, FiRefreshCw, FiShield } from 'react-icons/fi'

function deviceBadge(t: string) {
  const m: Record<string, { bg: string; fg: string; border: string }> = {
    Desktop: { bg: 'var(--color-brand-soft)', fg: 'var(--color-brand)', border: '#dbe0ff' },
    Android: { bg: 'var(--color-emerald-soft)', fg: 'var(--color-emerald)', border: '#bbf7d0' },
    iOS: { bg: 'var(--color-violet-soft)', fg: 'var(--color-violet)', border: '#e9d5ff' },
    Tablet: { bg: '#ecfeff', fg: '#0891b2', border: '#a5f3fc' },
    Bot: { bg: 'var(--color-rose-soft)', fg: 'var(--color-rose)', border: '#fecaca' },
  }
  return m[t] || { bg: 'var(--color-raised)', fg: 'var(--color-ink-3)', border: 'var(--color-border)' }
}

export default function Overview() {
  const [users, setUsers] = useState<UserData[]>([])
  const [devices, setDevices] = useState<DeviceData[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const { error } = useToast()
  const { t } = useI18n()

  const load = async (showSpinner = true) => {
    if (showSpinner) setLoading(true)
    else setRefreshing(true)
    try {
      const [u, d] = await Promise.all([api.listUsers(), api.allDevices()])
      if (u.ok && u.users) setUsers(u.users)
      else if (!u.ok) error(u.error || 'Failed to load users')
      if (d.ok && d.devices) setDevices(d.devices)
      else if (!d.ok) error(d.error || 'Failed to load devices')
    } catch (e) {
      error(e instanceof Error ? e.message : 'Failed to load overview')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => {
    load()
    const id = setInterval(() => load(false), 30000)
    return () => clearInterval(id)
  }, [])

  const stats = useMemo(() => {
    const admins = users.filter((u) => u.role === 'admin').length
    const recentDevices = [...devices].sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime())
    const byType = devices.reduce<Record<string, number>>((acc, d) => {
      const k = d.device_type || 'Unknown'
      acc[k] = (acc[k] || 0) + 1
      return acc
    }, {})
    return { admins, recentDevices, byType }
  }, [users, devices])

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="skeleton h-8 w-48" />
        <div className="grid gap-3 sm:grid-cols-3">
          <div className="skeleton h-28 rounded-[18px]" />
          <div className="skeleton h-28 rounded-[18px]" />
          <div className="skeleton h-28 rounded-[18px]" />
        </div>
        <div className="skeleton h-64 rounded-[18px]" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-[22px] font-semibold tracking-[-0.02em]">{t('overview.title')}</h1>
          <p className="mt-1 text-sm text-[var(--color-ink-3)]">{t('overview.subtitle')}</p>
        </div>
        <button onClick={() => load(false)} disabled={refreshing} className="btn btn-outline btn-sm inline-flex items-center gap-1.5">
          <FiRefreshCw size={12} className={refreshing ? 'animate-spin' : ''} /> {refreshing ? t('common.refreshing') : t('common.refresh')}
        </button>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        {[
          {
            label: t('overview.totalUsers'),
            value: users.length,
            hint: `${stats.admins} admins • ${users.length - stats.admins} members`,
            Icon: FiUsers,
            to: '/admin/users',
          },
          {
            label: t('overview.trackedDevices'),
            value: devices.length,
            hint: Object.entries(stats.byType).slice(0, 3).map(([k, v]) => `${k} ${v}`).join(', ') || t('overview.noDevices'),
            Icon: FiSmartphone,
            to: '/admin/devices',
          },
          {
            label: t('overview.health'),
            value: t('overview.ok'),
            hint: t('overview.allServices'),
            Icon: FiShield,
            to: '/healthz',
          },
        ].map((s) => (
          <Link key={s.label} to={s.to} className="card card-hover p-5 no-underline group">
            <div className="flex items-start justify-between">
              <div className="grid h-8 w-8 place-items-center rounded-xl bg-[var(--color-raised)] border text-[var(--color-ink-2)]">
                <s.Icon size={16} />
              </div>
              <span className="inline-flex items-center gap-1 text-xs text-[var(--color-ink-4)] group-hover:text-[var(--color-ink-3)]">
                {t('common.viewAll')} <FiArrowRight size={10} />
              </span>
            </div>
            <div className="mt-3">
              <div className="text-xs font-semibold tracking-wide text-[var(--color-ink-4)] uppercase">{s.label}</div>
              <div className="mt-1 text-[28px] font-semibold leading-none tracking-[-0.03em]">{s.value}</div>
              <div className="mt-1.5 text-xs leading-4 text-[var(--color-ink-3)] line-clamp-1">{s.hint}</div>
            </div>
          </Link>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2 rounded-2xl border bg-[var(--color-surface)] px-4 py-3 text-xs">
        <span className="inline-flex items-center gap-1.5 rounded-md bg-[var(--color-emerald-soft)] px-2.5 py-1 font-medium text-[var(--color-emerald)] border" style={{ borderColor: '#bbf7d0' }}>
          <FiActivity size={12} /> {t('overview.live')}
        </span>
        <span className="text-[var(--color-ink-3)]">{t('overview.dnsLine')}</span>
        <span className="ms-auto hidden sm:inline text-[var(--color-ink-4)]">
          {t('overview.lastRefresh')} {new Date().toLocaleTimeString()}
        </span>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="card overflow-hidden">
          <div className="flex items-center justify-between border-b px-5 py-3">
            <h3 className="text-sm font-semibold tracking-tight flex items-center gap-1.5">
              <FiSmartphone size={14} /> {t('overview.recentDevices')}
            </h3>
            <Link to="/admin/devices" className="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-brand)] no-underline hover:underline">
              {t('common.viewAll')} <FiArrowRight size={10} />
            </Link>
          </div>
          {devices.length === 0 ? (
            <div className="p-8 text-center">
              <div className="mx-auto grid h-10 w-10 place-items-center rounded-2xl border bg-[var(--color-raised)]">
                <FiSmartphone size={18} />
              </div>
              <p className="mt-3 text-sm font-medium">{t('overview.noDevices')}</p>
              <p className="mt-1 text-xs leading-5 text-[var(--color-ink-3)]">{t('overview.noDevicesDesc')}</p>
            </div>
          ) : (
            <div className="divide-y" style={{ borderColor: 'var(--color-border)' }}>
              {stats.recentDevices.slice(0, 6).map((d) => {
                const c = deviceBadge(d.device_type)
                return (
                  <div key={d.id} className="flex items-center gap-3 px-5 py-3">
                    <span className="badge shrink-0" style={{ background: c.bg, color: c.fg, borderColor: c.border }}>
                      {d.device_type || 'Unknown'}
                    </span>
                    <span className="mono min-w-0 flex-1 truncate text-xs text-[var(--color-ink-2)]">{d.ip}</span>
                    <span className="hidden sm:inline mono text-[11px] text-[var(--color-ink-4)]">{d.ja3_hash.slice(0, 10)}…</span>
                    <span className="shrink-0 text-xs text-[var(--color-ink-4)]">{new Date(d.last_seen).toLocaleDateString()}</span>
                  </div>
                )
              })}
            </div>
          )}
        </div>

        <div className="card overflow-hidden">
          <div className="flex items-center justify-between border-b px-5 py-3">
            <h3 className="text-sm font-semibold tracking-tight flex items-center gap-1.5">
              <FiUsers size={14} /> {t('overview.users')}
            </h3>
            <Link to="/admin/users" className="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-brand)] no-underline hover:underline">
              {t('common.manage')} <FiArrowRight size={10} />
            </Link>
          </div>
          {users.length === 0 ? (
            <div className="p-8 text-center text-sm text-[var(--color-ink-3)]">No users found.</div>
          ) : (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>User</th>
                    <th>Role</th>
                    <th>Last login</th>
                  </tr>
                </thead>
                <tbody>
                  {users.slice(0, 6).map((u) => (
                    <tr key={u.id}>
                      <td className="font-medium">{u.username}</td>
                      <td>
                        <span className="badge" style={{ background: u.role === 'admin' ? 'var(--color-ink)' : 'var(--color-raised)', color: u.role === 'admin' ? 'var(--color-bg)' : 'var(--color-ink-2)', borderColor: u.role === 'admin' ? 'var(--color-ink)' : 'var(--color-border)' }}>
                          {u.role}
                        </span>
                      </td>
                      <td className="text-xs text-[var(--color-ink-3)]">{u.last_login ? new Date(u.last_login).toLocaleString() : '-'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      {devices.length > 0 && (
        <div className="card p-5">
          <h3 className="text-sm font-semibold flex items-center gap-1.5">
            <FiActivity size={14} /> {t('overview.byType')}
          </h3>
          <div className="mt-3 flex flex-wrap gap-2">
            {Object.entries(stats.byType).map(([k, v]) => {
              const c = deviceBadge(k)
              return (
                <span key={k} className="inline-flex items-center gap-1.5 rounded-md border px-3 py-1 text-xs font-medium" style={{ background: c.bg, color: c.fg, borderColor: c.border }}>
                  <span className="h-1.5 w-1.5 rounded-sm" style={{ background: c.fg }} /> {k} <span className="opacity-70">{v}</span>
                </span>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
