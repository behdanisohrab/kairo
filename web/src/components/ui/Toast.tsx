import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from 'react'

type ToastKind = 'success' | 'error' | 'info'
interface ToastItem {
  id: number
  kind: ToastKind
  message: string
}

interface ToastCtx {
  toast: (msg: string, kind?: ToastKind) => void
  success: (msg: string) => void
  error: (msg: string) => void
}

const Ctx = createContext<ToastCtx | null>(null)

let counter = 0

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])

  const toast = useCallback((message: string, kind: ToastKind = 'info') => {
    const id = ++counter
    setItems((s) => [...s, { id, kind, message }])
    setTimeout(() => setItems((s) => s.filter((x) => x.id !== id)), 3800)
  }, [])

  return (
    <Ctx.Provider
      value={{
        toast,
        success: (m) => toast(m, 'success'),
        error: (m) => toast(m, 'error'),
      }}
    >
      {children}
      <div
        aria-live="polite"
        aria-atomic="true"
        className="fixed bottom-4 right-4 z-[80] flex flex-col gap-2 max-w-[min(92vw,420px)] pointer-events-none"
      >
        {items.map((t) => (
          <div
            key={t.id}
            role="status"
            className="animate-scale pointer-events-auto flex items-start gap-3 rounded-xl border bg-[var(--color-surface)] px-4 py-3 shadow-medium"
            style={{
              borderColor:
                t.kind === 'success' ? 'var(--color-border)' : t.kind === 'error' ? 'var(--color-border)' : 'var(--color-border)',
            }}
          >
            <span
              className="mt-0.5 inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-xs font-bold border"
              style={{
                background:
                  t.kind === 'success'
                    ? 'var(--color-emerald-soft)'
                    : t.kind === 'error'
                      ? 'var(--color-rose-soft)'
                      : 'var(--color-raised)',
                color:
                  t.kind === 'success'
                    ? 'var(--color-emerald)'
                    : t.kind === 'error'
                      ? 'var(--color-rose)'
                      : 'var(--color-ink-2)',
                borderColor: 'var(--color-border)',
              }}
            >
              {t.kind === 'success' ? '✓' : t.kind === 'error' ? '!' : '•'}
            </span>
            <p className="text-sm leading-5 text-[var(--color-ink)] pr-2">{t.message}</p>
            <button
              onClick={() => setItems((s) => s.filter((x) => x.id !== t.id))}
              className="ml-auto shrink-0 grid h-6 w-6 place-items-center rounded-md text-[var(--color-ink-4)] hover:bg-[var(--color-raised)] hover:text-[var(--color-ink)]"
              aria-label="Dismiss"
            >
              ×
            </button>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  )
}

export function useToast() {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useToast must be inside ToastProvider')
  return ctx
}

export function useToastEffect(msg: string | null, kind: ToastKind = 'info') {
  const { toast } = useToast()
  useEffect(() => {
    if (msg) toast(msg, kind)
  }, [msg, kind, toast])
}
