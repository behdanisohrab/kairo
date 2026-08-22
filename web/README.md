# Kairo Web — Production Frontend (Bun + Vite + React 19)

Production-ready SPA for Kairo’s split-routing gateway. Built with **Bun**, **Vite**, **React 19**, **React Router 7**, and **Tailwind CSS 4**.

## Stack

- **Runtime / Package manager:** Bun `>=1.1` (`oven/bun:1-alpine` in Docker)
- **Build:** Vite 8 + esbuild
- **UI:** Tailwind 4 (via `@tailwindcss/vite`), custom design tokens, accessible primitives
- **Routing:** `react-router-dom` with lazy code-splitting
- **State:** React Context for auth, local state + memo for lists
- **Quality:** TypeScript 6 strict, Oxlint, typecheck, gzip-aware chunks

## Design System

Warm stone/ink base (`#141210`) + electric brand `#2b2bff`, with emerald/amber/rose semantics. Tokens in `src/index.css` (`@theme`). Utilities: `.card`, `.btn-*`, `.input`, `.table`, `.badge`, `.skeleton`.

Key primitives in `src/components/ui/`:

- `Toast` – global toast provider (`useToast()`)
- `Modal` / `ConfirmModal` – accessible ESC-to-close modals
- `EmptyState`, `Skeleton` – loading & empty handling

## Scripts (use **bun**, not npm)

```bash
bun install        # install (frozen lockfile in Docker)
bun run dev        # vite dev @ http://localhost:5173 (proxies /api → :8443)
bun run build      # tsc -b + vite build → dist/ (chunked: vendor/deps + lazy pages)
bun run preview    # preview prod build
bun run lint       # oxlint
bun run typecheck  # tsc --noEmit
```

> Docker uses `oven/bun:1-alpine` and runs `bun install --frozen-lockfile && bun run build`. Host needs Bun 1.3+ locally.

## App Structure

```
src/
  api.ts               Robust fetch wrapper (timeout, abort, safe errors)
  App.tsx              AuthProvider, lazy routes, guards (admin/user), 404
  components/Layout.tsx Sticky header, user menu, responsive nav, breadcrumbs
  components/ui/*      Toast, Modal, Skeleton, EmptyState
  pages/
    Login.tsx          Split panel, validation, show/hide, redirects
    Guide.tsx          DoH endpoint + 7 platform tabs, copy, curl samples
    user/Dashboard.tsx API key (masked/reveal/copy/regen), device table
    admin/
      Overview.tsx     Stats, health strip, recent devices, auto-refresh
      Users.tsx        CRUD + search + regen + delete guard, key reveal
      AllDevices.tsx   Filter by q/type, sort, pagination (12/page)
      UserDevices.tsx  Per-user device list
```

## Auth & Functionality

- Session via `kairo_session` cookie (HttpOnly, Lax, Secure when TLS). Login sets session, `/api/auth/me` hydrates `UserData`.
- `api.ts` uses `safeRequest` so UI never crashes on network/timeout; surfaces `{ok:false, error}` for toasts.
- Guards: `Guard` checks `initialized` then role; `PublicOnly` redirects logged-in users; `*` → role-aware 404 inside Layout.
- All tables: loading skeletons, empty states, error toasts, copy/regenerate flows validated.
- Devices: JA3 fingerprint, type badge, IP + user-agent search, pagination, type filter.
- Users: validation (3-char username, 6-char password), rate_limit, delete blocked for admins, API key copy with toast.

## Production Checklist

- [x] Bun lockfile + `packageManager` field, `bunfig.toml`
- [x] Vite code-splitting (vendor + lazy per-route), gzip sizes in build log
- [x] CSS minify, no sourcemaps in prod, 0 FOUC flash
- [x] Secure cookie fix (`isSecureRequest` — dev http vs prod https)
- [x] Accessibility: labels, aria-live toasts, keyboard ESC, focus-visible
- [x] Responsive: mobile nav + bottom actions, table overflow, breakpoints
- [x] Error handling: timeout 15s, JSON parse guard, expired session → login
- [x] Optimizations: lazy routes + Suspense, memoized filters, 30s polling

## Running behind Kairo

Go server serves `web/dist` as SPA (fallback to `index.html`). Configure `web_dir` in `config.yaml` (Docker mounts `/app/web/dist`). Health: `GET /healthz`, status at `/`.

## Lint

Oxlint warnings for `setState in effect` are benign (hydration polling). Typecheck is clean.

