import { useEffect, useState } from 'react'
import { api } from '../api'
import type { UserIP } from '../api'
import { useAuth } from '../App'
import { useI18n } from '../lib/i18n'
import { useToast } from '../components/ui/Toast'
import { Modal } from '../components/ui/Modal'
import { EmptyState } from '../components/ui/EmptyState'
import { FiTrash2, FiPlus, FiRefreshCw, FiShield, FiUsers, FiMapPin } from 'react-icons/fi'

export default function Ips() {
  const { user } = useAuth()
  const { t } = useI18n()
  const toast = useToast()
  const isAdmin = user?.role === 'admin'

  const [myIps, setMyIps] = useState<UserIP[]>([])
  const [limit, setLimit] = useState<number>(3)
  const [loading, setLoading] = useState(true)
  const [ipInput, setIpInput] = useState('')
  const [adding, setAdding] = useState(false)
  const [currentIp, setCurrentIp] = useState<string | null>(null)

  // admin: all users aggregated
  const [all, setAll] = useState<{ username: string; ips: UserIP[]; limit: number; id: number }[]>([])
  const [adminLoading, setAdminLoading] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<{ ip: string; userId?: number } | null>(null)

  const fetchMy = async () => {
    setLoading(true)
    const r = await api.myIPs()
    if (r.ok) {
      setMyIps(r.ips ?? [])
      setLimit(r.limit ?? 3)
    } else toast.error(r.error ?? 'failed')
    setLoading(false)
  }

  const fetchAll = async () => {
    const isAdminNow = user?.role === 'admin'
    if (!isAdminNow) return
    setAdminLoading(true)
    const u = await api.listUsers()
    if (u.ok && u.users) {
      const results = await Promise.all(
        u.users.map(async (usr) => {
          const r = await api.userIPs(usr.id)
          if (!r.ok) return { username: usr.username, id: usr.id, ips: [] as UserIP[], limit: usr.ip_limit ?? 3, error: r.error }
          return { username: usr.username, id: usr.id, ips: r.ips ?? [], limit: r.limit ?? usr.ip_limit ?? 3 }
        }),
      )
      setAll(results)
      if (results.some((x: unknown) => (x as { error?: string }).error)) {
        const firstErr = (results.find((x: unknown) => (x as { error?: string }).error) as { error?: string } | undefined)?.error
        if (firstErr) toast.error(firstErr)
      }
    } else if (!u.ok) {
      toast.error(u.error ?? 'failed to load users')
    }
    setAdminLoading(false)
  }

  useEffect(() => {
    fetchMy()
    api.myIP().then((r) => setCurrentIp(r.ip))
  }, [])

  useEffect(() => {
    if (user?.role === 'admin') fetchAll()
  }, [user?.role])

  const handleAdd = async (targetIp?: string, forUserId?: number) => {
    const ip = (targetIp ?? ipInput).trim()
    if (!ip) return
    const ipRegex = /^(\d{1,3}\.){3}\d{1,3}$/
    if (!ipRegex.test(ip)) { toast.error('invalid IP'); return }
    setAdding(true)
    let res
    if (forUserId && isAdmin) res = await api.addUserIP(forUserId, ip)
    else res = await api.addMyIP(ip)
    if (res.ok) {
      toast.success('IP added')
      setIpInput('')
      fetchMy()
      if (isAdmin) fetchAll()
    } else toast.error(res.error ?? 'failed')
    setAdding(false)
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    const { ip, userId } = deleteTarget
    let res
    if (userId && isAdmin) res = await api.removeUserIP(userId, ip)
    else res = await api.removeMyIP(ip)
    if (res.ok) {
      toast.success('IP removed')
      fetchMy()
      if (isAdmin) fetchAll()
    } else toast.error(res.error ?? 'failed')
    setDeleteTarget(null)
  }

  const unlimited = limit === 0
  const count = myIps.length
  const atLimit = !unlimited && count >= limit

  return (
    <div className="space-y-6">
      {/* header */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-[22px] font-semibold tracking-tight flex items-center gap-2">
            <FiShield size={18} className="text-[var(--color-ink-3)]" /> {isAdmin ? 'IP Management' : 'My IPs'}
          </h1>
          <p className="mt-1 text-sm text-[var(--color-ink-3)]">
            {isAdmin ? 'Per-user separate storage. Each user has own allowlist limited by ip_limit' : `Your allowlisted IPs — ${unlimited ? 'unlimited' : `${count}/${limit}`} (separate per user)`}
          </p>
        </div>
        <button onClick={() => { fetchMy(); fetchAll() }} className="btn btn-ghost text-xs">
          <FiRefreshCw size={12} /> {t('common.refresh')}
        </button>
      </div>

      {/* my IPs card */}
      <section className="card p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-sm font-semibold flex items-center gap-2">
            <FiMapPin size={14} /> {t('common.status')} — My IPs
            <span className={`badge ${atLimit ? 'bg-amber-100 text-amber-700 border-amber-200' : 'badge-brand'}`}>
              {unlimited ? 'unlimited' : `${count} / ${limit}`}
            </span>
          </h2>
          {currentIp && <span className="mono text-[11px] px-2 py-1 rounded bg-[var(--color-bg)] border">current {currentIp}</span>}
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          <div className="field-wrap flex-1 min-w-[180px] max-w-[260px]">
            <input
              value={ipInput}
              onChange={(e) => setIpInput(e.target.value)}
              placeholder="e.g. 1.2.3.4"
              className="input input-with-icon mono text-sm"
              onKeyDown={(e) => e.key === 'Enter' && handleAdd()}
            />
            <span className="field-icon"><FiMapPin size={13} /></span>
          </div>
          <button disabled={adding || atLimit} onClick={() => handleAdd()} className="btn btn-primary text-xs disabled:opacity-50">
            <FiPlus size={12} /> {adding ? t('common.refreshing') : 'Add IP'}
          </button>
          {currentIp && (
            <button disabled={atLimit} onClick={() => handleAdd(currentIp)} className="btn btn-ghost text-xs disabled:opacity-50">
              Use my current IP
            </button>
          )}
        </div>
        {atLimit && <p className="help text-amber-600 mt-2">IP limit reached ({limit}). Remove one or ask admin to increase.</p>}

        <div className="mt-4">
          {loading ? (
            <div className="skeleton h-16 w-full rounded-xl" />
          ) : myIps.length === 0 ? (
            <EmptyState title="No IPs yet" description="Add your public IP to use SNI/DNS routing" />
          ) : (
            <ul className="divide-y divide-[var(--color-border)] rounded-xl border overflow-hidden">
              {myIps.map((row) => (
                <li key={row.id} className="flex items-center justify-between gap-3 px-3 py-2.5 bg-[var(--color-surface)]">
                  <div className="mono text-sm">{row.ip}</div>
                  <div className="flex items-center gap-2">
                    <span className="text-[11px] text-[var(--color-ink-3)] mono">{new Date(row.created_at).toLocaleDateString()}</span>
                    <button onClick={() => setDeleteTarget({ ip: row.ip })} className="btn btn-ghost h-7 px-2 text-[11px] text-red-600 hover:bg-red-50">
                      <FiTrash2 size={12} /> Delete
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>

      {/* admin: all users */}
      {isAdmin && (
        <section className="card p-4 sm:p-5">
          <h2 className="text-sm font-semibold flex items-center gap-2">
            <FiUsers size={14} /> All users IPs (admin can delete any)
          </h2>
          <p className="text-xs text-[var(--color-ink-3)] mt-1">Separate storage per user. Deleting here removes only that user's copy; global allowlist stays if another user still has the IP.</p>
          {adminLoading ? (
            <div className="mt-4 skeleton h-24 w-full rounded-xl" />
          ) : all.length === 0 ? (
            <EmptyState title="No users" description="" />
          ) : (
            <div className="mt-4 space-y-4">
              {all.map((u) => (
                <div key={u.id} className="rounded-xl border overflow-hidden">
                  <div className="flex flex-wrap items-center justify-between gap-2 px-3 py-2 bg-[var(--color-bg)]">
                    <div className="text-sm font-medium">{u.username} <span className="mono text-xs text-[var(--color-ink-3)]">{u.ips.length}/{u.limit === 0 ? '∞' : u.limit}</span></div>
                    <div className="flex gap-1">
                      <span className="badge text-[11px]">{u.ips.length} IPs</span>
                      {u.limit !== 0 && u.ips.length >= u.limit && <span className="badge bg-amber-100 text-amber-700">limit</span>}
                    </div>
                  </div>
                  {u.ips.length === 0 ? (
                    <div className="px-3 py-3 text-xs text-[var(--color-ink-3)]">No IPs</div>
                  ) : (
                    <ul className="divide-y divide-[var(--color-border)]">
                      {u.ips.map((ip) => (
                        <li key={ip.id} className="flex items-center justify-between gap-3 px-3 py-2">
                          <span className="mono text-sm">{ip.ip}</span>
                          <button onClick={() => setDeleteTarget({ ip: ip.ip, userId: u.id })} className="btn btn-ghost h-7 px-2 text-[11px] text-red-600 hover:bg-red-50">
                            <FiTrash2 size={12} /> Delete
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      <Modal open={!!deleteTarget} onClose={() => setDeleteTarget(null)} title="Delete IP?">
        <p className="text-sm text-[var(--color-ink-2)]">Remove <span className="mono font-medium">{deleteTarget?.ip}</span> {deleteTarget?.userId ? 'for that user' : 'from your list'}?</p>
        <div className="mt-4 flex justify-end gap-2">
          <button onClick={() => setDeleteTarget(null)} className="btn btn-ghost text-xs">Cancel</button>
          <button onClick={handleDelete} className="btn bg-red-600 text-white hover:bg-red-700 text-xs">Delete</button>
        </div>
      </Modal>
    </div>
  )
}
