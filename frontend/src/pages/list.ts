import { api } from '../api'
import { createLayout } from '../components/layout'
import { navigate } from '../router'
import { t, getDateLocale } from '../i18n'
import { icons } from '../icons'
import { esc } from '../sanitize'
import { statusLabel, STATUS_COLORS, ALL_STATUSES, type Application, type ApplicationStatus } from '../types'

export async function ListPage(): Promise<HTMLElement> {
  let statusFilter = ''
  let searchQuery = ''
  let viewMode: 'table' | 'kanban' = (localStorage.getItem('jc-view') as 'table' | 'kanban') || 'table'
  let debounceTimer: ReturnType<typeof setTimeout> | null = null

  const content = document.createElement('div')
  content.className = 'space-y-6 stagger'

  function renderTable(apps: Application[]): string {
    if (apps.length === 0) return `
      <div class="card text-center py-20">
        <div class="text-muted/20 mb-4 flex justify-center">${icons.briefcaseLg}</div>
        <p class="text-muted text-base mb-5">${t('list.empty')}</p>
        <a href="/applications/new" data-link class="btn-primary mt-2 inline-flex gap-1.5">${icons.plus} ${t('list.add_first')}</a>
      </div>
    `
    return `<div class="space-y-2">
      ${apps.map(app => `
        <div
          class="card card-hover flex items-center justify-between gap-4 cursor-pointer group focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50"
          data-app-id="${app.id}"
          tabindex="0"
          role="link"
          aria-label="${esc(app.company_name)} — ${esc(app.job_title)}"
        >
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 mb-1.5">
              <span class="font-semibold text-primary truncate">${esc(app.company_name)}</span>
              <span class="text-border">&middot;</span>
              <span class="text-muted truncate text-sm">${esc(app.job_title)}</span>
            </div>
            <div class="flex items-center gap-2.5 flex-wrap">
              <span class="badge ${STATUS_COLORS[app.status as ApplicationStatus]}">${statusLabel(app.status as ApplicationStatus)}</span>
              ${app.location ? `<span class="text-xs text-muted flex items-center gap-1"><span aria-hidden="true" class="opacity-60">${icons.pin}</span> ${esc(app.location)}</span>` : ''}
              ${app.salary_min ? `<span class="text-xs text-muted tabular-nums font-medium">${app.salary_min / 1000}k\u2013${(app.salary_max ?? app.salary_min) / 1000}k \u20ac</span>` : ''}
              <span class="text-xs text-muted/60 tabular-nums">${new Date(app.created_at).toLocaleDateString(getDateLocale())}</span>
            </div>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            ${app.rating ? `<span class="text-amber-400 text-sm flex gap-px">${Array.from({ length: app.rating }, () => icons.star).join('')}</span>` : ''}
            <button data-delete-id="${app.id}" class="btn-ghost opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 text-red-500 dark:text-red-400 hover:bg-red-500/10 p-1.5 transition-all duration-150" title="${t('detail.delete')}">
              ${icons.trash}
            </button>
          </div>
        </div>
      `).join('')}
    </div>`
  }

  function renderKanban(apps: Application[]): string {
    const columns = statusFilter ? [statusFilter as ApplicationStatus] : ALL_STATUSES
    const byStatus: Record<string, Application[]> = {}
    for (const s of ALL_STATUSES) byStatus[s] = []
    for (const app of apps) {
      if (byStatus[app.status]) byStatus[app.status].push(app)
    }

    return `
      <div class="overflow-x-auto pb-4 -mx-1 px-1">
        <div class="flex gap-3" style="min-width: max-content;">
          ${columns.map(status => {
            const colApps = byStatus[status] || []
            return `
              <div class="w-64 flex-shrink-0 flex flex-col">
                <div class="flex items-center justify-between mb-3 px-1">
                  <span class="badge ${STATUS_COLORS[status]} text-xs">${statusLabel(status)}</span>
                  <span class="text-xs text-muted/60 font-medium tabular-nums">${colApps.length}</span>
                </div>
                <div class="space-y-2 flex-1 min-h-24 bg-surface-2/30 rounded-xl p-2" data-kanban-col="${status}">
                  ${colApps.length === 0 ? `
                    <div class="rounded-lg border border-dashed border-border/60 h-20 flex items-center justify-center">
                      <span class="text-xs text-muted/30">${t('list.kanban_empty')}</span>
                    </div>
                  ` : colApps.map(app => `
                    <div
                      class="card !p-3.5 cursor-pointer card-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50"
                      data-app-id="${app.id}"
                      tabindex="0"
                      role="link"
                      aria-label="${esc(app.company_name)} — ${esc(app.job_title)}"
                    >
                      <div class="font-semibold text-primary text-sm truncate mb-0.5">${esc(app.company_name)}</div>
                      <div class="text-muted text-xs truncate mb-2.5">${esc(app.job_title)}</div>
                      <div class="flex items-center justify-between gap-1">
                        <span class="text-xs text-muted/70 tabular-nums font-medium">${app.salary_min ? `${app.salary_min / 1000}k\u2013${(app.salary_max ?? app.salary_min) / 1000}k \u20ac` : ''}</span>
                        ${app.rating ? `<span class="text-amber-400 text-xs flex gap-px shrink-0">${Array.from({ length: app.rating }, () => icons.star).join('')}</span>` : ''}
                      </div>
                      ${app.applied_at ? `<div class="text-[11px] text-muted/50 mt-1.5 tabular-nums">${new Date(app.applied_at).toLocaleDateString(getDateLocale())}</div>` : ''}
                    </div>
                  `).join('')}
                </div>
              </div>
            `
          }).join('')}
        </div>
      </div>
    `
  }

  async function load() {
    const resp = await api.applications.list({
      status: statusFilter,
      search: searchQuery,
      per_page: viewMode === 'kanban' ? 200 : 20,
    }).catch(() => ({ data: [], total: 0, page: 1, per_page: 20, total_pages: 1 }))
    const apps = resp.data

    const viewToggle = `
      <div class="flex items-center gap-0.5 rounded-lg border border-border p-0.5 bg-surface">
        <button
          id="view-table"
          class="p-1.5 rounded-md transition-all duration-150 ${viewMode === 'table' ? 'bg-accent/15 text-accent shadow-sm' : 'text-muted hover:text-primary'}"
          title="${t('list.view_table')}"
          aria-pressed="${viewMode === 'table'}"
        >${icons.tableView}</button>
        <button
          id="view-kanban"
          class="p-1.5 rounded-md transition-all duration-150 ${viewMode === 'kanban' ? 'bg-accent/15 text-accent shadow-sm' : 'text-muted hover:text-primary'}"
          title="${t('list.view_kanban')}"
          aria-pressed="${viewMode === 'kanban'}"
        >${icons.kanban}</button>
      </div>
    `

    content.innerHTML = `
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <h1 class="text-2xl font-bold text-primary tracking-tight">${t('list.title')}</h1>
        <div class="flex items-center gap-3">
          ${viewToggle}
          <a href="/applications/new" data-link class="btn-primary gap-1.5">${icons.plus} ${t('list.new')}</a>
        </div>
      </div>

      <div class="flex flex-col sm:flex-row gap-3">
        <div class="flex-1 relative">
          <label for="search-input" class="sr-only">${t('common.search')}</label>
          <span class="absolute left-3.5 top-1/2 -translate-y-1/2 text-muted/50 pointer-events-none" aria-hidden="true">${icons.search}</span>
          <input
            type="search"
            id="search-input"
            placeholder="${t('list.search')}"
            class="input pl-10"
            value="${searchQuery}"
          />
        </div>
        <div class="sm:w-48">
          <label for="status-filter" class="sr-only">${t('common.filter_status')}</label>
          <select id="status-filter" class="select">
            <option value="">${t('list.all_statuses')}</option>
            ${ALL_STATUSES.map(s => `<option value="${s}" ${statusFilter === s ? 'selected' : ''}>${statusLabel(s)}</option>`).join('')}
          </select>
        </div>
      </div>

      <div id="apps-content">
        ${viewMode === 'kanban' ? renderKanban(apps) : renderTable(apps)}
      </div>
    `

    content.querySelector('#view-table')?.addEventListener('click', () => {
      viewMode = 'table'
      localStorage.setItem('jc-view', 'table')
      load()
    })
    content.querySelector('#view-kanban')?.addEventListener('click', () => {
      viewMode = 'kanban'
      localStorage.setItem('jc-view', 'kanban')
      load()
    })

    content.querySelector('#search-input')?.addEventListener('input', (e) => {
      searchQuery = (e.target as HTMLInputElement).value
      if (debounceTimer) clearTimeout(debounceTimer)
      debounceTimer = setTimeout(() => load(), 200)
    })
    content.querySelector('#status-filter')?.addEventListener('change', (e) => {
      statusFilter = (e.target as HTMLSelectElement).value
      load()
    })

    content.querySelectorAll('[data-app-id]').forEach(el => {
      const navToApp = (e: Event) => {
        const target = e.target as HTMLElement
        if (target.closest('[data-delete-id]')) return
        navigate('/applications/' + el.getAttribute('data-app-id'))
      }
      el.addEventListener('click', navToApp)
      el.addEventListener('keydown', (e) => {
        if ((e as KeyboardEvent).key === 'Enter' || (e as KeyboardEvent).key === ' ') {
          e.preventDefault()
          navToApp(e)
        }
      })
    })

    content.querySelectorAll('[data-delete-id]').forEach(el => {
      el.addEventListener('click', async () => {
        if (!confirm(t('list.confirm_delete'))) return
        await api.applications.delete(el.getAttribute('data-delete-id')!)
        load()
      })
    })
  }

  await load()
  return createLayout(content)
}
