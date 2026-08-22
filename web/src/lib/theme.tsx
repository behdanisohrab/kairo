import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

type Theme = 'light' | 'dark' | 'system'

interface ThemeCtx {
  theme: Theme
  resolved: 'light' | 'dark'
  setTheme: (t: Theme) => void
  toggle: () => void
}

const Ctx = createContext<ThemeCtx | null>(null)

function getSystem(): 'light' | 'dark' {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() => {
    const saved = localStorage.getItem('kairo-theme') as Theme | null
    return saved || 'system'
  })
  const [resolved, setResolved] = useState<'light' | 'dark'>(() => {
    if (theme === 'system') return getSystem()
    return theme as 'light' | 'dark'
  })

  useEffect(() => {
    const actual = theme === 'system' ? getSystem() : (theme as 'light' | 'dark')
    setResolved(actual)
    document.documentElement.setAttribute('data-theme', actual)
    document.documentElement.style.colorScheme = actual
    localStorage.setItem('kairo-theme', theme)

    if (theme === 'system') {
      const m = window.matchMedia('(prefers-color-scheme: dark)')
      const handler = () => {
        const next = m.matches ? 'dark' : 'light'
        setResolved(next)
        document.documentElement.setAttribute('data-theme', next)
      }
      m.addEventListener('change', handler)
      return () => m.removeEventListener('change', handler)
    }
  }, [theme])

  const toggle = () => setTheme((t) => (t === 'dark' ? 'light' : t === 'light' ? 'dark' : getSystem() === 'dark' ? 'light' : 'dark'))

  return <Ctx.Provider value={{ theme, resolved, setTheme, toggle }}>{children}</Ctx.Provider>
}

export function useTheme() {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useTheme must be inside ThemeProvider')
  return ctx
}
