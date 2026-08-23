import { useState, useEffect, useMemo } from 'react'
import { api } from '../../api'
import type { DomainRequest } from '../../api'
import { useToast } from '../../components/ui/Toast'
import { useI18n } from '../../lib/i18n'
import { FiGlobe, FiPlus, FiTrash2, FiCheck, FiX, FiSearch, FiSend, FiUpload, FiRefreshCw, FiFastForward, FiInfo } from 'react-icons/fi'

type Tab = 'proxied' | 'direct'

export default function Domains() {
  const [tab, setTab] = useState<Tab>('proxied')
  const [domains, setDomains] = useState<string[]>([])
  const [direct, setDirect] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')
  const [addOne, setAddOne] = useState('')
  const [adding, setAdding] = useState(false)
  const [bulk, setBulk] = useState('')
  const [bulkLoading, setBulkLoading] = useState(false)
  const [requests, setRequests] = useState<DomainRequest[]>([])
  const { success, error } = useToast()
  const { t } = useI18n()

  const list = tab === 'proxied' ? domains : direct

  const load = async () => {
    setLoading(true)
    const r = await api.restrictedList()
    if (r.ok && r.data) setDomains(r.data)
    else if (!r.ok) error(r.error || 'Failed to load domains')
    const dr = await api.directList()
    if (dr.ok && dr.data) setDirect(dr.data)
    else if (!dr.ok) error(dr.error || 'Failed to load direct domains')
    const rq = await api.listDomainRequests()
    if (rq.ok && rq.requests) setRequests(rq.requests)
    setLoading(false)
  }
  useEffect(() => { load() }, [])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return list
    return list.filter((d) => d.toLowerCase().includes(q))
  }, [list, query])

  const addTo = async (d: string) =>
    tab === 'proxied' ? api.addRestricted(d) : api.addDirect(d)
  const removeFrom = async (d: string) =>
    tab === 'proxied' ? api.removeRestricted(d) : api.removeDirect(d)

  const handleAddOne = async (e: React.FormEvent) => {
    e.preventDefault()
    const d = addOne.trim().toLowerCase()
    if (!d) return
    setAdding(true)
    const r = await addTo(d)
    if (r.ok) {
      success(`Added ${d}`)
      setAddOne('')
      load()
    } else error(r.error || 'Failed')
    setAdding(false)
  }

  const handleDelete = async (d: string) => {
    if (!confirm(`Remove ${d}?`)) return
    const r = await removeFrom(d)
    if (r.ok) { success(`Removed ${d}`); load() }
    else error(r.error || 'Failed')
  }

  const handleBulk = async () => {
    const targets = bulk.split('\n').map((s) => s.trim().toLowerCase()).filter(Boolean)
    if (!targets.length) return
    setBulkLoading(true)
    let ok = 0; let fail = 0
    for (const d of targets) {
      const r = await addTo(d)
      if (r.ok) ok++
      else fail++
    }
    success(`Added ${ok}, skipped ${fail}`)
    setBulk('')
    setBulkLoading(false)
    load()
  }

  const handleApprove = async (id: number) => {
    const r = await api.approveDomainRequest(id)
    if (r.ok) { success('Approved and added to proxy list'); load() }
    else error(r.error || 'Failed')
  }
  const handleReject = async (id: number) => {
    const r = await api.rejectDomainRequest(id)
    if (r.ok) { success('Rejected'); load() }
    else error(r.error || 'Failed')
  }

  const pending = requests.filter((r) => r.status === 'pending')

  if (loading) {
    return <div className="space-y-4"><div className="skeleton h-8 w-40" /><div className="skeleton h-24 rounded-xl" /><div className="skeleton h-64 rounded-xl" /></div>
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-[22px] font-semibold tracking-[-0.02em] flex items-center gap-2"><FiGlobe size={20} /> {t('domains.title')}</h1>
          <p className="mt-1 text-sm text-[var(--color-ink-3)]">
            {t(tab === 'proxied' ? 'domains.subtitle' : 'domains.subtitleDirect', { filtered: filtered.length, total: list.length })}
          </p>
        </div>
        <button onClick={load} className="btn btn-outline btn-sm inline-flex items-center gap-1"><FiRefreshCw size={12} /> {t('common.refresh')}</button>
      </div>

      {/* mode tabs */}
      <div className="flex gap-1.5">
        {([
          ['proxied', t('domains.tabProxied'), domains.length],
          ['direct', t('domains.tabDirect'), direct.length],
        ] as [Tab, string, number][]).map(([key, label, count]) => (
          <button
            key={key}
            onClick={() => setTab(key)}
            className={`btn btn-sm inline-flex items-center gap-1.5 ${tab === key ? 'btn-primary' : 'btn-outline'}`}
          >
            {key === 'proxied' ? <FiGlobe size={12} /> : <FiFastForward size={12} />}
            {label}
            <span className="mono text-[10px] opacity-70">{count}</span>
          </button>
        ))}
      </div>

      {tab === 'direct' && (
        <div className="card p-4 flex gap-3">
          <FiInfo size={16} className="shrink-0 mt-0.5 text-blue-500" />
          <p className="text-xs leading-relaxed text-[var(--color-ink-2)]">{t('domains.directNote')}</p>
        </div>
      )}

      <form onSubmit={handleAddOne} className="card p-4">
        <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiPlus size={14} /> {t('domains.addTitle')}</h3>
        <p className="mt-1 text-xs text-[var(--color-ink-3)]">{t('domains.addDesc')}</p>
        <div className="mt-3 flex gap-2">
          <div className="field-wrap flex-1">
            <span className="field-icon"><FiGlobe size={14} /></span>
            <input value={addOne} onChange={(e) => setAddOne(e.target.value)} placeholder={t('domains.placeholder')} className="input input-with-icon" />
          </div>
          <button type="submit" disabled={adding || !addOne.trim()} className="btn btn-primary inline-flex items-center gap-1"><FiPlus size={12} /> {adding ? t('domains.adding') : t('domains.addBtn')}</button>
        </div>
      </form>

      <div className="card p-4">
        <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiUpload size={14} /> {t('domains.bulkTitle')}</h3>
        <p className="mt-1 text-xs text-[var(--color-ink-3)]">{t('domains.bulkDesc')}</p>
        <textarea value={bulk} onChange={(e) => setBulk(e.target.value)} placeholder={t('domains.bulkPlaceholder')} rows={4} className="input mt-3 font-mono text-sm" />
        <button onClick={handleBulk} disabled={bulkLoading || !bulk.trim()} className="btn btn-secondary mt-3 inline-flex items-center gap-1"><FiCheck size={12} /> {bulkLoading ? t('domains.applying') : t('domains.apply')}</button>
      </div>

      <div className="card p-0 overflow-hidden">
        <div className="flex items-center gap-3 border-b px-4 py-3">
          <div className="field-wrap flex-1">
            <span className="field-icon"><FiSearch size={14} /></span>
            <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t('domains.filterPh')} className="input input-with-icon" />
          </div>
          <span className="text-xs text-[var(--color-ink-4)]">{filtered.length}</span>
        </div>
        {filtered.length === 0 ? (
          <div className="p-8 text-center text-sm text-[var(--color-ink-3)]">{t('domains.noDomains')}</div>
        ) : (
          <div className="divide-y" style={{ borderColor: 'var(--color-border)' }}>
            {filtered.map((d) => (
              <div key={d} className="flex items-center gap-3 px-4 py-2.5">
                <span className="mono flex-1 truncate text-sm">{d}</span>
                <button onClick={() => handleDelete(d)} className="btn btn-ghost btn-sm inline-flex items-center gap-1 text-[var(--color-rose)]"><FiTrash2 size={12} /> {t('domains.remove')}</button>
              </div>
            ))}
          </div>
        )}
      </div>

      {tab === 'proxied' && (
        <div className="card p-0 overflow-hidden">
          <div className="px-4 py-3 border-b flex items-center justify-between">
            <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiSend size={14} /> {t('domains.requestsTitle')} {pending.length > 0 && <span className="ml-1 rounded-md bg-amber-100 px-1.5 py-0.5 text-xs text-amber-800">{t('domains.pending', { n: pending.length })}</span>}</h3>
            <span className="text-xs text-[var(--color-ink-4)]">{t('domains.total', { n: requests.length })}</span>
          </div>
          {pending.length === 0 ? (
            <div className="p-6 text-center text-sm text-[var(--color-ink-3)]">No pending requests</div>
          ) : (
            <div className="divide-y" style={{ borderColor: 'var(--color-border)' }}>
              {pending.map((r) => (
                <div key={r.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                  <span className="mono text-sm font-medium">{r.domain}</span>
                  <span className="text-xs text-[var(--color-ink-4)]">by {r.username}</span>
                  <span className="text-xs text-[var(--color-ink-4)]">{new Date(r.created_at).toLocaleString()}</span>
                  <div className="ms-auto flex gap-1.5">
                    <button onClick={() => handleApprove(r.id)} className="btn btn-primary btn-sm inline-flex items-center gap-1"><FiCheck size={12} /> {t('domains.approve')}</button>
                    <button onClick={() => handleReject(r.id)} className="btn btn-ghost btn-sm inline-flex items-center gap-1"><FiX size={12} /> {t('domains.reject')}</button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
