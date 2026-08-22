const API_BASE = '/api'

// ── Types ────────────────────────────────────────────────────────────
export interface UserData {
  id: number
  username: string
  api_key: string
  role: string
  rate_limit: number
  created_at: string
  last_login?: string | null
}

export interface DeviceData {
  id: number
  ip: string
  ja3_hash: string
  user_agent: string
  device_type: string
  first_seen: string
  last_seen: string
  user_id?: number
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
export interface DevicesResponse extends APIResponse {
  devices?: DeviceData[]
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

  createUser: (username: string, password: string, rate_limit?: number) =>
    safeRequest<CreateUserResponse>('/users', {
      method: 'POST',
      body: JSON.stringify({ username: username.trim(), password, rate_limit }),
    }),

  deleteUser: (id: number) =>
    safeRequest<APIResponse>(`/users/${id}`, { method: 'DELETE' }),

  getUserDevices: (id: number) =>
    safeRequest<DevicesResponse>(`/users/${id}/devices`),

  regenerateAPIKey: (id: number) =>
    safeRequest<RegenerateKeyResponse>(`/users/${id}/api-key/regenerate`, {
      method: 'POST',
    }),

  regenerateMyAPIKey: () =>
    safeRequest<RegenerateKeyResponse>('/me/api-key/regenerate', {
      method: 'POST',
    }),

  // Devices
  allDevices: () => safeRequest<DevicesResponse>('/devices'),
  myDevices: () => safeRequest<DevicesResponse>('/me/devices'),

  // Legacy / status (admin)
  allowList: () => safeRequest<{ ok: boolean; data?: string[]; error?: string }>('/allow'),
  addAllow: (ip: string) => safeRequest<APIResponse>(`/allow?ip=${encodeURIComponent(ip)}`, { method: 'POST' }),
  removeAllow: (ip: string) => safeRequest<APIResponse>(`/allow?ip=${encodeURIComponent(ip)}`, { method: 'DELETE' }),
  restrictedList: () =>
    safeRequest<{ ok: boolean; data?: string[]; error?: string }>('/restricted'),
  addRestricted: (domain: string) => safeRequest<APIResponse>(`/restricted?domain=${encodeURIComponent(domain)}`, { method: 'POST' }),
  removeRestricted: (domain: string) => safeRequest<APIResponse>(`/restricted?domain=${encodeURIComponent(domain)}`, { method: 'DELETE' }),
  // Domain check / request (new)
  checkDomain: (domain: string) => safeRequest<{ ok: boolean; restricted?: boolean; error?: string }>(`/domain/check?domain=${encodeURIComponent(domain)}`),
  requestDomain: (domain: string) => safeRequest<APIResponse>('/domain/request', { method: 'POST', body: JSON.stringify({ domain }) }),
  listDomainRequests: () => safeRequest<{ ok: boolean; requests?: DomainRequest[]; error?: string }>('/domain/requests'),
  approveDomainRequest: (id: number) => safeRequest<APIResponse>(`/domain/requests/${id}/approve`, { method: 'POST' }),
  rejectDomainRequest: (id: number) => safeRequest<APIResponse>(`/domain/requests/${id}/reject`, { method: 'POST' }),
  publicConfig: () => safeRequest<{ ok: boolean; admin_url?: string; doh_url?: string; host?: string; vps_ip?: string; error?: string }>('/public-config'),
  traffic: () => safeRequest<{ ok: boolean; total_users?: number; total_devices?: number; total_requests?: number; allowlisted?: number; restricted?: number; uptime_seconds?: number; version?: string; recent?: any[]; per_user?: any[]; error?: string }>('/traffic'),
  myTraffic: () => safeRequest<{ ok: boolean; devices?: number; total_requests?: number; recent?: any[]; rate_limit?: number; unlimited?: boolean; error?: string }>('/me/traffic'),
  updateUserRateLimit: (id: number, rate_limit: number) => safeRequest<APIResponse>(`/users/${id}/rate-limit`, { method: 'POST', body: JSON.stringify({ rate_limit }) }),
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
