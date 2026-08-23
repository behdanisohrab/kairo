import { useState, useEffect, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import type { UserData, ConnectionLogData, NameCount } from '../../api'
import { useToast } from '../../components/ui/Toast'
import { useI18n } from '../../lib/i18n'
import { FiUsers, FiActivity, FiArrowRight, FiRefreshCw, FiShield, FiServer } from 'react-icons/fi'

interface HealthData {
  ok?: boolean
  version?: string
  uptime?: string
  host?: string
  vps_ip?: string
  checks?: { database?: { ok?: boolean; error?: string }; allowlisted?: number; restricted?: number; direct?: number }
}

export default function Overview() {
  const [users, setUsers] = useState<UserData[]>([])
  const [recent, setRecent] = useState<ConnectionLogData[]>([])
  const [topDomains, setTopDomains] = useState<NameCount[]>([])
  const [connections, setConnections] = useState(0)
  const [health, setHealth] = useState<HealthData>({})
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const { error } = useToast()
  const { t } = useI18n()

  const load = async (showSpinner = true) => {
    if (showSpinner) setLoading(true)
    else setRefreshing(true)
    try {
      const [u, tr] = await Promise.all([api.listUsers(), api.traffic('24h')])
      if (u.ok && u.users) setUsers(u.users)
      else if (!u.ok) error(u.error || 'Failed to load users')
      if (tr.ok) {
        setRecent(tr.recent ?? [])
        setTopDomains(tr.top_domains ?? [])
        setConnections(tr.connections ?? 0)
      }
      setHealth(await api.health())
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

  const admins = useMemo(() => users.filter((u) => u.role === 'admin').length, [users])
  const dbOk = health.checks?.database?.ok !== false

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="skeleton h-8 w-48" />
        <div className="grid gap-3 sm:grid-cols-3"><div className="skeleton h-28 rounded-[18px]" /><div className="skeleton h-28 rounded-[18px]" /><div className="skeleton h-28 rounded-[18px]" /></div>
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
        <Link to="/admin/users" className="card card-hover p-5 no-underline group">
          <div className="flex items-start justify-between">
            <div className="grid h-8 w-8 place-items-center rounded-xl bg-[var(--color-raised)] border text-[var(--color-ink-2)]"><FiUsers size={16} /></div>
            <span className="inline-flex items-center gap-1 text-xs text-[var(--color-ink-4)] group-hover:text-[var(--color-ink-3)]">{t('common.manage')} <FiArrowRight size={10} /></span>
          </div>
          <div className="mt-3">
            <div className="text-xs font-semibold tracking-wide text-[var(--color-ink-4)] uppercase">{t('overview.totalUsers')}</div>
            <div className="mt-1 text-[28px] font-semibold leading-none tracking-[-0.03em]">{users.length}</div>
            <div className="mt-1.5 text-xs leading-4 text-[var(--color-ink-3)] line-clamp-1">{admins} admin · {users.length - admins} user</div>
          </div>
        </Link>

        <Link to="/traffic" className="card card-hover p-5 no-underline group">
          <div className="flex items-start justify-between">
            <div className="grid h-8 w-8 place-items-center rounded-xl bg-[var(--color-raised)] border text-[var(--color-ink-2)]"><FiActivity size={16} /></div>
            <span className="inline-flex items-center gap-1 text-xs text-[var(--color-ink-4)] group-hover:text-[var(--color-ink-3)]">{t('common.viewAll')} <FiArrowRight size={10} /></span>
          </div>
          <div className="mt-3">
            <div className="text-xs font-semibold tracking-wide text-[var(--color-ink-4)] uppercase">{t('traffic.requests')} (24h)</div>
            <div className="mt-1 text-[28px] font-semibold leading-none tracking-[-0.03em]">{connections}</div>
            <div className="mt-1.5 text-xs leading-4 text-[var(--color-ink-3)] line-clamp-1">
              {(topDomains.slice(0, 3).map((d) => d.name).join(', ') || '\u00A0')}
            </div>
          </div>
        </Link>

        <a href="/healthz?detailed=1" target="_blank" rel="noreferrer" className="card card-hover p-5 no-underline group">
          <div className="flex items-start justify-between">
            <div className="grid h-8 w-8 place-items-center rounded-xl bg-[var(--color-raised)] border text-[var(--color-ink-2)]"><FiServer size={16} /></div>
            <span className={`inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-[11px] font-medium border ${dbOk ? 'bg-[var(--color-emerald-soft)] text-emerald-700' : 'bg-red-50 text-red-700'}`} style={{ borderColor: dbOk ? '#bbf7d0' : '#fecaca' }}>
              <span className={`h-1.5 w-1.5 rounded-sm ${dbOk ? 'bg-emerald-500' : 'bg-red-500'}`} /> {dbOk ? t('overview.ok') : t('overview.degraded')}
            </span>
          </div>
          <div className="mt-3">
            <div className="text-xs font-semibold tracking-wide text-[var(--color-ink-4)] uppercase">{t('overview.health')}</div>
            <div className="mt-1 text-lg font-semibold leading-tight tracking-[-0.02em] mono">{health.version || '–'}</div>
            <div className="mt-1.5 text-xs leading-4 text-[var(--color-ink-3)] line-clamp-1">
              {health.host || ''}{health.vps_ip ? ` · ${health.vps_ip}` : ''}
            </div>
          </div>
        </a>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="card overflow-hidden">
          <div className="flex items-center justify-between border-b px-5 py-3">
            <h3 className="text-sm font-semibold tracking-tight flex items-center gap-1.5"><FiActivity size={14} /> {t('traffic.recent')}</h3>
            <Link to="/traffic" className="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-brand)] no-underline hover:underline">{t('common.viewAll')} <FiArrowRight size={10} /></Link>
          </div>
          {recent.length === 0 ? (
            <p className="p-8 text-center text-sm text-[var(--color-ink-3)]">{t('traffic.noRecent')}</p>
          ) : (
            <div className="divide-y" style={{ borderColor: 'var(--color-border)' }}>
              {recent.slice(0, 6).map((l) => (
                <div key={l.id} className="flex items-center gap-3 px-5 py-3">
                  <span className="mono min-w-0 flex-1 truncate text-xs text-[var(--color-ink-2)]">{l.ip}</span>
                  <span className="hidden sm:inline shrink-0 rounded-md border bg-[var(--color-raised)] px-2 py-0.5 text-[11px] text-[var(--color-ink-3)]">{l.username || '—'}</span>
                  <span className="mono min-w-0 flex-1 truncate text-xs text-[var(--color-ink-4)]">{l.domain}</span>
                  <span className="shrink-0 text-xs text-[var(--color-ink-4)]">{new Date(l.created_at).toLocaleTimeString()}</span>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="card overflow-hidden">
          <div className="flex items-center justify-between border-b px-5 py-3">
            <h3 className="text-sm font-semibold tracking-tight flex items-center gap-1.5"><FiShield size={14} /> {t('overview.users')}</h3>
            <Link to="/admin/users" className="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-brand)] no-underline hover:underline">{t('common.manage')} <FiArrowRight size={10} /></Link>
          </div>
          {users.length === 0 ? (
            <div className="p-8 text-center text-sm text-[var(--color-ink-3)]">No users found.</div>
          ) : (
            <div className="table-wrap">
              <table className="table">
                <thead><tr><th>User</th><th>Role</th><th>Last login</th></tr></thead>
                <tbody>
                  {users.slice(0, 6).map((u) => (
                    <tr key={u.id}>
                      <td className="font-medium">{u.username}</td>
                      <td><span className="badge" style={{ background: u.role === 'admin' ? 'var(--color-ink)' : 'var(--color-raised)', color: u.role === 'admin' ? 'var(--color-bg)' : 'var(--color-ink-2)', borderColor: u.role === 'admin' ? 'var(--color-ink)' : 'var(--color-border)' }}>{u.role}</span></td>
                      <td className="text-xs text-[var(--color-ink-3)]">{u.last_login ? new Date(u.last_login).toLocaleString() : '-'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
