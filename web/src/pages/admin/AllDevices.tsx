import { useState, useEffect, useMemo } from 'react'
import { api } from '../../api'
import type { DeviceData } from '../../api'
import { useToast } from '../../components/ui/Toast'
import { useI18n } from '../../lib/i18n'
import { FiSearch, FiSmartphone, FiFilter, FiChevronLeft, FiChevronRight } from 'react-icons/fi'

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
const PAGE_SIZE = 12
export default function AllDevices() {
  const [devices, setDevices] = useState<DeviceData[]>([])
  const [loading, setLoading] = useState(true)
  const [q, setQ] = useState('')
  const [type, setType] = useState<string>('all')
  const [sort, setSort] = useState<'recent' | 'oldest' | 'ip'>('recent')
  const [page, setPage] = useState(1)
  const { error } = useToast()
  const { t } = useI18n()

  useEffect(() => {
    api.allDevices().then((r) => { if (r.ok && r.devices) setDevices(r.devices); else if (!r.ok) error(r.error || 'Failed') }).catch((e) => error(e instanceof Error ? e.message : 'Failed')).finally(() => setLoading(false))
  }, [])

  const types = useMemo(() => { const s = new Set(devices.map((d) => d.device_type || 'Unknown')); return ['all', ...Array.from(s).sort()] }, [devices])
  const filtered = useMemo(() => {
    let out = devices.slice()
    if (type !== 'all') out = out.filter((d) => (d.device_type || 'Unknown') === type)
    if (q.trim()) {
      const needle = q.trim().toLowerCase()
      out = out.filter((d) => d.ip.toLowerCase().includes(needle) || d.ja3_hash.toLowerCase().includes(needle) || (d.device_type || '').toLowerCase().includes(needle) || (d.user_agent || '').toLowerCase().includes(needle))
    }
    if (sort === 'recent') out.sort((a, b) => +new Date(b.last_seen) - +new Date(a.last_seen))
    else if (sort === 'oldest') out.sort((a, b) => +new Date(a.first_seen) - +new Date(b.first_seen))
    else out.sort((a, b) => a.ip.localeCompare(b.ip))
    return out
  }, [devices, q, type, sort])
  const paged = useMemo(() => { const start = (page - 1) * PAGE_SIZE; return filtered.slice(start, start + PAGE_SIZE) }, [filtered, page])
  useEffect(() => setPage(1), [q, type, sort])
  if (loading) return <div className="space-y-4"><div className="skeleton h-8 w-40" /><div className="skeleton h-14 rounded-[18px]" /><div className="skeleton h-72 rounded-[18px]" /></div>
  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-[22px] font-semibold tracking-[-0.02em] flex items-center gap-2"><FiSmartphone size={20} /> {t('devices.title')}</h1>
          <p className="mt-1 text-sm text-[var(--color-ink-3)]">{t('devices.subtitle', { filtered: filtered.length, total: devices.length })}</p>
        </div>
        <button onClick={() => window.location.reload()} className="btn btn-outline btn-sm">{t('common.refresh')}</button>
      </div>
      <div className="card p-4">
        <div className="grid gap-3 sm:grid-cols-[1.5fr_0.8fr_0.7fr]">
          <div className="field-wrap">
            <span className="field-icon"><FiSearch size={14} /></span>
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('devices.filterPh')} className="input input-with-icon" />
          </div>
          <div className="field-wrap">
            <span className="field-icon"><FiFilter size={12} /></span>
            <select value={type} onChange={(e) => setType(e.target.value)} className="input input-with-icon">
              {types.map((t) => <option key={t} value={t}>{t === 'all' ? 'All types' : t}</option>)}
            </select>
          </div>
          <select value={sort} onChange={(e) => setSort(e.target.value as never)} className="input">
            <option value="recent">{t('devices.mostRecent')}</option>
            <option value="oldest">{t('devices.oldestFirst')}</option>
            <option value="ip">{t('devices.ipAddr')}</option>
          </select>
        </div>
      </div>
      <div className="card p-0 overflow-hidden">
        {filtered.length === 0 ? (
          <div className="p-10 text-center"><div className="mx-auto grid h-10 w-10 place-items-center rounded-2xl border bg-[var(--color-raised)]"><FiSmartphone size={18} /></div><p className="mt-3 text-sm font-medium">{t('devices.noFoundTitle')}</p><p className="mt-1 text-xs text-[var(--color-ink-3)]">{t('devices.noFoundDesc')}</p></div>
        ) : (
          <>
            <div className="table-wrap"><table className="table"><thead><tr><th>Type</th><th>IP</th><th className="hidden md:table-cell">JA3 hash</th><th className="hidden xl:table-cell">User agent</th><th>Last seen</th></tr></thead><tbody>{paged.map((d) => { const c = badge(d.device_type); return <tr key={d.id}><td><span className="badge" style={{ background: c.bg, color: c.fg, borderColor: c.border }}>{d.device_type || 'Unknown'}</span></td><td className="mono text-xs font-medium">{d.ip}</td><td className="hidden md:table-cell mono text-[11px] text-[var(--color-ink-3)] max-w-[14ch] truncate">{d.ja3_hash}</td><td className="hidden xl:table-cell text-xs text-[var(--color-ink-3)] max-w-[28ch] truncate">{d.user_agent || '-'}</td><td className="text-xs text-[var(--color-ink-3)] whitespace-nowrap">{new Date(d.last_seen).toLocaleString()}</td></tr> })}</tbody></table></div>
            {totalPages > 1 && <div className="flex items-center justify-between border-t px-4 py-3 text-xs"><span className="text-[var(--color-ink-4)]">{t('devices.pageOf', { page, total: totalPages, filtered: filtered.length })}</span><div className="flex gap-1.5"><button onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page === 1} className="btn btn-outline btn-sm inline-flex items-center gap-1"><FiChevronLeft size={12} /> {t('devices.prev')}</button><button onClick={() => setPage((p) => Math.min(totalPages, p + 1))} disabled={page === totalPages} className="btn btn-outline btn-sm inline-flex items-center gap-1">{t('devices.next')} <FiChevronRight size={12} /></button></div></div>}
          </>
        )}
      </div>
    </div>
  )
}
