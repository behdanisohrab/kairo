import { useState, useEffect } from 'react'
import { api } from '../api'
import { useAuth } from '../App'
import { FiActivity, FiUsers, FiSmartphone, FiGlobe, FiClock, FiBarChart2, FiServer } from 'react-icons/fi'

export default function Traffic() {
  const { user } = useAuth()
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
        <h1 className="text-[22px] font-semibold tracking-[-0.02em] flex items-center gap-2"><FiActivity size={20} /> Traffic Status</h1>
        <p className="mt-1 text-sm text-[var(--color-ink-3)]">{isAdmin ? 'Global traffic and per-user breakdown' : 'Your traffic overview'}</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <div className="card p-5">
          <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-4)]"><FiSmartphone size={12} /> Devices</div>
          <div className="mt-2 text-[28px] font-semibold leading-none">{isAdmin ? data?.total_devices ?? 0 : data?.devices ?? 0}</div>
          <div className="mt-1 text-xs text-[var(--color-ink-3)]">{isAdmin ? `${data?.allowlisted ?? 0} allowlisted / ${data?.restricted ?? 0} restricted` : `Rate limit: ${data?.unlimited ? 'Unlimited' : (data?.rate_limit ?? 0) + '/min'}`}</div>
        </div>
        <div className="card p-5">
          <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-4)]"><FiBarChart2 size={12} /> Requests</div>
          <div className="mt-2 text-[28px] font-semibold leading-none">{data?.total_requests ?? 0}</div>
          <div className="mt-1 text-xs text-[var(--color-ink-3)]">{isAdmin ? `${data?.total_users ?? 0} users` : 'Total DNS/tunnel requests'}</div>
        </div>
        <div className="card p-5">
          <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-4)]"><FiClock size={12} /> Uptime</div>
          <div className="mt-2 text-[18px] font-semibold leading-none">{isAdmin ? `${Math.floor((data?.uptime_seconds ?? 0)/3600)}h ${Math.floor(((data?.uptime_seconds ?? 0)%3600)/60)}m` : `${data?.devices ?? 0} devices active`}</div>
          <div className="mt-1 text-xs text-[var(--color-ink-3)]">{isAdmin ? `v${data?.version ?? ''}` : 'Since last reset'}</div>
        </div>
      </div>

      {isAdmin && data?.per_user && (
        <div className="card p-0 overflow-hidden">
          <div className="px-4 py-3 border-b flex items-center gap-2 text-sm font-semibold"><FiUsers size={14} /> Per-user breakdown</div>
          <div className="table-wrap">
            <table className="table">
              <thead><tr><th>User</th><th>Devices</th><th>Requests</th></tr></thead>
              <tbody>
                {(data.per_user as any[]).map((u: any) => (
                  <tr key={u.username}>
                    <td className="font-medium">{u.username}</td>
                    <td>{u.devices}</td>
                    <td>{u.requests}</td>
                  </tr>
                ))}
                {(!data.per_user || data.per_user.length === 0) && <tr><td colSpan={3} className="text-center text-xs text-[var(--color-ink-3)] py-6">No data</td></tr>}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <div className="card p-0 overflow-hidden">
        <div className="px-4 py-3 border-b flex items-center gap-2 text-sm font-semibold"><FiGlobe size={14} /> Recent requests {data?.recent ? `(${data.recent.length})` : ''}</div>
        {!data?.recent || data.recent.length === 0 ? (
          <div className="p-8 text-center text-sm text-[var(--color-ink-3)]">No recent traffic</div>
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead><tr><th>Domain</th><th>Device</th><th>Time</th></tr></thead>
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
          <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiServer size={14} /> System</h3>
          <div className="mt-2 grid gap-2 text-xs text-[var(--color-ink-3)]">
            <div>Version: <span className="font-mono text-[var(--color-ink)]">{data?.version}</span></div>
            <div>Host: <span className="font-mono text-[var(--color-ink)]">{data?.host || '-'}</span> • VPS IP: <span className="font-mono text-[var(--color-ink)]">{data?.vps_ip || '-'}</span></div>
          </div>
        </div>
      )}
    </div>
  )
}
