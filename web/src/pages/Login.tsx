import { useState } from 'react'
import { useNavigate, useLocation, Link } from 'react-router-dom'
import { api } from '../api'
import { useAuth } from '../App'
import { useToast } from '../components/ui/Toast'
import { useI18n } from '../lib/i18n'
import { useTheme } from '../lib/theme'
import { FiEye, FiEyeOff, FiAlertCircle, FiGlobe, FiSun, FiMoon, FiShield, FiZap, FiLayers } from 'react-icons/fi'

export default function Login() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPass, setShowPass] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const { setUser, refresh } = useAuth()
  const navigate = useNavigate()
  const loc = useLocation() as unknown as { state?: { from?: { pathname: string } } }
  const { error: toastError } = useToast()
  const { t, lang, setLang } = useI18n()
  const { resolved, toggle } = useTheme()

  const from = loc.state?.from?.pathname || null

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!username.trim() || !password) {
      setError(t('login.fillBoth'))
      return
    }
    setError('')
    setLoading(true)
    try {
      const r = await api.login(username.trim(), password)
      if (r.ok && r.user) {
        const me = await api.me()
        if (me.ok && me.user) setUser(me.user)
        else setUser({ ...r.user, api_key: '', rate_limit: 100, created_at: new Date().toISOString() } as never)
        await refresh().catch(() => {})
        const target = from || (r.user.role === 'admin' ? '/admin' : '/dashboard')
        navigate(target, { replace: true })
      } else {
        const msg = r.error || t('login.invalid')
        setError(msg)
        toastError(msg)
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Something went wrong'
      setError(msg)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-[var(--color-bg)] flex">
      {/* Left brand panel - fixed dark for readability in both themes */}
      <div className="hidden lg:flex lg:w-[44%] relative overflow-hidden border-r bg-[#141210] text-white">
        <div className="absolute inset-0 opacity-[0.12]">
          <div className="absolute -top-24 -left-24 h-[520px] w-[520px] rounded-full bg-[var(--color-brand)] blur-[60px]" />
          <div className="absolute bottom-0 right-0 h-[420px] w-[420px] rounded-full bg-amber-400 blur-[60px]" />
        </div>
        <div
          className="absolute inset-0 opacity-[0.08]"
          style={{
            backgroundImage:
              'linear-gradient(rgba(255,255,255,0.12) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.12) 1px, transparent 1px)',
            backgroundSize: '36px 36px',
          }}
        />
        <div className="relative z-10 flex flex-1 flex-col p-8 xl:p-10">
          <Link to="/" className="inline-flex items-center gap-2.5 no-underline">
            <span className="grid h-9 w-9 place-items-center rounded-xl bg-white text-[#141210] font-mono text-xs font-bold">k.</span>
            <span className="mono text-sm font-semibold tracking-[-0.02em]">kairo</span>
            <span className="rounded-md border border-white/20 bg-white/10 px-2 py-0.5 text-[10px] tracking-wide">v0.2</span>
          </Link>

          <div className="mt-auto">
            <p className="inline-flex items-center gap-2 rounded-md border border-white/15 bg-white/10 px-3 py-1 text-xs">
              <span className="h-1.5 w-1.5 rounded-sm bg-emerald-400 animate-pulse" />
              {t('login.trusted')}
            </p>
            <h1 className="mt-4 text-[28px] font-semibold leading-[1.1] tracking-[-0.03em] text-white">
              {t('login.secureDns')}
              <br />
              {t('login.routed')}
            </h1>
            <p className="mt-3 max-w-[36ch] text-[13.5px] leading-6 text-white/70">{t('login.heroDesc')}</p>

            <div className="mt-8 grid grid-cols-3 gap-3 text-xs">
              <div className="rounded-2xl border border-white/10 bg-white/10 p-3 backdrop-blur">
                <FiShield size={16} className="text-white/90" />
                <div className="mt-1 text-white font-semibold">{t('login.tagDns')}</div>
                <div className="text-white/60">Plain + encrypted</div>
              </div>
              <div className="rounded-2xl border border-white/10 bg-white/10 p-3 backdrop-blur">
                <FiZap size={16} className="text-white/90" />
                <div className="mt-1 text-white font-semibold">{t('login.tagSni')}</div>
                <div className="text-white/60">:443 transparent</div>
              </div>
              <div className="rounded-2xl border border-white/10 bg-white/10 p-3 backdrop-blur">
                <FiLayers size={16} className="text-white/90" />
                <div className="mt-1 text-white font-semibold">{t('login.tagPolicy')}</div>
                <div className="text-white/60">Hot-reload files</div>
              </div>
            </div>

            <div className="mt-8 flex items-center gap-2 text-xs text-white/60">
              <span>{t('login.footer')}</span>
              <span className="h-1 w-1 rounded-full bg-white/30" />
              <span>BSD-3-Clause</span>
            </div>
          </div>
        </div>
      </div>

      {/* Right form */}
      <div className="flex flex-1 items-center justify-center p-4 sm:p-6 relative">
        {/* Top controls */}
        <div className="absolute top-4 end-4 flex items-center gap-1.5">
          <button onClick={() => setLang(lang === 'fa' ? 'en' : 'fa')} className="inline-flex items-center gap-1.5 rounded-md border bg-[var(--color-surface)] px-3 py-1.5 text-xs font-medium">
            <FiGlobe size={12} /> {lang === 'fa' ? 'EN' : 'FA'}
          </button>
          <button onClick={toggle} className="grid h-8 w-8 place-items-center rounded-lg border bg-[var(--color-surface)]">
            {resolved === 'dark' ? <FiSun size={14} /> : <FiMoon size={14} />}
          </button>
        </div>

        <div className="w-full max-w-[420px] animate-in">
          <div className="lg:hidden mb-6 flex items-center gap-2">
            <span className="grid h-8 w-8 place-items-center rounded-xl bg-[var(--color-ink)] text-[var(--color-bg)] font-mono text-xs font-bold">k.</span>
            <span className="mono text-sm font-bold tracking-tight">kairo</span>
            <span className="ms-auto text-xs text-[var(--color-ink-4)]">Split gateway</span>
          </div>

          <div className="card p-6 sm:p-7">
            <div className="mb-6">
              <h2 className="text-[22px] font-semibold tracking-[-0.02em]">{t('login.title')}</h2>
              <p className="mt-1 text-sm text-[var(--color-ink-3)]">{t('login.subtitle')}</p>
            </div>

            {error && (
              <div role="alert" aria-live="polite" className="mb-4 flex gap-2.5 rounded-2xl border bg-[var(--color-rose-soft)] px-3.5 py-3 text-sm leading-5 text-[var(--color-rose)]" style={{ borderColor: '#fecaca' }}>
                <span className="mt-0.5 grid h-5 w-5 shrink-0 place-items-center rounded-md bg-[var(--color-surface)] border">
                  <FiAlertCircle size={12} />
                </span>
                <span>{error}</span>
              </div>
            )}

            <form onSubmit={submit} noValidate className="space-y-4">
              <div>
                <label htmlFor="username" className="label">{t('login.username')}</label>
                <input id="username" type="text" value={username} onChange={(e) => setUsername(e.target.value)} required autoFocus autoComplete="username" aria-invalid={error ? true : undefined} placeholder={t('login.usernamePh')} className="input" disabled={loading} />
              </div>

              <div>
                <div className="flex items-center justify-between">
                  <label htmlFor="password" className="label mb-0">{t('login.password')}</label>
                  <button type="button" onClick={() => setShowPass((v) => !v)} className="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-brand)] hover:underline" tabIndex={-1}>
                    {showPass ? <FiEyeOff size={12} /> : <FiEye size={12} />} {showPass ? t('login.hide') : t('login.show')}
                  </button>
                </div>
                <div className="mt-1.5 relative">
                  <input id="password" type={showPass ? 'text' : 'password'} value={password} onChange={(e) => setPassword(e.target.value)} required autoComplete="current-password" placeholder="••••••••" className="input pr-10" disabled={loading} />
                </div>
                <p className="help">Use the credentials created by your administrator.</p>
              </div>

              <button type="submit" disabled={loading} className="btn btn-primary w-full py-3 text-sm shadow-[0_10px_20px_rgba(43,43,255,0.18)]">
                {loading ? <><span className="h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" /> {t('login.signingIn')}</> : t('login.signIn')}
              </button>

              <div className="flex items-center gap-3 py-1">
                <div className="h-px flex-1 bg-[var(--color-border)]" />
                <span className="text-xs text-[var(--color-ink-4)]">or</span>
                <div className="h-px flex-1 bg-[var(--color-border)]" />
              </div>

              <div className="rounded-2xl border bg-[var(--color-raised)] p-3 text-xs leading-5 text-[var(--color-ink-3)]">
                <span className="font-semibold text-[var(--color-ink-2)]">Demo?</span> {t('login.demo')}
              </div>
            </form>
          </div>

          <p className="mt-4 text-center text-xs text-[var(--color-ink-4)]">{t('login.protected')}</p>
        </div>
      </div>
    </div>
  )
}
