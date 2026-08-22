import { useState, useEffect } from 'react'
import { api } from '../api'
import { useAuth } from '../App'
import { useI18n } from '../lib/i18n'
import { FiActivity, FiUsers, FiSmartphone, FiGlobe, FiClock, FiBarChart2, FiServer } from 'react-icons/fi'

export default function Traffic() {
  const { user } = useAuth()
  const { t } = useI18n()
  const [data, setData] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const load = async () => {
      setLoading(true)
      const isAdmin = user?.role === 'admin'
      const r = isAdmin ? await api.traffic() : await api.myTraffic()
      if (r.ok) setData(r)
      else setError(r.error || 'Failed to load traffic')
      setLoading(false)
    }
    load()
  }, [user])

  if (loading) return <div className="space-y-4"><div className="skeleton h-8 w-40" /><div className="grid gap-3 sm:grid-cols-3"><div className="skeleton h-28 rounded-xl" /><div className="skeleton h-28 rounded-xl" /><div className="skeleton h-28 rounded-xl" /></div><div className="skeleton h-64 rounded-xl" /></div>

  if (error) return <div className="card p-6 text-sm text-[var(--color-rose)]">{error}</div>

  const isAdmin = user?.role === 'admin'

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-[22px] font-semibold tracking-[-0.02em] flex items-center gap-2"><FiActivity size={20} /> {t('traffic.title')}</h1>
        <p className="mt-1 text-sm text-[var(--color-ink-3)]">{isAdmin ? t('traffic.subtitleAdmin') : t('traffic.subtitleUser')}</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="card p-5">
          <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-4)]"><FiSmartphone size={12} /> {t('traffic.devices')}</div>
          <div className="mt-2 text-[28px] font-semibold leading-none">{isAdmin ? data?.total_devices ?? 0 : data?.devices ?? 0}</div>
          <div className="mt-1 text-xs text-[var(--color-ink-3)]">{isAdmin ? `${data?.allowlisted ?? 0} ${t('traffic.allowlisted')} / ${data?.restricted ?? 0} ${t('traffic.restricted')}` : `${t('common.status')}: ${data?.unlimited ? t('users.unlimited') : (data?.rate_limit ?? 0) + '/min'}`}</div>
        </div>
        <div className="card p-5">
          <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-4)]"><FiBarChart2 size={12} /> {t('traffic.requests')}</div>
          <div className="mt-2 text-[28px] font-semibold leading-none">{data?.total_requests ?? 0}</div>
          <div className="mt-1 text-xs text-[var(--color-ink-3)]">{isAdmin ? `${data?.total_users ?? 0} ${t('traffic.totalUsers')}` : t('common.requests')}</div>
        </div>
        <div className="card p-5">
          <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-4)]"><FiClock size={12} /> {t('traffic.uptime')}</div>
          <div className="mt-2 text-[18px] font-semibold leading-none">{isAdmin ? `${Math.floor((data?.uptime_seconds ?? 0)/3600)}h ${Math.floor(((data?.uptime_seconds ?? 0)%3600)/60)}m` : `${data?.devices ?? 0} ${t('traffic.devices')}`}</div>
          <div className="mt-1 text-xs text-[var(--color-ink-3)]">{isAdmin ? `${t('traffic.version')} ${data?.version ?? ''}` : t('traffic.subtitleUser')}</div>
        </div>
      </div>

      {isAdmin && data?.per_user && (
        <div className="card p-0 overflow-hidden">
          <div className="px-4 py-3 border-b flex items-center gap-2 text-sm font-semibold"><FiUsers size={14} /> {t('traffic.perUser')}</div>
          <div className="table-wrap">
            <table className="table">
              <thead><tr><th>{t('common.user')}</th><th>{t('traffic.devices')}</th><th>{t('traffic.requests')}</th></tr></thead>
              <tbody>
                {(data.per_user as any[]).map((u: any) => (
                  <tr key={u.username}>
                    <td className="font-medium">{u.username}</td>
                    <td>{u.devices}</td>
                    <td>{u.requests}</td>
                  </tr>
                ))}
                {(!data.per_user || data.per_user.length === 0) && <tr><td colSpan={3} className="text-center text-xs text-[var(--color-ink-3)] py-6">{t('traffic.noData')}</td></tr>}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <div className="card p-0 overflow-hidden">
        <div className="px-4 py-3 border-b flex items-center gap-2 text-sm font-semibold"><FiGlobe size={14} /> {t('traffic.recent')} {data?.recent ? `(${data.recent.length})` : ''}</div>
        {!data?.recent || data.recent.length === 0 ? (
          <div className="p-8 text-center text-sm text-[var(--color-ink-3)]">{t('traffic.noRecent')}</div>
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead><tr><th>{t('common.domain')}</th><th>{t('common.device')}</th><th>{t('common.time')}</th></tr></thead>
              <tbody>
                {(data.recent as any[]).slice(0, 20).map((r: any, i: number) => (
                  <tr key={i}>
                    <td className="mono text-xs">{r.domain || r.Domain || '-'}</td>
                    <td className="text-xs text-[var(--color-ink-3)]">{r.device_id || r.DeviceID || '-'}</td>
                    <td className="text-xs text-[var(--color-ink-3)]">{r.created_at ? new Date(r.created_at).toLocaleString() : r.CreatedAt ? new Date(r.CreatedAt).toLocaleString() : '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {isAdmin && (
        <div className="card p-4">
          <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiServer size={14} /> {t('traffic.system')}</h3>
          <div className="mt-2 grid gap-2 text-xs text-[var(--color-ink-3)]">
            <div>{t('common.version')}: <span className="font-mono text-[var(--color-ink)]">{data?.version}</span></div>
            <div>{t('common.host')}: <span className="font-mono text-[var(--color-ink)]">{data?.host || '-'}</span> • {t('common.vpsIp')}: <span className="font-mono text-[var(--color-ink)]">{data?.vps_ip || '-'}</span></div>
          </div>
        </div>
      )}
    </div>
  )
}
