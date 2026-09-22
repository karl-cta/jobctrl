import type { Application, Interview, Contact, Stats, PaginatedResponse, DashboardPeriod, ActivityItem } from './types'

const BASE = '/api'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  applications: {
    list: (params?: { status?: string; source?: string; search?: string; sort?: string; dir?: string; page?: number; per_page?: number; has_interviews?: boolean; has_reply?: boolean }) => {
      const q = new URLSearchParams()
      if (params) {
        for (const [k, v] of Object.entries(params)) {
          // The API expects the flag as `has_interviews=1`, and absent when off
          // — `String(true)` would send the string "true".
          if (k === 'has_interviews' || k === 'has_reply') { if (v) q.set(k, '1'); continue }
          if (v !== undefined && v !== '') q.set(k, String(v))
        }
      }
      const qs = q.toString()
      return request<PaginatedResponse<Application>>(`/applications${qs ? '?' + qs : ''}`)
    },
    get: (id: string) => request<Application>(`/applications/${id}`),
    create: (data: Partial<Application>) =>
      request<Application>('/applications', { method: 'POST', body: JSON.stringify(data) }),
    update: (id: string, data: Partial<Application>) =>
      request<Application>(`/applications/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/applications/${id}`, { method: 'DELETE' }),
    snooze: (id: string, data: { until?: string; skip?: boolean }) =>
      request<{ status: string }>(`/applications/${id}/snooze`, { method: 'PUT', body: JSON.stringify(data) }),
    checkDuplicates: (companyName: string) =>
      request<Array<{ id: string; company_name: string; job_title: string; status: string; created_at: string }>>(
        `/applications/duplicates?company_name=${encodeURIComponent(companyName)}`),
    bulkStatus: (ids: string[], status: string) =>
      request<{ updated: number }>('/applications/bulk/status', { method: 'PUT', body: JSON.stringify({ ids, status }) }),
    bulkDelete: (ids: string[]) =>
      request<{ deleted: number }>('/applications/bulk', { method: 'DELETE', body: JSON.stringify({ ids }) }),
  },
  interviews: {
    list: (appId: string) => request<Interview[]>(`/applications/${appId}/interviews`),
    create: (appId: string, data: Partial<Interview>) =>
      request<Interview>(`/applications/${appId}/interviews`, { method: 'POST', body: JSON.stringify(data) }),
    update: (id: string, data: Partial<Interview>) =>
      request<Interview>(`/interviews/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/interviews/${id}`, { method: 'DELETE' }),
  },
  contacts: {
    list: (appId: string) => request<Contact[]>(`/applications/${appId}/contacts`),
    create: (appId: string, data: Partial<Contact>) =>
      request<Contact>(`/applications/${appId}/contacts`, { method: 'POST', body: JSON.stringify(data) }),
    update: (id: string, data: Partial<Contact>) =>
      request<Contact>(`/contacts/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: string) => request<void>(`/contacts/${id}`, { method: 'DELETE' }),
  },
  extract: (url: string) =>
    request<Partial<Application>>('/extract', { method: 'POST', body: JSON.stringify({ url }) }),
  sources: () => request<string[]>('/sources'),
  /** Timeline events for one UTC day (`YYYY-MM-DD`) — the heatmap's cell key. */
  activityByDay: (date: string) =>
    request<ActivityItem[]>(`/activity?date=${encodeURIComponent(date)}`),
  stats: (period?: DashboardPeriod) =>
    request<Stats>(`/stats${period ? `?period=${encodeURIComponent(period)}` : ''}`),
  export: () => request<unknown>('/export'),
  import_: (data: unknown) =>
    request<{ imported: number; skipped: number; total: number }>('/import', { method: 'POST', body: JSON.stringify(data) }),
}
