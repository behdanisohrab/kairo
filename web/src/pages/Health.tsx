import { useState, useEffect } from 'react'
import { api } from '../api'
import { FiActivity, FiServer, FiClock, FiShield, FiGlobe, FiRefreshCw, FiCheckCircle, FiAlertTriangle } from 'react-icons/fi'

export default function Health() {
  const [health, setHealth] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const res = await fetch('/healthz?detailed=1', { headers: { Accept: 'application/json' } })
      const data = await res.json()
      setHealth(data)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load health')
    }
    setLoading(false)
  }
  useEffect(() => { load() }, [])

  // Also try to fetch status if admin (optional)
  const [status, setStatus] = useState<any>(null)
  useEffect(() => {
    api.status().then(r => { if (r.ok) setStatus(r) }).catch(() => {})
  }, [])

  if (loading) return <div className="space-y-4"><div className="skeleton h-8 w-40" /><div className="skeleton h-32 rounded-xl" /><div className="skeleton h-48 rounded-xl" /></div>

  const checks = health?.checks || {}
  const isOk = health?.ok !== false

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-[22px] font-semibold tracking-[-0.02em] flex items-center gap-2"><FiActivity size={20} /> Health</h1>
          <p className="mt-1 text-sm text-[var(--color-ink-3)]">Service status and diagnostics</p>
        </div>
        <button onClick={load} className="btn btn-outline btn-sm inline-flex items-center gap-1"><FiRefreshCw size={12} /> Refresh</button>
      </div>

      <div className={`card p-5 border-2 ${isOk ? 'border-emerald-200 bg-emerald-50/50' : 'border-amber-200 bg-amber-50/50'}`} style={{ borderColor: isOk ? '#bbf7d0' : '#fde68a', background: isOk ? 'var(--color-emerald-soft)' : 'var(--color-amber-soft)' }}>
        <div className="flex items-center gap-3">
          <span className={`grid h-10 w-10 place-items-center rounded-xl ${isOk ? 'bg-emerald-100 text-emerald-700' : 'bg-amber-100 text-amber-700'}`}>
            {isOk ? <FiCheckCircle size={20} /> : <FiAlertTriangle size={20} />}
          </span>
          <div>
            <div className="text-sm font-semibold" style={{ color: isOk ? 'var(--color-emerald)' : 'var(--color-amber)' }}>{isOk ? 'Operational' : 'Degraded'}</div>
            <div className="text-xs text-[var(--color-ink-3)]">{health?.status?.overall || (isOk ? 'ok' : 'degraded')} • {health?.version || ''} • uptime {health?.uptime || health?.uptime_seconds ? `${Math.floor((health?.uptime_seconds||0)/3600)}h` : '-'}</div>
          </div>
          <span className="ms-auto text-xs font-mono text-[var(--color-ink-4)]">{health?.uptime || ''}</span>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="card p-4">
          <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiServer size={14} /> Core</h3>
          <div className="mt-3 space-y-2 text-sm">
            <div className="flex justify-between"><span className="text-[var(--color-ink-3)]">Host</span><span className="font-mono text-xs">{health?.host || '-'}</span></div>
            <div className="flex justify-between"><span className="text-[var(--color-ink-3)]">VPS IP</span><span className="font-mono text-xs">{health?.vps_ip || '-'}</span></div>
            <div className="flex justify-between"><span className="text-[var(--color-ink-3)]">Admin URL</span><span className="font-mono text-xs truncate max-w-[150px]">{health?.admin_url || '-'}</span></div>
            <div className="flex justify-between"><span className="text-[var(--color-ink-3)]">DoH URL</span><span className="font-mono text-xs truncate max-w-[150px]">{health?.doh_url || '-'}</span></div>
            <div className="flex justify-between"><span className="text-[var(--color-ink-3)]">Version</span><span className="font-mono text-xs">{health?.version || '-'}</span></div>
            <div className="flex justify-between"><span className="text-[var(--color-ink-3)]">Uptime</span><span className="text-xs">{health?.uptime || '-'}</span></div>
          </div>
        </div>

        <div className="card p-4">
          <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiShield size={14} /> Checks</h3>
          <div className="mt-3 space-y-2">
            <div className="flex items-center justify-between rounded-lg border px-3 py-2" style={{ background: checks.database?.ok !== false ? 'var(--color-emerald-soft)' : 'var(--color-rose-soft)', borderColor: 'var(--color-border)' }}>
              <span className="text-sm">Database</span>
              <span className={`text-xs font-semibold ${checks.database?.ok !== false ? 'text-emerald-600' : 'text-rose-600'}`}>{checks.database?.ok !== false ? 'OK' : 'FAIL'}</span>
            </div>
            <div className="flex items-center justify-between rounded-lg border bg-[var(--color-raised)] px-3 py-2">
              <span className="text-sm flex items-center gap-1"><FiGlobe size={12} /> Allowlisted</span><span className="font-mono text-sm">{checks.allowlisted ?? health?.checks?.allowlisted ?? '-'}</span>
            </div>
            <div className="flex items-center justify-between rounded-lg border bg-[var(--color-raised)] px-3 py-2">
              <span className="text-sm flex items-center gap-1"><FiServer size={12} /> Restricted</span><span className="font-mono text-sm">{checks.restricted ?? health?.checks?.restricted ?? '-'}</span>
            </div>
            {error && <div className="text-xs text-[var(--color-rose)]">{error}</div>}
          </div>
        </div>
      </div>

      <div className="card p-4">
        <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiClock size={14} /> Endpoints</h3>
        <div className="mt-3 grid gap-2 text-xs">
          <div className="flex items-center justify-between"><code className="inline">GET /healthz</code><span className="text-[var(--color-ink-3)]">plain ok (no auth)</span></div>
          <div className="flex items-center justify-between"><code className="inline">GET /healthz?detailed=1</code><span className="text-[var(--color-ink-3)]">JSON detailed (no auth)</span></div>
          <div className="flex items-center justify-between"><code className="inline">GET /api/public-config</code><span className="text-[var(--color-ink-3)]">any auth</span></div>
          <div className="flex items-center justify-between"><code className="inline">GET /api/status</code><span className="text-[var(--color-ink-3)]">admin</span></div>
          <div className="flex items-center justify-between"><code className="inline">GET /metrics</code><span className="text-[var(--color-ink-3)]">127.0.0.1:9090</span></div>
        </div>
      </div>

      {status && status.ok && (
        <div className="card p-4">
          <h3 className="text-sm font-semibold">Status API (admin)</h3>
          <pre className="mt-2 overflow-auto rounded-xl border bg-[var(--color-raised)] p-3 text-xs leading-5 font-mono text-[var(--color-ink-2)]">{JSON.stringify(status, null, 2)}</pre>
        </div>
      )}
    </div>
  )
}
