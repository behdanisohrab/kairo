import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'
import { api } from '../../api'
import type { DeviceData } from '../../api'
import { useToast } from '../../components/ui/Toast'
import { useI18n } from '../../lib/i18n'
import { FiArrowLeft, FiSmartphone } from 'react-icons/fi'
function badge(t: string) {
  const m: Record<string, { bg: string; fg: string; border: string }> = {
    Desktop: { bg: 'var(--color-brand-soft)', fg: 'var(--color-brand)', border: '#dbe0ff' },
    Android: { bg: 'var(--color-emerald-soft)', fg: 'var(--color-emerald)', border: '#bbf7d0' },
    iOS: { bg: 'var(--color-violet-soft)', fg: 'var(--color-violet)', border: '#e9d5ff' },
    Tablet: { bg: '#ecfeff', fg: '#0891b2', border: '#a5f3fc' },
    Bot: { bg: 'var(--color-rose-soft)', fg: 'var(--color-rose)', border: '#fecaca' },
  }
  return m[t] || { bg: 'var(--color-raised)', fg: 'var(--color-ink-3)', border: 'var(--color-border)' }
}
export default function UserDevices() {
  const { id } = useParams<{ id: string }>()
  const [devices, setDevices] = useState<DeviceData[]>([])
  const [loading, setLoading] = useState(true)
  const { error } = useToast()
  const { t } = useI18n()
  useEffect(() => {
    if (!id) return
    api.getUserDevices(parseInt(id)).then((r) => { if (r.ok && r.devices) setDevices(r.devices); else if (!r.ok) error(r.error || 'Failed') }).catch((e) => error(e instanceof Error ? e.message : 'Failed')).finally(() => setLoading(false))
  }, [id])
  if (loading) return <div className="space-y-4"><div className="skeleton h-6 w-32" /><div className="skeleton h-8 w-48" /><div className="skeleton h-64 rounded-[18px]" /></div>
  return (
    <div className="space-y-4">
      <div>
        <Link to="/admin/users" className="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--color-brand)] no-underline hover:underline"><FiArrowLeft size={12} /> {t('common.backToUsers')}</Link>
        <h1 className="mt-1 text-[22px] font-semibold tracking-[-0.02em] flex items-center gap-2"><FiSmartphone size={20} /> {t('devices.userTitle', { id: id || '' })}</h1>
        <p className="mt-1 text-sm text-[var(--color-ink-3)]">{t('devices.userSubtitle', { n: devices.length })}</p>
      </div>
      <div className="card p-0 overflow-hidden">
        {devices.length === 0 ? (
          <div className="p-10 text-center"><div className="mx-auto grid h-10 w-10 place-items-center rounded-2xl border bg-[var(--color-raised)]"><FiSmartphone size={18} /></div><p className="mt-3 text-sm font-medium">{t('devices.noRecordTitle')}</p><p className="mt-1 text-xs text-[var(--color-ink-3)]">{t('devices.noRecordDesc')}</p></div>
        ) : (
          <div className="table-wrap"><table className="table"><thead><tr><th>Type</th><th>IP</th><th className="hidden md:table-cell">JA3</th><th className="hidden lg:table-cell">User agent</th><th>Last seen</th></tr></thead><tbody>{devices.map((d) => { const c = badge(d.device_type); return <tr key={d.id}><td><span className="badge" style={{ background: c.bg, color: c.fg, borderColor: c.border }}>{d.device_type || 'Unknown'}</span></td><td className="mono text-xs font-medium">{d.ip}</td><td className="hidden md:table-cell mono text-[11px] text-[var(--color-ink-3)]">{d.ja3_hash}</td><td className="hidden lg:table-cell text-xs text-[var(--color-ink-3)] max-w-[26ch] truncate">{d.user_agent || '-'}</td><td className="text-xs text-[var(--color-ink-3)] whitespace-nowrap">{new Date(d.last_seen).toLocaleString()}</td></tr> })}</tbody></table></div>
        )}
      </div>
    </div>
  )
}
