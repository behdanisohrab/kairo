import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { useAuth } from '../../App'
import { useToast } from '../../components/ui/Toast'
import { ConfirmModal } from '../../components/ui/Modal'
import MyIPsCard from '../../components/MyIPsCard'
import { useI18n } from '../../lib/i18n'
import { FiKey, FiCopy, FiCheck, FiEye, FiEyeOff, FiUser, FiShield, FiGlobe, FiSearch, FiSend } from 'react-icons/fi'

export default function Dashboard() {
  const [loading, setLoading] = useState(true)
  const [copied, setCopied] = useState(false)
  const [showKey, setShowKey] = useState(false)
  const [regenOpen, setRegenOpen] = useState(false)
  const [regenerating, setRegenerating] = useState(false)
  const [domain, setDomain] = useState('')
  const [checking, setChecking] = useState(false)
  const [checkResult, setCheckResult] = useState<{ restricted: boolean; checked: string } | null>(null)
  const [ipCount, setIpCount] = useState<number | null>(null)
  const { user, setUser } = useAuth()
  const { success, error } = useToast()
  const { t } = useI18n()

  useEffect(() => {
    api.myIPs().then((r) => setIpCount(r.ok ? r.ips?.length ?? 0 : null)).finally(() => setLoading(false))
  }, [])

  const copy = async () => {
    if (!user?.api_key) return
    await navigator.clipboard.writeText(user.api_key)
    setCopied(true); success('API key copied'); setTimeout(() => setCopied(false), 1400)
  }
  const regen = async () => {
    setRegenerating(true)
    const r = await api.regenerateMyAPIKey()
    if (r.ok && r.api_key && user) { setUser({ ...user, api_key: r.api_key }); success('API key regenerated') }
    else error(r.error || 'Failed')
    setRegenerating(false); setRegenOpen(false)
  }

  const checkDomain = async () => {
    const d = domain.trim().toLowerCase()
    if (!d) return
    setChecking(true)
    const r = await api.checkDomain(d)
    if (r.ok) setCheckResult({ restricted: !!r.restricted, checked: d })
    else { error(r.error || 'Failed'); setCheckResult(null) }
    setChecking(false)
  }
  const requestDomain = async () => {
    if (!checkResult?.checked) return
    const r = await api.requestDomain(checkResult.checked)
    if (r.ok) success(t('guide.requestSent'))
    else error(r.error || 'Failed')
  }

  if (loading) return <div className="space-y-4"><div className="skeleton h-8 w-48" /><div className="grid gap-4 md:grid-cols-2"><div className="skeleton h-48 rounded-[18px]" /><div className="skeleton h-48 rounded-[18px]" /></div><div className="skeleton h-64 rounded-[18px]" /></div>
  const masked = user?.api_key ? `${user.api_key.slice(0, 8)}${'*'.repeat(16)}${user.api_key.slice(-4)}` : '-'

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-[22px] font-semibold tracking-[-0.02em]">{t('dashboard.title')}</h1>
        <p className="mt-1 text-sm text-[var(--color-ink-3)]">{t('dashboard.subtitle')}</p>
      </div>

      <div className="grid gap-4 lg:grid-cols-[1.6fr_0.9fr]">
        <div className="card p-5">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiKey size={14} /> {t('dashboard.apiKey')}</h3>
            <span className="inline-flex items-center gap-1 rounded-md border bg-[var(--color-raised)] px-2.5 py-1 text-[11px] font-medium text-[var(--color-ink-3)]"><span className="h-1.5 w-1.5 rounded-sm bg-emerald-500" /> {t('dashboard.active')}</span>
          </div>
          <div className="mt-3 flex items-center gap-2 rounded-2xl border bg-[var(--color-raised)] p-2">
            <code className="mono flex-1 break-all px-2 text-xs leading-5 text-[var(--color-ink-2)]">{showKey ? user?.api_key : masked}</code>
            <button onClick={() => setShowKey((v) => !v)} className="btn btn-ghost btn-sm shrink-0 inline-flex items-center gap-1">{showKey ? <FiEyeOff size={12} /> : <FiEye size={12} />} {showKey ? t('dashboard.hide') : t('dashboard.reveal')}</button>
            <button onClick={copy} className="btn btn-outline btn-sm shrink-0 inline-flex items-center gap-1">{copied ? <FiCheck size={12} /> : <FiCopy size={12} />} {copied ? t('common.copied') : t('common.copy')}</button>
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <button onClick={() => setRegenOpen(true)} className="btn btn-ghost btn-sm inline-flex items-center gap-1"><FiKey size={12} /> {t('dashboard.regen')}</button>
            <span className="text-xs text-[var(--color-ink-4)]">{t('dashboard.oldKey')}</span>
          </div>
          <div className="mt-4 rounded-xl border bg-[var(--color-surface)] p-3 text-xs leading-5 text-[var(--color-ink-3)] flex gap-1.5"><FiShield size={14} className="mt-0.5 shrink-0" /> <span>{t('dashboard.useKey')} <Link to="/guide" className="font-medium text-[var(--color-brand)] no-underline hover:underline">{t('nav.guide')}</Link></span></div>
        </div>

        <div className="card p-5">
          <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiUser size={14} /> {t('dashboard.account')}</h3>
          <div className="mt-4 space-y-3">
            <div className="flex items-center justify-between gap-3"><span className="text-sm text-[var(--color-ink-3)]">{t('dashboard.username')}</span><span className="text-sm font-medium">{user?.username}</span></div>
            <div className="flex items-center justify-between gap-3"><span className="text-sm text-[var(--color-ink-3)]">{t('dashboard.role')}</span><span className="badge" style={{ background: user?.role === 'admin' ? 'var(--color-ink)' : 'var(--color-brand-soft)', color: user?.role === 'admin' ? 'var(--color-bg)' : 'var(--color-brand)', borderColor: user?.role === 'admin' ? 'var(--color-ink)' : '#dbe0ff' }}>{user?.role}</span></div>
            <div className="flex items-center justify-between gap-3"><span className="text-sm text-[var(--color-ink-3)]">{t('dashboard.rateLimit')}</span><span className="mono text-sm font-medium">{user?.rate_limit ?? 100}/min</span></div>
            <div className="h-px bg-[var(--color-border)]" />
            <div className="grid grid-cols-1 gap-3 text-center">
              <div className="rounded-2xl border bg-[var(--color-raised)] p-3"><div className="text-[20px] font-semibold leading-none">{ipCount ?? '–'}</div><div className="mt-1 text-[11px] font-medium uppercase tracking-wide text-[var(--color-ink-4)]">{t('ips.title')}</div></div>
            </div>
          </div>
        </div>
      </div>

      <MyIPsCard />

      <div className="card p-5">
        <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiGlobe size={14} /> {t('guide.domainCheck')}</h3>
        <p className="mt-1 text-xs text-[var(--color-ink-3)]">{t('guide.domainCheckDesc')}</p>
        <div className="mt-3 flex gap-2">
          <div className="field-wrap flex-1">
            <span className="field-icon"><FiSearch size={14} /></span>
            <input value={domain} onChange={(e) => setDomain(e.target.value)} placeholder={t('guide.enterDomain')} className="input input-with-icon" onKeyDown={(e) => e.key === 'Enter' && checkDomain()} />
          </div>
          <button onClick={checkDomain} disabled={checking || !domain.trim()} className="btn btn-primary inline-flex items-center gap-1"><FiSearch size={12} /> {checking ? t('common.loading') : t('guide.check')}</button>
        </div>
        {checkResult && (
          <div className="mt-3 flex flex-col sm:flex-row sm:items-center justify-between gap-2 rounded-xl border bg-[var(--color-raised)] px-3 py-2.5" style={{ borderColor: 'var(--color-border)' }}>
            <span className="text-sm flex items-center gap-1.5 break-all"><span className={`grid h-6 w-6 place-items-center rounded-md border ${checkResult.restricted ? 'bg-[var(--color-emerald-soft)] text-emerald-600 border-emerald-200' : 'bg-[var(--color-amber-soft)] text-amber-600 border-amber-200'}`}>{checkResult.restricted ? <FiShield size={12} /> : <FiGlobe size={12} />}</span> {checkResult.checked} <span className="text-xs text-[var(--color-ink-3)]">{checkResult.restricted ? t('guide.isRestricted') : t('guide.notRestricted')}</span></span>
            {!checkResult.restricted && <button onClick={requestDomain} className="btn btn-primary btn-sm shrink-0 inline-flex items-center gap-1"><FiSend size={12} /> {t('guide.request')}</button>}
          </div>
        )}
      </div>

      <ConfirmModal open={regenOpen} onClose={() => setRegenOpen(false)} onConfirm={regen} title={t('dashboard.regenConfirmTitle')} description={t('dashboard.regenConfirmDesc')} confirmText={regenerating ? t('common.loading') : t('dashboard.regenYes')} variant="primary" loading={regenerating} />
    </div>
  )
}
