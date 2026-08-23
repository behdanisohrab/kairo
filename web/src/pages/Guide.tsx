import { useState, useEffect, useMemo } from 'react'
import { useAuth } from '../App'
import { useI18n } from '../lib/i18n'
import { api } from '../api'
import { FiCopy, FiCheck, FiMonitor, FiSmartphone, FiTablet, FiChrome, FiGlobe, FiCommand, FiServer, FiShield } from 'react-icons/fi'
import { FaApple, FaWindows, FaLinux, FaAndroid } from 'react-icons/fa'
import { SiFirefox } from 'react-icons/si'

function CopyBtn({ text }: { text: string }) {
  const [ok, setOk] = useState(false)
  const { t } = useI18n()
  return (
    <button
      onClick={async () => { await navigator.clipboard.writeText(text); setOk(true); setTimeout(() => setOk(false), 1200) }}
      className="btn btn-outline btn-sm shrink-0 inline-flex items-center gap-1"
    >
      {ok ? <FiCheck size={12} /> : <FiCopy size={12} />} {ok ? t('common.copied') : t('common.copy')}
    </button>
  )
}

function Code({ title, children }: { title?: string; children: string }) {
  return (
    <div className="overflow-hidden rounded-xl border bg-[var(--color-surface)]">
      {title && (
        <div className="flex items-center justify-between border-b bg-[var(--color-raised)] px-2.5 sm:px-3.5 py-2">
          <span className="mono text-[10px] sm:text-[11px] font-semibold tracking-wide text-[var(--color-ink-4)] uppercase">{title}</span>
          <CopyBtn text={children} />
        </div>
      )}
      <pre className="m-0 overflow-x-auto bg-[var(--color-surface)] p-2.5 sm:p-3.5 font-mono text-[11px] sm:text-xs leading-5 sm:leading-6 text-[var(--color-ink-2)]">
        <code className="break-all sm:break-normal">{children}</code>
      </pre>
    </div>
  )
}

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-3">
      <span className="grid h-7 w-7 shrink-0 place-items-center rounded-md bg-[var(--color-brand-soft)] text-xs font-bold text-[var(--color-brand)] border" style={{ borderColor: 'var(--color-border)' }}>{n}</span>
      <div className="min-w-0 flex-1 pb-5">
        <h4 className="text-sm font-semibold tracking-tight">{title}</h4>
        <div className="mt-1 text-sm leading-6 text-[var(--color-ink-2)]">{children}</div>
      </div>
    </div>
  )
}

export default function Guide() {
  const { user } = useAuth()
  const { t, lang } = useI18n()
  const key = user?.api_key || 'YOUR_API_KEY'
  const originDefault = typeof window !== 'undefined' ? window.location.origin : 'https://dns.example.com'
  const hostDefault = typeof window !== 'undefined' ? window.location.hostname : 'dns.example.com'
  const [cfgDohUrl, setCfgDohUrl] = useState<string | null>(null)
  const [vpsIp, setVpsIp] = useState<string | null>(null)
  useEffect(() => { api.publicConfig().then(r => { if (r.ok && r.doh_url) setCfgDohUrl(r.doh_url); if (r.ok && r.vps_ip) setVpsIp(r.vps_ip) }).catch(() => {}) }, [])
  const origin = cfgDohUrl ? cfgDohUrl.replace(/\/dns-query\/?$/, '') : originDefault
  const dohUrl = cfgDohUrl || `${originDefault}/dns-query`
  const host = (() => { try { return new URL(dohUrl).hostname } catch { return hostDefault } })()
  const displayVpsIp = vpsIp || 'YOUR_VPS_IP'
  const [platform, setPlatform] = useState<'windows' | 'mac' | 'linux' | 'android' | 'ios' | 'firefox' | 'chrome'>('windows')
  const [search, setSearch] = useState('')

  const platforms = useMemo(() => [
    { id: 'windows' as const, label: 'Windows', Icon: FaWindows },
    { id: 'mac' as const, label: 'macOS', Icon: FaApple },
    { id: 'linux' as const, label: 'Linux', Icon: FaLinux },
    { id: 'android' as const, label: 'Android', Icon: FaAndroid },
    { id: 'ios' as const, label: 'iOS', Icon: FiSmartphone },
    { id: 'firefox' as const, label: 'Firefox', Icon: SiFirefox },
    { id: 'chrome' as const, label: 'Chrome', Icon: FiChrome },
  ], [])
  const filteredPlatforms = platforms.filter((p) => p.label.toLowerCase().includes(search.toLowerCase()))

  const steps: Record<string, { en: { title: string; desc: React.ReactNode }[]; fa: { title: string; desc: React.ReactNode }[] }> = {
    windows: {
      en: [
        { title: 'Open Settings', desc: <p>Settings → Network & Internet → Ethernet (or Wi-Fi).</p> },
        { title: 'Change DNS', desc: <p>Click Edit next to DNS assignment, select Manual.</p> },
        { title: 'Enter DoH server', desc: <><p>Enable IPv4, enter:</p><div className="mt-2"><Code title="DNS">{host}</Code></div><p className="mt-2">Set encryption to Encrypted only (DoH).</p></> },
        { title: 'Verify', desc: <Code title="PowerShell">{`Resolve-DnsName -Name example.com -Server ${host}`}</Code> },
      ],
      fa: [
        { title: 'تنظیمات را باز کنید', desc: <p>تنظیمات → شبکه و اینترنت → اترنت (یا Wi-Fi).</p> },
        { title: 'DNS را تغییر دهید', desc: <p>روی ویرایش کلیک کنید و دستی را انتخاب کنید.</p> },
        { title: 'سرور DoH را وارد کنید', desc: <><p>IPv4 را فعال کنید و وارد کنید:</p><div className="mt-2"><Code title="DNS">{host}</Code></div><p className="mt-2">رمزگذاری را روی فقط رمز شده بگذارید.</p></> },
        { title: 'بررسی', desc: <Code title="PowerShell">{`Resolve-DnsName -Name example.com -Server ${host}`}</Code> },
      ],
    },
    mac: {
      en: [
        { title: 'Open Network Settings', desc: <p>System Settings → Network → WiFi → Details → DNS.</p> },
        { title: 'Add DNS server', desc: <><p>Remove old entries, click + and add:</p><div className="mt-2"><Code title="DNS">{host}</Code></div></> },
        { title: 'Encrypted DNS', desc: <p>For DoH use profile or dnscrypt-proxy. macOS supports DoH via profile.</p> },
        { title: 'Verify', desc: <Code title="Terminal">{`dig @${host} example.com`}</Code> },
      ],
      fa: [
        { title: 'تنظیمات شبکه', desc: <p>تنظیمات سیستم → شبکه → WiFi → جزئیات → DNS.</p> },
        { title: 'سرور DNS', desc: <><p>موارد قدیمی را حذف، + بزنید و اضافه کنید:</p><div className="mt-2"><Code title="DNS">{host}</Code></div></> },
        { title: 'DNS رمز شده', desc: <p>برای DoH از پروفایل یا dnscrypt-proxy استفاده کنید.</p> },
        { title: 'بررسی', desc: <Code title="Terminal">{`dig @${host} example.com`}</Code> },
      ],
    },
    linux: {
      en: [
        { title: 'systemd-resolved', desc: <><p>Edit <code className="inline">/etc/systemd/resolved.conf</code>:</p><div className="mt-2"><Code title="resolved.conf">{`[Resolve]\nDNS=${host}\nDNSOverHTTPS=yes`}</Code></div><p className="mt-2">Restart: <code className="inline">sudo systemctl restart systemd-resolved</code></p></> },
        { title: 'NetworkManager', desc: <Code title="Terminal">{`nmcli con mod "Connection" ipv4.dns "${host}"\nnmcli con mod "Connection" ipv4.ignore-auto-dns yes\nnmcli con up "Connection"`}</Code> },
        { title: 'Verify', desc: <Code title="Terminal">{`resolvectl status\ndig @${host} example.com`}</Code> },
      ],
      fa: [
        { title: 'systemd-resolved', desc: <><p><code className="inline">/etc/systemd/resolved.conf</code> را ویرایش کنید:</p><div className="mt-2"><Code title="resolved.conf">{`[Resolve]\nDNS=${host}\nDNSOverHTTPS=yes`}</Code></div><p className="mt-2">راه اندازی مجدد: <code className="inline">sudo systemctl restart systemd-resolved</code></p></> },
        { title: 'NetworkManager', desc: <Code title="Terminal">{`nmcli con mod "Connection" ipv4.dns "${host}"\nnmcli con mod "Connection" ipv4.ignore-auto-dns yes\nnmcli con up "Connection"`}</Code> },
        { title: 'بررسی', desc: <Code title="Terminal">{`resolvectl status\ndig @${host} example.com`}</Code> },
      ],
    },
    android: {
      en: [
        { title: 'Private DNS', desc: <p>Settings → Network & Internet → Private DNS.</p> },
        { title: 'Provider hostname', desc: <><p>Select Private DNS provider hostname and enter:</p><div className="mt-2"><Code title="Hostname">{host}</Code></div></> },
        { title: 'Verify', desc: <p>Open browser and check Dashboard. Device appears in seconds.</p> },
      ],
      fa: [
        { title: 'DNS خصوصی', desc: <p>تنظیمات → شبکه و اینترنت → DNS خصوصی.</p> },
        { title: 'نام میزبان', desc: <><p>گزینه نام میزبان را انتخاب و وارد کنید:</p><div className="mt-2"><Code title="Hostname">{host}</Code></div></> },
        { title: 'بررسی', desc: <p>مرورگر را باز کنید و داشبورد را چک کنید.</p> },
      ],
    },
    ios: {
      en: [
        { title: 'Install profile', desc: <p>Open Safari to DoH endpoint to install DNS profile.</p> },
        { title: 'Enable', desc: <p>Settings → VPN & Network → DNS → select configuration.</p> },
        { title: 'Manual', desc: <><p>WiFi → tap i → Configure DNS → Manual and add:</p><div className="mt-2"><Code title="DNS">{host}</Code></div></> },
      ],
      fa: [
        { title: 'نصب پروفایل', desc: <p>سافاری را به آدرس DoH ببرید تا پروفایل نصب شود.</p> },
        { title: 'فعال سازی', desc: <p>تنظیمات → VPN و شبکه → DNS → پیکربندی را انتخاب کنید.</p> },
        { title: 'دستی', desc: <><p>WiFi → i → پیکربندی DNS → دستی و اضافه کنید:</p><div className="mt-2"><Code title="DNS">{host}</Code></div></> },
      ],
    },
    firefox: {
      en: [
        { title: 'DNS settings', desc: <p>Settings → Privacy & Security → DNS over HTTPS.</p> },
        { title: 'Custom provider', desc: <p>Choose Increased or Max Protection → Custom → enter DoH URL.</p> },
        { title: 'DoH URL', desc: <Code title="DoH URL">{dohUrl}</Code> },
      ],
      fa: [
        { title: 'تنظیمات DNS', desc: <p>تنظیمات → حریم خصوصی → DNS over HTTPS.</p> },
        { title: 'ارائه دهنده سفارشی', desc: <p>افزایش یا حداکثر → سفارشی → آدرس را وارد کنید.</p> },
        { title: 'آدرس DoH', desc: <Code title="DoH URL">{dohUrl}</Code> },
      ],
    },
    chrome: {
      en: [
        { title: 'Security settings', desc: <p>Settings → Privacy and Security → Security.</p> },
        { title: 'Secure DNS', desc: <p>Toggle Use secure DNS on → With → Custom.</p> },
        { title: 'DoH URL', desc: <Code title="DoH URL">{dohUrl}</Code> },
      ],
      fa: [
        { title: 'تنظیمات امنیت', desc: <p>تنظیمات → حریم خصوصی → امنیت.</p> },
        { title: 'DNS امن', desc: <p>استفاده از DNS امن را روشن → سفارشی.</p> },
        { title: 'آدرس DoH', desc: <Code title="DoH URL">{dohUrl}</Code> },
      ],
    },
  }

  const currentSteps = steps[platform][lang as 'en' | 'fa'] || steps[platform].en

  return (
    <div className="mx-auto max-w-[760px] space-y-4 sm:space-y-6">
      <div>
        <h1 className="text-[20px] sm:text-[22px] font-semibold tracking-[-0.02em] flex items-center gap-2"><FiGlobe size={18} className="sm:w-5 sm:h-5" /> {t('guide.title')}</h1>
        <p className="mt-1 text-sm text-[var(--color-ink-3)]">{t('guide.desc')}</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <div className="card p-3 sm:p-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-4)] flex items-center gap-1"><FiCommand size={12} /> {t('guide.yourKey')}</div>
          <div className="mt-2 flex items-center gap-2 rounded-xl border bg-[var(--color-raised)] p-2">
            <code className="mono flex-1 break-all text-xs">{key}</code>
            <CopyBtn text={key} />
          </div>
          <p className="help">{t('guide.keyDesc')}</p>
        </div>
        <div className="card p-3 sm:p-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-4)] flex items-center gap-1"><FiServer size={12} /> {t('guide.dohEndpoint')}</div>
          <div className="mt-2"><Code title="DoH URL">{dohUrl}</Code></div>
          <div className="mt-3"><Code title={t('guide.plainDns')}>{displayVpsIp}</Code></div>
          <p className="help">{t('guide.dohDesc', { host, vps_ip: displayVpsIp })}</p>
        </div>
      </div>

      <div className="card p-4 sm:p-5">
        <h2 className="text-sm font-semibold flex items-center gap-1.5"><FiShield size={14} /> {t('guide.ipTitle')}</h2>
        <p className="mt-1 text-xs leading-5 text-[var(--color-ink-3)]">{t('guide.ipDesc')}</p>
        <div className="mt-4 space-y-1 rounded-xl border bg-[var(--color-raised)] p-3 sm:p-4">
          <Step n={1} title={t('guide.ipStep1')}><p>{t('guide.ipStep1Desc')}</p><div className="mt-2"><Code title="curl">curl -4 ifconfig.me</Code></div></Step>
          <Step n={2} title={t('guide.ipStep2')}><p>{t('guide.ipStep2Desc')}</p><div className="mt-2"><Code title="API">{`curl -X POST -H "Authorization: Bearer ${key}" "${origin}/api/allow?ip=YOUR_IP"`}</Code></div></Step>
          <Step n={3} title={t('guide.ipStep3')}><p>{t('guide.ipStep3Desc')}</p><div className="mt-2"><Code title="API">{`curl -H "Authorization: Bearer ${key}" "${origin}/api/allow"`}</Code></div><p className="mt-2 text-xs text-[var(--color-ink-3)]">{t('guide.ipNote')}</p></Step>
        </div>
      </div>

      <div className="card p-3 sm:p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-sm font-semibold flex items-center gap-1.5"><FiMonitor size={14} /> {t('guide.platformSetup')}</h2>
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('guide.filter')} className="input h-8 w-28 sm:w-32 py-1 text-xs" />
        </div>
        <div className="mt-3 flex gap-1.5 overflow-x-auto no-scrollbar pb-1">
          {filteredPlatforms.map((p) => (
            <button key={p.id} onClick={() => setPlatform(p.id)} className={`inline-flex shrink-0 items-center gap-1.5 rounded-md border px-2.5 sm:px-3.5 py-1.5 text-xs font-semibold transition ${platform === p.id ? 'bg-[var(--color-ink)] text-[var(--color-bg)] border-[var(--color-ink)]' : 'bg-[var(--color-surface)] text-[var(--color-ink-2)] border-[var(--color-border)]'}`}>
              <p.Icon size={12} /> {p.label}
            </button>
          ))}
        </div>
        <div className="mt-4 sm:mt-5 rounded-xl border bg-[var(--color-surface)] p-3 sm:p-5">
          {currentSteps.map((s, i) => <Step key={i} n={i + 1} title={s.title}><>{s.desc}</></Step>)}
        </div>
      </div>

      <div className="grid gap-3 sm:gap-4">
        <div className="card p-3 sm:p-5">
          <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiCommand size={14} /> {t('guide.curlTitle')}</h3>
          <p className="mt-1 text-xs text-[var(--color-ink-3)]">{t('guide.curlDesc')}</p>
          <div className="mt-3"><Code title="curl">{`curl -H "Content-Type: application/dns-message" \\\n     -H "Authorization: Bearer ${key}" \\\n     --data-binary @query.bin \\\n     ${dohUrl}`}</Code></div>
        </div>
        <div className="card p-3 sm:p-5">
          <h3 className="text-sm font-semibold flex items-center gap-1.5"><FiTablet size={14} /> {t('guide.trackingTitle')}</h3>
          <p className="mt-1 text-xs leading-5 text-[var(--color-ink-3)]">{t('guide.trackingDesc')}</p>
          <div className="mt-3"><Code title="API">{`curl -H "Authorization: Bearer ${key}" "${origin}/api/me/traffic?range=7d"`}</Code></div>
        </div>
        {user?.role === 'admin' && (
          <div className="card p-0 overflow-hidden">
            <div className="px-3 sm:px-5 py-3 border-b"><h3 className="text-sm font-semibold flex items-center gap-1.5"><FiServer size={14} /> {t('guide.adminRef')}</h3><p className="text-xs text-[var(--color-ink-3)]">{t('guide.adminDesc')}</p></div>
            <div className="table-wrap"><table className="table"><thead><tr><th>Method</th><th>Endpoint</th><th>Description</th></tr></thead><tbody>{[ ['GET','/api/users','List all users'], ['POST','/api/users','Create user'], ['DELETE','/api/users/:id','Delete user'], ['POST','/api/users/:id/api-key/regenerate','Regen key'], ['POST','/api/me/api-key/regenerate','Regen own'], ['GET','/api/allow','List allowlist union'], ['POST','/api/allow?ip=','Add IP to caller account'], ['DELETE','/api/allow?ip=','Remove IP from caller account'], ['GET','/api/restricted','Tunnelled domains'], ['POST','/api/restricted?domain=','Restrict domain'], ['GET','/api/direct','Direct-mode domains'], ['POST','/api/direct?domain=','Mark domain direct'], ['GET','/api/traffic?range=24h','Traffic analytics'] ].map(([m,ep,desc]) => <tr key={ep}><td><span className="mono text-[11px] font-bold" style={{ color: m==='GET'?'var(--color-emerald)':m==='POST'?'var(--color-brand)':'var(--color-rose)' }}>{m}</span></td><td className="mono text-xs">{ep}</td><td className="text-xs text-[var(--color-ink-3)]">{desc}</td></tr>)}</tbody></table></div>
          </div>
        )}
      </div>
    </div>
  )
}
