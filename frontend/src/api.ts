import type { Application, Interview, Contact, Stats, PaginatedResponse, DashboardPeriod, ActivityItem } from './types'
import { t } from './i18n'

const BASE = '/api'

// IDs go through encodeURIComponent: an imported ID holding "../" must not
// resolve to another route.
const seg = encodeURIComponent

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(BASE + path, {
      headers: { 'Content-Type': 'application/json' },
      ...options,
    })
  } catch {
    throw new Error(t('common.network_error'))
  }
  if (!res.ok) {
    // The API answers errors as {"error": "..."}. A proxy error page (HTML) or
    // an empty body gets a readable, translated message instead.
    const body = await res.json().catch(() => null)
    if (typeof body?.error === 'string' && body.error) throw new Error(body.error)
    const key = res.status === 502 || res.status === 503 || res.status === 504
      ? 'common.server_unreachable'
      : 'common.server_error'
    throw new Error(t(key).replace('{status}', String(res.status)))
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

/** Today's local calendar date as stored for applied_at: `YYYY-MM-DDT00:00:00Z`. */
function localCalendarDay(d = new Date()): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}T00:00:00Z`
}

/** The browser's IANA time zone (e.g. "Europe/Paris"), so the server can cut
 *  days at local midnight. Empty when unknown: the server then uses UTC. */
function timeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || ''
  } catch {
    return ''
  }
}

function withTz(q: URLSearchParams): string {
  const tz = timeZone()
  if (tz) q.set('tz', tz)
  const qs = q.toString()
  return qs ? '?' + qs : ''
}

export const api = {
  applications: {
    list: (params?: { status?: string; source?: string; search?: string; sort?: string; dir?: string; page?: number; per_page?: number; has_interviews?: boolean; has_reply?: boolean; sent?: boolean; period?: string }) => {
      const q = new URLSearchParams()
      if (params) {
        for (const [k, v] of Object.entries(params)) {
          // The API expects the flag as `has_interviews=1`, and absent when off
          // — `String(true)` would send the string "true".
          if (k === 'has_interviews' || k === 'has_reply' || k === 'sent') { if (v) q.set(k, '1'); continue }
          if (v !== undefined && v !== '') q.set(k, String(v))
        }
      }
      const qs = q.toString()
      return request<PaginatedResponse<Application>>(`/applications${qs ? '?' + qs : ''}`)
    },
    get: (id: string) => request<Application>(`/applications/${seg(id)}`),
    create: (data: Partial<Application>) =>
      request<Application>('/applications', { method: 'POST', body: JSON.stringify(data) }),
    update: (id: string, data: Partial<Application>) =>
      request<Application>(`/applications/${seg(id)}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/applications/${seg(id)}`, { method: 'DELETE' }),
    snooze: (id: string, data: { until?: string; skip?: boolean }) =>
      request<{ status: string }>(`/applications/${seg(id)}/snooze`, { method: 'PUT', body: JSON.stringify(data) }),
    checkDuplicates: (companyName: string) =>
      request<Array<{ id: string; company_name: string; job_title: string; status: string; created_at: string }>>(
        `/applications/duplicates?company_name=${encodeURIComponent(companyName)}`),
    // applied_at is the user's local calendar day, stamped by the server only
    // on applications moving to Applied without a sent date.
    bulkStatus: (ids: string[], status: string) =>
      request<{ updated: number }>('/applications/bulk/status', {
        method: 'PUT', body: JSON.stringify({ ids, status, applied_at: localCalendarDay() }),
      }),
    bulkDelete: (ids: string[]) =>
      request<{ deleted: number }>('/applications/bulk', { method: 'DELETE', body: JSON.stringify({ ids }) }),
  },
  interviews: {
    list: (appId: string) => request<Interview[]>(`/applications/${seg(appId)}/interviews`),
    create: (appId: string, data: Partial<Interview>) =>
      request<Interview>(`/applications/${seg(appId)}/interviews`, { method: 'POST', body: JSON.stringify(data) }),
    update: (id: string, data: Partial<Interview>) =>
      request<Interview>(`/interviews/${seg(id)}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/interviews/${seg(id)}`, { method: 'DELETE' }),
  },
  contacts: {
    list: (appId: string) => request<Contact[]>(`/applications/${seg(appId)}/contacts`),
    create: (appId: string, data: Partial<Contact>) =>
      request<Contact>(`/applications/${seg(appId)}/contacts`, { method: 'POST', body: JSON.stringify(data) }),
    update: (id: string, data: Partial<Contact>) =>
      request<Contact>(`/contacts/${seg(id)}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/contacts/${seg(id)}`, { method: 'DELETE' }),
  },
  extract: (url: string) =>
    request<Partial<Application>>('/extract', { method: 'POST', body: JSON.stringify({ url }) }),
  sources: () => request<string[]>('/sources'),
  /** Timeline events for one local day (`YYYY-MM-DD`) in the browser's time
   *  zone: the heatmap's cell key. */
  activityByDay: (date: string) =>
    request<ActivityItem[]>(`/activity${withTz(new URLSearchParams({ date }))}`),
  stats: (period?: DashboardPeriod) =>
    request<Stats>(`/stats${withTz(new URLSearchParams(period ? { period } : {}))}`),
  export: () => request<unknown>('/export'),
  import_: (data: unknown) =>
    request<{ imported: number; skipped: number; total: number }>('/import', { method: 'POST', body: JSON.stringify(data) }),
}
