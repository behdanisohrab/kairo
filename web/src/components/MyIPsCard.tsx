import { useState, useEffect } from 'react'
import { api } from '../api'
import type { UserIP } from '../api'
import { useToast } from './ui/Toast'
import { Modal } from './ui/Modal'
import { useI18n } from '../lib/i18n'
import { FiMapPin, FiPlus, FiTrash2 } from 'react-icons/fi'

// MyIPsCard is the self-service allowlist manager shown on the user
// dashboard: add/remove the IPs allowed to be split-routed for this account.
export default function MyIPsCard({ onChange }: { onChange?: () => void }) {
  const [ips, setIps] = useState<UserIP[]>([])
  const [limit, setLimit] = useState<number>(3)
  const [currentIp, setCurrentIp] = useState<string | null>(null)
  const [ipInput, setIpInput] = useState('')
  const [adding, setAdding] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const { success, error } = useToast()
  const { t } = useI18n()

  const load = async () => {
    const r = await api.myIPs()
    if (r.ok) {
      setIps(r.ips ?? [])
      setLimit(r.limit ?? 3)
      onChange?.()
    } else error(r.error ?? 'failed')
  }
  useEffect(() => {
    load().then(() => api.myIP().then((r) => setCurrentIp(r.ip)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const add = async (target?: string) => {
    const ip = (target ?? ipInput).trim()
    if (!ip) return
    setAdding(true)
    const r = await api.addMyIP(ip)
    if (r.ok) {
      success('IP added')
      setIpInput('')
      load()
    } else error(r.error ?? 'failed')
    setAdding(false)
  }

  const remove = async () => {
    if (!deleteTarget) return
    const r = await api.removeMyIP(deleteTarget)
    if (r.ok) {
      success('IP removed')
      load()
    } else error(r.error ?? 'failed')
    setDeleteTarget(null)
  }

  const unlimited = limit === 0
  const atLimit = !unlimited && ips.length >= limit

  return (
    <section className="card p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold flex items-center gap-1.5">
          <FiMapPin size={14} /> {t('ips.title')}
          <span className={`badge ${atLimit ? 'bg-amber-100 text-amber-700 border-amber-200' : 'badge-brand'}`}>
            {unlimited ? '∞' : `${ips.length} / ${limit}`}
          </span>
        </h2>
        {currentIp && <span className="mono text-[11px] px-2 py-1 rounded bg-[var(--color-bg)] border">current {currentIp}</span>}
      </div>

      <div className="mt-3 flex flex-wrap gap-2">
        <div className="field-wrap flex-1 min-w-[180px] max-w-[260px]">
          <input
            value={ipInput}
            onChange={(e) => setIpInput(e.target.value)}
            placeholder="1.2.3.4"
            className="input input-with-icon mono text-sm"
            onKeyDown={(e) => e.key === 'Enter' && add()}
          />
          <span className="field-icon"><FiMapPin size={13} /></span>
        </div>
        <button disabled={adding || atLimit} onClick={() => add()} className="btn btn-primary text-xs disabled:opacity-50">
          <FiPlus size={12} /> {adding ? t('common.loading') : t('ips.add')}
        </button>
        {currentIp && (
          <button disabled={atLimit} onClick={() => add(currentIp)} className="btn btn-ghost text-xs disabled:opacity-50">
            {t('ips.useCurrent')}
          </button>
        )}
      </div>
      {atLimit && <p className="help text-amber-600 mt-2">{t('ips.limitReached')}</p>}

      <div className="mt-4">
        {ips.length === 0 ? (
          <p className="p-4 text-center text-sm text-[var(--color-ink-3)]">{t('ips.empty')}</p>
        ) : (
          <ul className="divide-y divide-[var(--color-border)] rounded-xl border overflow-hidden">
            {ips.map((row) => (
              <li key={row.id} className="flex items-center justify-between gap-3 px-3 py-2.5 bg-[var(--color-surface)]">
                <div className="mono text-sm">{row.ip}</div>
                <div className="flex items-center gap-2">
                  <span className="text-[11px] text-[var(--color-ink-3)] mono">{new Date(row.created_at).toLocaleDateString()}</span>
                  <button onClick={() => setDeleteTarget(row.ip)} className="btn btn-ghost h-7 px-2 text-[11px] text-red-600 hover:bg-red-50">
                    <FiTrash2 size={12} /> {t('common.delete')}
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      <Modal open={!!deleteTarget} onClose={() => setDeleteTarget(null)} title={t('ips.deleteTitle')}>
        <p className="text-sm text-[var(--color-ink-2)]">{t('ips.deleteBody', { ip: deleteTarget ?? '' })}</p>
        <div className="mt-4 flex justify-end gap-2">
          <button onClick={() => setDeleteTarget(null)} className="btn btn-ghost text-xs">{t('common.cancel')}</button>
          <button onClick={remove} className="btn bg-red-600 text-white hover:bg-red-700 text-xs">{t('common.delete')}</button>
        </div>
      </Modal>
    </section>
  )
}
