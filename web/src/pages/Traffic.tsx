import { useState, useEffect, useMemo } from 'react'
import { api } from '../api'
import type { AdminTrafficResponse, UserTrafficResponse, NameCount } from '../api'
import { useAuth } from '../App'
import { useI18n } from '../lib/i18n'
import BarChart from '../components/charts/BarChart'
import { FiActivity, FiClock, FiUsers, FiGlobe, FiRefreshCw } from 'react-icons/fi'

type SortKey = 'name' | 'count'

function SortableTop({ title, rows }: { title: string; rows: NameCount[] }) {
  const { t } = useI18n()
  const [key, setKey] = useState<SortKey>('count')
  const [asc, setAsc] = useState(false)
  const sorted = useMemo(() => {
    const out = [...rows]
    out.sort((a, b) => (key === 'count' ? a.count - b.count : a.name.localeCompare(b.name)))
    return asc ? out : out.reverse()
  }, [rows, key, asc])
  const toggle = (k: SortKey) => {
    if (key === k) setAsc((v) => !v)
    else { setKey(k); setAsc(k === 'name') }
  }
  return (
    <div className="card p-0 overflow-hidden">
      <h3 className="border-b px-4 py-3 text-sm font-semibold">{title}</h3>
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th className="cursor-pointer select-none" onClick={() => toggle('name')}>{t('traffic.domainCol')} ⇅</th>
              <th className="w-24 cursor-pointer select-none text-right" onClick={() => toggle('count')}>{t('traffic.countCol')} ⇅</th>
            </tr>
          </thead>
          <tbody>
            {sorted.length === 0 ? (
              <tr><td colSpan={2} className="p-4 text-center text-sm text-[var(--color-ink-3)]">{t('traffic.noData')}</td></tr>
            ) : sorted.map((r) => (
              <tr key={r.name}>
                <td className="mono text-xs font-medium break-all">{r.name}</td>
                <td className="text-right mono text-xs">{r.count}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

export default function Traffic() {
  const { user } = useAuth()
  const { t } = useI18n()
  const [range, setRange] = useState('24h')
  const [admin, setAdmin] = useState<AdminTrafficResponse | null>(null)
  const [mine, setMine] = useState<UserTrafficResponse | null>(null)
  const [loading, setLoading] = useState(true)

  const load = async () => {
    setLoading(true)
    if (user?.role === 'admin') {
      const r = await api.traffic(range)
      setAdmin(r.ok ? r : null)
    } else {
      const r = await api.myTraffic(range)
      setMine(r.ok ? r : null)
    }
    setLoading(false)
  }
  useEffect(() => { load() }, [user?.role, range])

  const isAdmin = user?.role === 'admin'
  const statCards = isAdmin
    ? [
        { icon: FiActivity, label: t('traffic.requests'), value: admin?.connections ?? 0 },
        { icon: FiUsers, label: t('traffic.uniqueIps'), value: admin?.unique_ips ?? 0 },
        { icon: FiGlobe, label: t('traffic.totalUsers'), value: admin?.total_users ?? 0 },
        { icon: FiClock, label: t('traffic.uptime'), value: fmtUptime(admin?.uptime_seconds) },
      ]
    : [
        { icon: FiActivity, label: t('traffic.requests'), value: mine?.total_requests ?? 0 },
        { icon: FiGlobe, label: t('traffic.uniqueDomains') || 'domains', value: mine?.unique_domains ?? 0 },
      ]

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-[22px] font-semibold tracking-[-0.02em] flex items-center gap-2"><FiActivity size={20} /> {t('traffic.title')}</h1>
          <p className="mt-1 text-sm text-[var(--color-ink-3)]">{isAdmin ? t('traffic.subtitleAdmin') : t('traffic.subtitleUser')}</p>
        </div>
        <div className="flex items-center gap-1.5">
          {['24h', '7d', '30d'].map((r) => (
            <button key={r} onClick={() => setRange(r)}
              className={`btn btn-sm ${range === r ? 'btn-primary' : 'btn-outline'}`}>
              {t(`traffic.range${r.charAt(0).toUpperCase()}${r.slice(1)}` as Parameters<typeof t>[0])}
            </button>
          ))}
          <button onClick={load} className="btn btn-outline btn-sm inline-flex items-center gap-1"><FiRefreshCw size={12} /> {t('common.refresh')}</button>
        </div>
      </div>

      {loading ? (
        <div className="space-y-4"><div className="grid gap-3 sm:grid-cols-3">{[0, 1, 2].map((i) => <div key={i} className="skeleton h-24 rounded-xl" />)}</div><div className="skeleton h-48 rounded-xl" /></div>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-3">
            {statCards.map(({ icon: Icon, label, value }) => (
              <div key={label} className="card p-5">
                <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-[var(--color-ink-3)]"><Icon size={13} /> {label}</div>
                <div className="mono mt-2 text-2xl font-semibold">{value}</div>
              </div>
            ))}
          </div>

          <div className="card p-4 sm:p-5">
            <h3 className="text-sm font-semibold">{t('traffic.connectionsChart')}</h3>
            <div className="mt-3">
              <BarChart buckets={(isAdmin ? admin?.buckets : mine?.buckets) ?? []} label={t('traffic.noData')} />
            </div>
          </div>

          {isAdmin && (
            <>
              <div className="grid gap-4 lg:grid-cols-2">
                <SortableTop title={t('traffic.topDomains')} rows={admin?.top_domains ?? []} />
                <SortableTop title={t('traffic.perUser')} rows={admin?.top_users ?? []} />
              </div>

              <div className="card p-0 overflow-hidden">
                <h3 className="border-b px-4 py-3 text-sm font-semibold">{t('traffic.recent')}</h3>
                {(admin?.recent ?? []).length === 0 ? (
                  <p className="p-6 text-center text-sm text-[var(--color-ink-3)]">{t('traffic.noRecent')}</p>
                ) : (
                  <div className="table-wrap"><table className="table">
                    <thead><tr><th>IP</th><th>{t('traffic.userCol')}</th><th>{t('traffic.domainCol')}</th><th>{t('traffic.timeCol')}</th></tr></thead>
                    <tbody>
                      {admin!.recent!.map((l) => (
                        <tr key={l.id}>
                          <td className="mono text-xs">{l.ip}</td>
                          <td className="text-xs">{l.username || '—'}</td>
                          <td className="mono text-xs break-all">{l.domain}</td>
                          <td className="whitespace-nowrap text-xs text-[var(--color-ink-3)]">{new Date(l.created_at).toLocaleString()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table></div>
                )}
              </div>
            </>
          )}

          {!isAdmin && (
            <div className="card p-0 overflow-hidden">
              <h3 className="border-b px-4 py-3 text-sm font-semibold">{t('traffic.recent')}</h3>
              {(mine?.recent ?? []).length === 0 ? (
                <p className="p-6 text-center text-sm text-[var(--color-ink-3)]">{t('traffic.noRecent')}</p>
              ) : (
                <ul className="divide-y divide-[var(--color-border)]">
                  {mine!.recent!.map((l) => (
                    <li key={l.id} className="flex items-center justify-between gap-3 px-4 py-2.5">
                      <span className="mono truncate text-sm">{l.domain}</span>
                      <span className="shrink-0 text-xs text-[var(--color-ink-3)]">{new Date(l.created_at).toLocaleString()}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}

function fmtUptime(sec?: number): string {
  if (!sec) return '–'
  const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60)
  return d > 0 ? `${d}d ${h}h` : h > 0 ? `${h}h ${m}m` : `${m}m`
}
