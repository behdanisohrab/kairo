const API_BASE = '/api'

// ── Types ────────────────────────────────────────────────────────────
export interface UserData {
  id: number
  username: string
  api_key: string
  role: string
  rate_limit: number
  ip_limit: number
  created_at: string
  last_login?: string | null
}
export interface UserIP {
  id: number
  user_id: number
  ip: string
  created_at: string
}

export interface ConnectionLogData {
  id: number
  ip: string
  user_id?: number | null
  username?: string
  domain: string
  created_at: string
}

export interface TrafficBucket {
  bucket: string
  count: number
}

export interface NameCount {
  name: string
  count: number
}

export interface AdminTrafficResponse extends APIResponse {
  range_hours?: number
  total_users?: number
  connections?: number
  unique_ips?: number
  allowlisted?: number
  restricted?: number
  direct?: number
  uptime_seconds?: number
  version?: string
  buckets?: TrafficBucket[]
  top_domains?: NameCount[]
  top_users?: NameCount[]
  recent?: ConnectionLogData[]
}

export interface UserTrafficResponse extends APIResponse {
  user_id?: number
  range_hours?: number
  total_requests?: number
  unique_domains?: number
  buckets?: TrafficBucket[]
  recent?: ConnectionLogData[]
  rate_limit?: number
  unlimited?: boolean
}

export interface APIResponse {
  ok: boolean
  error?: string
  message?: string
}
export interface LoginResponse extends APIResponse {
  user?: { id: number; username: string; role: string }
}
export interface MeResponse extends APIResponse {
  user?: UserData
}
export interface UsersResponse extends APIResponse {
  users?: UserData[]
}
export interface CreateUserResponse extends APIResponse {
  user?: UserData
}
export interface RegenerateKeyResponse extends APIResponse {
  api_key?: string
}

// ── Request core ─────────────────────────────────────────────────────
class ApiError extends Error {
  status: number
  payload: unknown
  constructor(message: string, status: number, payload: unknown) {
    super(message)
    this.status = status
    this.payload = payload
  }
}

type RequestOpts = RequestInit & { timeoutMs?: number }

async function request<T>(path: string, options: RequestOpts = {}): Promise<T> {
  const { timeoutMs = 15000, ...fetchOpts } = options
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeoutMs)

  let res: Response
  try {
    res = await fetch(`${API_BASE}${path}`, {
      ...fetchOpts,
      credentials: 'include',
      signal: controller.signal,
      headers: {
        'Content-Type': 'application/json',
        ...(fetchOpts.headers as Record<string, string> | undefined),
      },
    })
  } catch (err: unknown) {
    clearTimeout(timer)
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw new ApiError('Request timed out please try again', 408, null)
    }
    throw new ApiError(
      err instanceof Error ? err.message : 'Network error check your connection',
      0,
      null,
    )
  }
  clearTimeout(timer)

  // 401 is handled by the caller via global handler; still parse JSON
  let data: unknown
  const text = await res.text()
  try {
    data = text ? JSON.parse(text) : {}
  } catch {
    // Non-JSON (e.g. HTML error) surface as error
    throw new ApiError(`Server error (${res.status})`, res.status, text)
  }

  if (!res.ok) {
    const msg =
      typeof data === 'object' && data !== null && 'error' in data
        ? String((data as Record<string, unknown>).error)
        : `Request failed (${res.status})`
    throw new ApiError(msg, res.status, data)
  }

  return data as T
}

// Safe wrapper that never throws returns {ok:false} instead, for UI convenience
async function safeRequest<T extends APIResponse>(
  path: string,
  opts?: RequestOpts,
): Promise<T> {
  try {
    return await request<T>(path, opts)
  } catch (e) {
    const err = e as ApiError
    return { ok: false, error: err.message } as T
  }
}

// ── Public API ───────────────────────────────────────────────────────
export const api = {
  // Auth
  login: (username: string, password: string) =>
    safeRequest<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username: username.trim(), password }),
    }),

  logout: () => safeRequest<APIResponse>('/auth/logout', { method: 'POST' }),

  me: () => safeRequest<MeResponse>('/auth/me'),

  // Users (admin)
  listUsers: () => safeRequest<UsersResponse>('/users'),

  createUser: (username: string, password: string, rate_limit?: number, ip_limit?: number) =>
    safeRequest<CreateUserResponse>('/users', {
      method: 'POST',
      body: JSON.stringify({ username: username.trim(), password, rate_limit, ip_limit }),
    }),

  deleteUser: (id: number) =>
    safeRequest<APIResponse>(`/users/${id}`, { method: 'DELETE' }),

  regenerateAPIKey: (id: number) =>
    safeRequest<RegenerateKeyResponse>(`/users/${id}/api-key/regenerate`, {
      method: 'POST',
    }),

  regenerateMyAPIKey: () =>
    safeRequest<RegenerateKeyResponse>('/me/api-key/regenerate', {
      method: 'POST',
    }),

  // Legacy / status (admin)
  allowList: () => safeRequest<{ ok: boolean; data?: string[]; error?: string }>('/allow'),
  addAllow: (ip: string) => safeRequest<APIResponse>(`/allow?ip=${encodeURIComponent(ip)}`, { method: 'POST' }),
  removeAllow: (ip: string) => safeRequest<APIResponse>(`/allow?ip=${encodeURIComponent(ip)}`, { method: 'DELETE' }),
  restrictedList: () =>
    safeRequest<{ ok: boolean; data?: string[]; error?: string }>('/restricted'),
  addRestricted: (domain: string) => safeRequest<APIResponse>(`/restricted?domain=${encodeURIComponent(domain)}`, { method: 'POST' }),
  removeRestricted: (domain: string) => safeRequest<APIResponse>(`/restricted?domain=${encodeURIComponent(domain)}`, { method: 'DELETE' }),

  directList: () =>
    safeRequest<{ ok: boolean; data?: string[]; error?: string }>('/direct'),
  addDirect: (domain: string) => safeRequest<APIResponse>(`/direct?domain=${encodeURIComponent(domain)}`, { method: 'POST' }),
  removeDirect: (domain: string) => safeRequest<APIResponse>(`/direct?domain=${encodeURIComponent(domain)}`, { method: 'DELETE' }),
  // Domain check / request (new)
  checkDomain: (domain: string) => safeRequest<{ ok: boolean; restricted?: boolean; error?: string }>(`/domain/check?domain=${encodeURIComponent(domain)}`),
  requestDomain: (domain: string) => safeRequest<APIResponse>('/domain/request', { method: 'POST', body: JSON.stringify({ domain }) }),
  listDomainRequests: () => safeRequest<{ ok: boolean; requests?: DomainRequest[]; error?: string }>('/domain/requests'),
  approveDomainRequest: (id: number) => safeRequest<APIResponse>(`/domain/requests/${id}/approve`, { method: 'POST' }),
  rejectDomainRequest: (id: number) => safeRequest<APIResponse>(`/domain/requests/${id}/reject`, { method: 'POST' }),
  publicConfig: () => safeRequest<{ ok: boolean; admin_url?: string; doh_url?: string; host?: string; vps_ip?: string; error?: string }>('/public-config'),
  traffic: (range: string = '24h') =>
    safeRequest<AdminTrafficResponse>(`/traffic?range=${encodeURIComponent(range)}`),
  myTraffic: (range: string = '24h') =>
    safeRequest<UserTrafficResponse>(`/me/traffic?range=${encodeURIComponent(range)}`),
  updateUserRateLimit: (id: number, rate_limit: number) => safeRequest<APIResponse>(`/users/${id}/rate-limit`, { method: 'POST', body: JSON.stringify({ rate_limit }) }),
  updateUserIpLimit: (id: number, ip_limit: number) => safeRequest<APIResponse>(`/users/${id}/ip-limit`, { method: 'POST', body: JSON.stringify({ ip_limit }) }),
  myIPs: () => safeRequest<{ ok: boolean; ips?: UserIP[]; limit?: number; count?: number; error?: string }>('/me/ips'),
  addMyIP: (ip: string) => safeRequest<APIResponse>(`/me/ips?ip=${encodeURIComponent(ip)}`, { method: 'POST' }),
  removeMyIP: (ip: string) => safeRequest<APIResponse>(`/me/ips?ip=${encodeURIComponent(ip)}`, { method: 'DELETE' }),
  userIPs: (id: number) => safeRequest<{ ok: boolean; ips?: UserIP[]; limit?: number; count?: number; error?: string }>(`/users/${id}/ips`),
  addUserIP: (id: number, ip: string) => safeRequest<APIResponse>(`/users/${id}/ips?ip=${encodeURIComponent(ip)}`, { method: 'POST' }),
  removeUserIP: (id: number, ip: string) => safeRequest<APIResponse>(`/users/${id}/ips?ip=${encodeURIComponent(ip)}`, { method: 'DELETE' }),
  myIP: async (): Promise<{ ip: string | null }> => {
    try {
      const r = await fetch('https://api.ipify.org?format=json', { cache: 'no-store' })
      const j = (await r.json()) as { ip?: string }
      return { ip: j.ip ?? null }
    } catch { return { ip: null } }
  },
  health: async () => {
    try {
      const res = await fetch('/healthz?detailed=1', { credentials: 'include', headers: { Accept: 'application/json' } })
      return await res.json()
    } catch (e) {
      return { ok: false, error: e instanceof Error ? e.message : 'failed' }
    }
  },
  status: () =>
    safeRequest<
      APIResponse & {
        version?: string
        host?: string
        vps_ip?: string
        uptime_seconds?: number
        allowlisted?: string[]
        restricted?: string[]
      }
    >('/status'),
}

export interface DomainRequest {
  id: number
  user_id: number
  username: string
  domain: string
  status: string
  created_at: string
}

export { ApiError }
export type { RequestOpts }
