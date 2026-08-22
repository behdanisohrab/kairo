import { useState, useEffect, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import type { UserData } from '../../api'
import { useToast } from '../../components/ui/Toast'
import { ConfirmModal, Modal } from '../../components/ui/Modal'
import { useI18n } from '../../lib/i18n'
import { FiSearch, FiPlus, FiTrash2, FiKey, FiSmartphone, FiCopy, FiCheck, FiX, FiUserPlus, FiZap, FiEdit3 } from 'react-icons/fi'

function validateUsername(v: string) {
  if (v.length < 3) return 'At least 3 characters'
  if (!/^[a-zA-Z0-9._-]+$/.test(v)) return 'Only letters, numbers, dot, dash, underscore'
  return null
}
function validatePassword(v: string) {
  if (v.length < 6) return 'At least 6 characters'
  return null
}

export default function Users() {
  const [users, setUsers] = useState<UserData[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [formUser, setFormUser] = useState('')
  const [formPass, setFormPass] = useState('')
  const [formRate, setFormRate] = useState('100')
  const [unlimited, setUnlimited] = useState(false)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<UserData | null>(null)
  const [regenTarget, setRegenTarget] = useState<UserData | null>(null)
  const [regenLoading, setRegenLoading] = useState(false)
  const [lastKey, setLastKey] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [editRate, setEditRate] = useState<{ id: number; rate: string; unlimited: boolean } | null>(null)
  const { success, error } = useToast()
  const { t } = useI18n()

  const load = async () => {
    setLoading(true)
    const r = await api.listUsers()
    if (r.ok && r.users) setUsers(r.users)
    else if (!r.ok) error(r.error || 'Failed to load users')
    setLoading(false)
  }
  useEffect(() => { load() }, [])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return users
    return users.filter((u) => u.username.toLowerCase().includes(q) || u.role.toLowerCase().includes(q))
  }, [users, query])

  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    const uErr = validateUsername(formUser.trim())
    const pErr = validatePassword(formPass)
    if (uErr || pErr) { error(uErr || pErr || 'Fix form errors'); return }
    setCreating(true)
    const rate = unlimited ? 0 : (parseInt(formRate) || 100)
    const r = await api.createUser(formUser.trim(), formPass, rate)
    if (r.ok && r.user) {
      success(`User "${r.user.username}" created${rate===0?' (unlimited)':''}`)
      setLastKey(r.user.api_key)
      setFormUser(''); setFormPass(''); setShowCreate(false); setUnlimited(false); setFormRate('100'); load()
    } else error(r.error || 'Failed to create user')
    setCreating(false)
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    const r = await api.deleteUser(deleteTarget.id)
    if (r.ok) { success(`Deleted "${deleteTarget.username}"`); setDeleteTarget(null); load() }
    else error(r.error || 'Failed to delete')
  }
  const confirmRegen = async () => {
    if (!regenTarget) return
    setRegenLoading(true)
    const r = await api.regenerateAPIKey(regenTarget.id)
    if (r.ok && r.api_key) { setLastKey(r.api_key); success(`API key regenerated for "${regenTarget.username}"`); load() }
    else error(r.error || 'Failed to regenerate')
    setRegenLoading(false); setRegenTarget(null)
  }
  const copyKey = async (key: string) => {
    await navigator.clipboard.writeText(key)
    setCopied(true); setTimeout(() => setCopied(false), 1200)
  }
  const saveRate = async (id: number) => {
    if (!editRate) return
    const rate = editRate.unlimited ? 0 : (parseInt(editRate.rate) || 0)
    if (!editRate.unlimited && (rate < 1 || rate > 10000)) { error('Rate must be 1-10000 or unlimited'); return }
    const r = await api.updateUserRateLimit(id, rate)
    if (r.ok) { success(rate===0?'Unlimited enabled':`Rate set to ${rate}`); setEditRate(null); load() }
    else error(r.error || 'Failed')
  }

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="skeleton h-8 w-40" />
        <div className="skeleton h-14 rounded-[18px]" />
        <div className="skeleton h-64 rounded-[18px]" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-[22px] font-semibold tracking-[-0.02em]">{t('users.title')}</h1>
          <p className="mt-1 text-sm text-[var(--color-ink-3)]">{t('users.subtitle', { filtered: filtered.length, total: users.length })}</p>
        </div>
        <button onClick={() => setShowCreate((v) => !v)} className={`btn ${showCreate ? 'btn-outline' : 'btn-primary'} inline-flex items-center gap-1.5`}>
          {showCreate ? <FiX size={14} /> : <FiPlus size={14} />} {showCreate ? t('users.close') : t('users.newUser')}
        </button>
      </div>

      {lastKey && (
        <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-[var(--color-emerald-soft)] px-4 py-3 text-sm" style={{ borderColor: '#bbf7d0' }}>
          <span className="inline-flex items-center gap-1.5 font-semibold text-[var(--color-emerald)]"><FiKey size={14} /> {t('users.apiKey')}</span>
          <code className="inline flex-1 break-all text-xs">{lastKey}</code>
          <button onClick={() => copyKey(lastKey)} className="btn btn-outline btn-sm shrink-0 inline-flex items-center gap-1">
            {copied ? <FiCheck size={12} /> : <FiCopy size={12} />} {copied ? t('common.copied') : t('common.copy')}
          </button>
          <button onClick={() => setLastKey(null)} className="btn btn-ghost btn-sm">{t('users.dismiss')}</button>
        </div>
      )}

      {showCreate && (
        <form onSubmit={create} className="card p-5 animate-in" noValidate>
          <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiUserPlus size={14} /> {t('users.createTitle')}</h3>
          <p className="mt-1 text-xs text-[var(--color-ink-3)]">{t('users.createDesc')}</p>
          <div className="mt-4 grid gap-4 sm:grid-cols-3 items-start">
            <div className="flex flex-col">
              <label htmlFor="new-username" className="label">{t('users.username')}</label>
              <input id="new-username" value={formUser} onChange={(e) => setFormUser(e.target.value)} placeholder="e.g. johndoe" className="input" required />
              <p className="help min-h-[1.25rem] text-[var(--color-rose)]">{formUser ? validateUsername(formUser) || '\u00A0' : '\u00A0'}</p>
            </div>
            <div className="flex flex-col">
              <label htmlFor="new-password" className="label">{t('users.password')}</label>
              <input id="new-password" type="password" value={formPass} onChange={(e) => setFormPass(e.target.value)} placeholder="Min 6 characters" className="input" required />
              <p className="help min-h-[1.25rem] text-[var(--color-rose)]">{formPass ? validatePassword(formPass) || '\u00A0' : '\u00A0'}</p>
            </div>
            <div className="flex flex-col">
              <label className="label">{t('users.rateLimit')}</label>
              <div className="flex items-center gap-2">
                <input type="number" min={1} max={10000} value={formRate} onChange={(e) => setFormRate(e.target.value)} className="input flex-1" disabled={unlimited} placeholder="100" />
                <label className="inline-flex items-center gap-1 text-xs whitespace-nowrap shrink-0">
                  <input type="checkbox" checked={unlimited} onChange={(e) => setUnlimited(e.target.checked)} className="rounded" />
                  <FiZap size={12} /> {t('users.unlimited')}
                </label>
              </div>
              <p className="help min-h-[1.25rem]">{unlimited ? t('users.noLimit') : t('users.rateHelp')}</p>
            </div>
          </div>
          <div className="mt-4 flex justify-end">
            <button type="submit" disabled={creating} className="btn btn-primary px-6 py-2.5">{creating ? t('users.creating') : t('users.createBtn')}</button>
          </div>
        </form>
      )}

      <div className="card overflow-hidden p-0">
        <div className="flex flex-wrap items-center gap-3 border-b px-4 py-3">
          <div className="field-wrap flex-1 min-w-[220px]">
            <span className="field-icon"><FiSearch size={14} /></span>
            <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t('users.searchPh')} className="input input-with-icon" aria-label="Search users" />
          </div>
          <span className="text-xs text-[var(--color-ink-4)]">{t('users.results', { n: filtered.length })}</span>
        </div>

        {filtered.length === 0 ? (
          <div className="p-10 text-center">
            <div className="mx-auto grid h-10 w-10 place-items-center rounded-xl border bg-[var(--color-raised)]"><FiSearch size={18} /></div>
            <p className="mt-3 text-sm font-medium">{t('users.noMatchTitle')}</p>
            <p className="mt-1 text-xs text-[var(--color-ink-3)]">{t('users.noMatchDesc')}</p>
          </div>
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>User</th>
                  <th>Role</th>
                  <th>Limit</th>
                  <th className="hidden md:table-cell">Created</th>
                  <th className="w-[1%] whitespace-nowrap">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((u) => (
                  <tr key={u.id}>
                    <td>
                      <div className="flex items-center gap-2.5">
                        <span className="grid h-7 w-7 place-items-center rounded-full bg-[var(--color-raised)] border text-xs font-bold">{u.username[0]?.toUpperCase()}</span>
                        <span className="font-medium">{u.username}</span>
                        <span className="mono hidden sm:inline text-xs text-[var(--color-ink-4)]">#{u.id}</span>
                      </div>
                    </td>
                    <td>
                      <span className="badge" style={{ background: u.role === 'admin' ? 'var(--color-ink)' : 'var(--color-raised)', color: u.role === 'admin' ? 'var(--color-bg)' : 'var(--color-ink-2)', borderColor: u.role === 'admin' ? 'var(--color-ink)' : 'var(--color-border)' }}>
                        {u.role}
                      </span>
                    </td>
                    <td>
                      {editRate?.id === u.id ? (
                        <div className="flex items-center gap-1">
                          <input type="number" value={editRate.rate} onChange={(e) => setEditRate({ ...editRate, rate: e.target.value })} disabled={editRate.unlimited} className="input h-7 w-20 py-1 text-xs" />
                          <label className="inline-flex items-center gap-1 text-xs"><input type="checkbox" checked={editRate.unlimited} onChange={(e) => setEditRate({ ...editRate, unlimited: e.target.checked })} /> <FiZap size={10} /></label>
                          <button onClick={() => saveRate(u.id)} className="btn btn-primary btn-sm h-7 px-2"><FiCheck size={10} /></button>
                          <button onClick={() => setEditRate(null)} className="btn btn-ghost btn-sm h-7 px-2"><FiX size={10} /></button>
                        </div>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-xs">
                          {u.rate_limit === 0 ? <span className="badge" style={{ background: 'var(--color-violet-soft)', color: 'var(--color-violet)', borderColor: '#e9d5ff' }}><FiZap size={10} /> Unlimited</span> : `${u.rate_limit}/s`}
                          <button onClick={() => setEditRate({ id: u.id, rate: String(u.rate_limit), unlimited: u.rate_limit === 0 })} className="btn btn-ghost btn-sm h-6 w-6 p-0"><FiEdit3 size={10} /></button>
                        </span>
                      )}
                    </td>
                    <td className="hidden md:table-cell text-xs text-[var(--color-ink-3)]">{new Date(u.created_at).toLocaleDateString()}</td>
                    <td>
                      <div className="flex flex-wrap gap-1.5 justify-end">
                        <Link to={`/admin/users/${u.id}/devices`} className="btn btn-outline btn-sm no-underline inline-flex items-center gap-1"><FiSmartphone size={12} /> Devices</Link>
                        <button onClick={() => setRegenTarget(u)} className="btn btn-outline btn-sm inline-flex items-center gap-1"><FiKey size={12} /> Regen</button>
                        <button onClick={() => setDeleteTarget(u)} disabled={u.role === 'admin'} className="btn btn-danger btn-sm inline-flex items-center gap-1"><FiTrash2 size={12} /> {t('common.delete')}</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <ConfirmModal open={!!deleteTarget} onClose={() => setDeleteTarget(null)} onConfirm={confirmDelete} title={t('users.deleteTitle', { name: deleteTarget?.username || '' })} description={t('users.deleteDesc')} confirmText={t('users.deleteBtn')} variant="danger" />
      <Modal open={!!regenTarget} onClose={() => setRegenTarget(null)} title={t('users.regenTitle', { name: regenTarget?.username || '' })} description={t('users.regenDesc')}>
        <div className="mt-6 flex justify-end gap-2">
          <button onClick={() => setRegenTarget(null)} className="btn btn-ghost" disabled={regenLoading}>{t('common.cancel')}</button>
          <button onClick={confirmRegen} disabled={regenLoading} className="btn btn-primary inline-flex items-center gap-1"><FiKey size={12} /> {regenLoading ? t('common.loading') : t('users.regenBtn')}</button>
        </div>
      </Modal>
    </div>
  )
}
