import { api } from '../api'
import { createLayout } from '../components/layout'
import { navigate } from '../router'
import { t, tp, getDateLocale } from '../i18n'
import { icons } from '../icons'
import { esc } from '../sanitize'
import { toast } from '../components/toast'
import { statusLabel, STATUS_COLORS, ALL_STATUSES, type Application, type ApplicationStatus } from '../types'
import { faviconUrl, domainFromUrl } from '../job-boards'

const STATUS_BORDER: Record<string, string> = {
  Wishlist: 'border-l-stone-400 dark:border-l-stone-500',
  Applied: 'border-l-sky-500 dark:border-l-sky-400',
  Screening: 'border-l-amber-500 dark:border-l-amber-400',
  Interviewing: 'border-l-orange-500 dark:border-l-orange-400',
  Offer: 'border-l-emerald-500 dark:border-l-emerald-400',
  Accepted: 'border-l-teal-500 dark:border-l-teal-400',
  Rejected: 'border-l-rose-400',
  Withdrawn: 'border-l-stone-400 dark:border-l-stone-500',
}

const STATUS_TEXT: Record<string, string> = {
  Wishlist: 'text-stone-500 dark:text-stone-400',
  Applied: 'text-sky-600 dark:text-sky-400',
  Screening: 'text-amber-600 dark:text-amber-400',
  Interviewing: 'text-orange-600 dark:text-orange-400',
  Offer: 'text-emerald-600 dark:text-emerald-400',
  Accepted: 'text-teal-600 dark:text-teal-400',
  Rejected: 'text-rose-500 dark:text-rose-400',
  Withdrawn: 'text-stone-400 dark:text-stone-500',
}

const CONF_FILL: Record<number, string> = {
  1: 'bg-stone-400 dark:bg-stone-500',
  2: 'bg-amber-500 dark:bg-amber-400',
  3: 'bg-emerald-500 dark:bg-emerald-400',
  4: 'bg-teal-500 dark:bg-teal-400',
}

function companyFavicon(app: Application, cls = 'w-5 h-5'): string {
  const domain = app.company_website ? domainFromUrl(app.company_website) : null
  if (!domain) return ''
  return `<img src="${faviconUrl(domain, 64)}" alt="" class="source-favicon rounded shrink-0 ${cls}" onerror="this.style.display='none'" />`
}

function confidenceMeter(level: number): string {
  return `<span class="inline-flex gap-1 items-center" aria-label="${t('form.confidence')}: ${level}/4" title="${t('form.confidence_' + level)}">
    <span class="text-xs text-muted/60">${t('form.confidence')}</span>
    <span class="inline-flex gap-px items-center">${
    [1,2,3,4].map(n =>
      `<span class="w-1.5 h-3 rounded-sm ${n <= level ? (CONF_FILL[level] || 'bg-muted') : 'bg-surface-3/50'}"></span>`
    ).join('')
  }</span></span>`
}

export async function ListPage(): Promise<HTMLElement> {
  const urlParams = new URLSearchParams(window.location.search)
  let statusFilter = urlParams.get('status') || ''
  let sourceFilter = urlParams.get('source') || ''
  let searchQuery = ''
  let sortValue = localStorage.getItem('jc-sort') || 'created_at:desc'
  let currentPage = 1
  let viewMode: 'table' | 'kanban' = (localStorage.getItem('jc-view') as 'table' | 'kanban') || 'table'
  let debounceTimer: ReturnType<typeof setTimeout> | null = null
  const selectedIds = new Set<string>()
  let selectMode = false

  const content = document.createElement('div')
  content.className = 'space-y-6 stagger'

  const hasActiveFilters = () => !!(statusFilter || sourceFilter || searchQuery)

  function renderEmpty(): string {
    if (hasActiveFilters()) return `
      <div class="text-center py-24">
        <div class="text-muted/15 mb-6 flex justify-center">${icons.search}</div>
        <p class="text-primary text-lg font-semibold mb-2">${t('list.empty_filtered')}</p>
        <p class="text-muted text-sm">${t('list.empty_filtered_hint')}</p>
      </div>
    `
    return `
      <div class="text-center py-24">
        <div class="text-muted/15 mb-6 flex justify-center">${icons.briefcaseLg}</div>
        <p class="text-primary text-lg font-semibold mb-2">${t('list.empty')}</p>
        <p class="text-muted text-sm mb-8">${t('list.empty_hint')}</p>
        <a href="/applications/new" data-link class="btn-primary gap-1.5">${icons.plus} ${t('list.add_first')}</a>
      </div>
    `
  }

  function renderTable(apps: Application[]): string {
    if (apps.length === 0) return renderEmpty()
    return `<div class="space-y-2">
      ${apps.map(app => `
        <div
          class="card card-hover flex items-center justify-between gap-4 cursor-pointer group border-l-[3px] ${STATUS_BORDER[app.status] || 'border-l-border'} focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50"
          data-app-id="${app.id}"
          tabindex="0"
          role="link"
          aria-label="${esc(app.company_name)} — ${esc(app.job_title)}"
        >
          <button data-select-id="${app.id}" class="shrink-0 w-5 h-5 rounded-full border-2 flex items-center justify-center transition-colors ${selectedIds.has(app.id) ? 'bg-accent border-accent text-white' : 'border-border hover:border-accent/50'} ${selectMode ? '' : 'hidden'}" aria-pressed="${selectedIds.has(app.id)}">${selectedIds.has(app.id) ? '<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/></svg>' : ''}</button>
          <div class="flex-1 min-w-0">
            <div class="mb-1.5">
              <span class="font-semibold text-primary block break-words sm:truncate">${companyFavicon(app, 'w-5 h-5 sm:w-6 sm:h-6 inline-block -mt-0.5 mr-1.5')}${esc(app.company_name)}</span>
              <span class="text-muted text-sm block break-words sm:truncate">${esc(app.job_title)}</span>
            </div>
            <div class="flex items-center gap-2.5 flex-wrap">
              <span class="text-xs font-semibold uppercase tracking-wide ${STATUS_TEXT[app.status] || 'text-muted'}">${statusLabel(app.status as ApplicationStatus)}</span>
              ${app.confidence ? confidenceMeter(app.confidence) : ''}
              ${app.location ? `<span class="text-xs text-muted flex items-center gap-1"><span aria-hidden="true" class="opacity-60">${icons.pin}</span> ${esc(app.location)}</span>` : ''}
              ${app.salary ? `<span class="text-xs text-muted tabular-nums font-medium">${app.salary / 1000}k \u20ac</span>` : ''}
              ${app.applied_at ? `<span class="text-xs text-muted/60 tabular-nums">${new Date(app.applied_at).toLocaleDateString(getDateLocale())}</span>` : ''}
            </div>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            ${app.rating ? `<span class="text-amber-400 text-sm flex gap-px">${Array.from({ length: app.rating }, () => icons.star).join('')}</span>` : ''}
            <button data-delete-id="${app.id}" class="btn-ghost sm:opacity-0 sm:group-hover:opacity-100 group-focus-visible:opacity-100 text-red-500 dark:text-red-400 hover:bg-red-500/10 p-1.5 min-w-[44px] min-h-[44px] transition-all duration-150" title="${t('detail.delete')}">
              ${icons.trash}
            </button>
          </div>
        </div>
      `).join('')}
    </div>`
  }

  function renderKanban(apps: Application[], total: number): string {
    const truncated = total > apps.length
    const columns = statusFilter ? [statusFilter as ApplicationStatus] : ALL_STATUSES
    const byStatus: Record<string, Application[]> = {}
    for (const s of ALL_STATUSES) byStatus[s] = []
    for (const app of apps) {
      if (byStatus[app.status]) byStatus[app.status].push(app)
    }

    return `
      ${truncated ? `<div class="text-xs text-muted bg-surface-2/50 rounded px-3 py-2 mb-3">${apps.length} / ${total} ${tp('list.result_count', total)}. ${t('list.kanban_filter_hint')}</div>` : ''}
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
                <div class="space-y-2 flex-1 min-h-24 bg-surface-2/30 rounded p-2" data-kanban-col="${status}">
                  ${colApps.length === 0 ? `
                    <div class="border border-dashed border-border/60 h-20 flex items-center justify-center">
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
                      <div class="flex items-center gap-2 mb-0.5">${companyFavicon(app)}<span class="font-semibold text-primary text-sm truncate">${esc(app.company_name)}</span></div>
                      <div class="text-muted text-xs truncate mb-2.5">${esc(app.job_title)}</div>
                      <div class="flex items-center justify-between gap-1">
                        <span class="text-xs text-muted/70 tabular-nums font-medium">${app.salary ? `${app.salary / 1000}k \u20ac` : ''}</span>
                        ${app.rating ? `<span class="text-amber-400 text-xs flex gap-px shrink-0">${Array.from({ length: app.rating }, () => icons.star).join('')}</span>` : ''}
                      </div>
                      ${app.applied_at ? `<div class="text-xs text-muted/60 mt-1.5 tabular-nums">${new Date(app.applied_at).toLocaleDateString(getDateLocale())}</div>` : ''}
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

  function toggleSelectMode(on?: boolean, selectAll = false) {
    selectMode = on ?? !selectMode
    selectedIds.clear()
    content.querySelectorAll<HTMLButtonElement>('[data-select-id]').forEach(btn => {
      btn.classList.toggle('hidden', !selectMode)
      const id = btn.dataset.selectId!
      if (selectMode && selectAll) {
        selectedIds.add(id)
        btn.className = btn.className.replace(/border-border hover:border-accent\/50/, 'bg-accent border-accent text-white')
        btn.innerHTML = '<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/></svg>'
        btn.setAttribute('aria-pressed', 'true')
      } else {
        btn.className = btn.className.replace(/bg-accent border-accent text-white/, 'border-border hover:border-accent/50')
        btn.innerHTML = ''
        btn.setAttribute('aria-pressed', 'false')
      }
    })
    const modeBtn = content.querySelector('#select-mode-btn')
    if (modeBtn) modeBtn.textContent = selectMode ? t('form.cancel') : t('list.select')
    updateBulkBar()
  }

  function updateBulkBar() {
    const bar = content.querySelector('#bulk-bar') as HTMLElement
    if (!bar) return
    const count = selectedIds.size
    if (count === 0) {
      bar.classList.add('hidden')
      return
    }
    bar.classList.remove('hidden')
    const countEl = bar.querySelector('#bulk-count')
    if (countEl) countEl.textContent = count === 1 ? t('list.selected_one') : t('list.selected_other').replace('{count}', String(count))
  }

  function attachCardListeners() {
    content.querySelectorAll('[data-app-id]').forEach(el => {
      const navToApp = (e: Event) => {
        const target = e.target as HTMLElement
        if (target.closest('[data-delete-id]') || target.closest('[data-select-id]')) return
        navigate('/applications/' + el.getAttribute('data-app-id'))
      }
      el.addEventListener('click', navToApp)
      el.addEventListener('keydown', (e) => {
        if ((e as KeyboardEvent).key === 'Enter' || (e as KeyboardEvent).key === ' ') {
          if ((e.target as HTMLElement).matches('[data-select-id]')) return
          e.preventDefault()
          navToApp(e)
        }
      })
    })

    // Selection toggle
    content.querySelectorAll<HTMLButtonElement>('[data-select-id]').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation()
        const id = btn.dataset.selectId!
        if (selectedIds.has(id)) {
          selectedIds.delete(id)
          btn.className = btn.className.replace(/bg-accent border-accent text-white/, 'border-border hover:border-accent/50')
          btn.innerHTML = ''
          btn.setAttribute('aria-pressed', 'false')
        } else {
          selectedIds.add(id)
          btn.className = btn.className.replace(/border-border hover:border-accent\/50/, 'bg-accent border-accent text-white')
          btn.innerHTML = '<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/></svg>'
          btn.setAttribute('aria-pressed', 'true')
        }
        updateBulkBar()
      })
    })

    // Long press to enter select mode (mobile)
    content.querySelectorAll<HTMLElement>('[data-app-id]').forEach(el => {
      let longPressTimer: ReturnType<typeof setTimeout> | null = null
      el.addEventListener('touchstart', () => {
        longPressTimer = setTimeout(() => {
          if (!selectMode) toggleSelectMode(true)
          const id = el.dataset.appId!
          const btn = el.querySelector<HTMLButtonElement>(`[data-select-id="${id}"]`)
          if (btn) btn.click()
          longPressTimer = null
        }, 500)
      }, { passive: true })
      el.addEventListener('touchend', () => { if (longPressTimer) clearTimeout(longPressTimer) })
      el.addEventListener('touchmove', () => { if (longPressTimer) clearTimeout(longPressTimer) })
    })

    content.querySelectorAll('[data-delete-id]').forEach(el => {
      el.addEventListener('click', async () => {
        if (!confirm(t('list.confirm_delete'))) return
        const card = el.closest('[data-app-id]') as HTMLElement | null
        if (card) {
          card.classList.add('card-exit')
          await new Promise(r => setTimeout(r, 200))
        }
        await api.applications.delete(el.getAttribute('data-delete-id')!)
        load()
      })
    })
  }

  function updateViewToggle() {
    const tableBtn = content.querySelector('#view-table') as HTMLElement | null
    const kanbanBtn = content.querySelector('#view-kanban') as HTMLElement | null
    if (tableBtn) {
      tableBtn.className = `p-1.5 rounded transition-colors duration-100 ${viewMode === 'table' ? 'bg-accent/15 text-accent' : 'text-muted hover:text-primary'}`
      tableBtn.setAttribute('aria-pressed', String(viewMode === 'table'))
    }
    if (kanbanBtn) {
      kanbanBtn.className = `p-1.5 rounded transition-colors duration-100 ${viewMode === 'kanban' ? 'bg-accent/15 text-accent' : 'text-muted hover:text-primary'}`
      kanbanBtn.setAttribute('aria-pressed', String(viewMode === 'kanban'))
    }
  }

  async function load() {
    const [sortField, sortDir] = sortValue.split(':')
    const resp = await api.applications.list({
      status: statusFilter,
      source: sourceFilter,
      search: searchQuery,
      sort: sortField,
      dir: sortDir,
      page: viewMode === 'table' ? currentPage : undefined,
      per_page: viewMode === 'kanban' ? 200 : 20,
    }).catch(() => ({ data: [], total: 0, page: 1, per_page: 20, total_pages: 1 }))
    const apps = resp.data

    const appsContainer = content.querySelector('#apps-content')

    if (!appsContainer) {
      const viewToggle = `
        <div class="flex items-center gap-0.5 rounded border border-border p-0.5 bg-surface">
          <button
            id="view-table"
            class="p-1.5 rounded transition-colors duration-100 ${viewMode === 'table' ? 'bg-accent/15 text-accent' : 'text-muted hover:text-primary'}"
            title="${t('list.view_table')}"
            aria-pressed="${viewMode === 'table'}"
          >${icons.tableView}</button>
          <button
            id="view-kanban"
            class="p-1.5 rounded transition-colors duration-100 ${viewMode === 'kanban' ? 'bg-accent/15 text-accent' : 'text-muted hover:text-primary'}"
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
            <button id="select-mode-btn" class="btn-ghost text-sm py-1.5 px-3 ${viewMode === 'kanban' ? 'hidden' : ''}">${t('list.select')}</button>
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
          <div class="flex gap-3 sm:contents">
            <div class="flex-1 sm:flex-none sm:w-48">
              <label for="status-filter" class="sr-only">${t('common.filter_status')}</label>
              <select id="status-filter" class="select">
                <option value="">${t('list.all_statuses')}</option>
                ${ALL_STATUSES.map(s => `<option value="${s}" ${statusFilter === s ? 'selected' : ''}>${statusLabel(s)}</option>`).join('')}
              </select>
            </div>
            <div class="flex-1 sm:flex-none sm:w-48">
              <label for="sort-select" class="sr-only">${t('list.sort')}</label>
              <select id="sort-select" class="select">
                <option value="created_at:desc" ${sortValue === 'created_at:desc' ? 'selected' : ''}>${t('list.sort_date_desc')}</option>
                <option value="created_at:asc" ${sortValue === 'created_at:asc' ? 'selected' : ''}>${t('list.sort_date_asc')}</option>
                <option value="company_name:asc" ${sortValue === 'company_name:asc' ? 'selected' : ''}>${t('list.sort_company_az')}</option>
                <option value="company_name:desc" ${sortValue === 'company_name:desc' ? 'selected' : ''}>${t('list.sort_company_za')}</option>
                <option value="status:asc" ${sortValue === 'status:asc' ? 'selected' : ''}>${t('list.sort_status')}</option>
                <option value="confidence:desc" ${sortValue === 'confidence:desc' ? 'selected' : ''}>${t('list.sort_confidence_desc')}</option>
                <option value="confidence:asc" ${sortValue === 'confidence:asc' ? 'selected' : ''}>${t('list.sort_confidence_asc')}</option>
                <option value="rating:desc" ${sortValue === 'rating:desc' ? 'selected' : ''}>${t('list.sort_rating_desc')}</option>
                <option value="rating:asc" ${sortValue === 'rating:asc' ? 'selected' : ''}>${t('list.sort_rating_asc')}</option>
              </select>
            </div>
          </div>
        </div>

        ${sourceFilter ? `
        <div id="source-chip" class="flex items-center gap-2 chip-enter">
          <span class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-accent/10 text-accent text-sm font-medium">
            ${t('list.source_filter')}: ${esc(sourceFilter)}
            <button id="clear-source" class="hover:bg-accent/20 rounded-full p-0.5 transition-colors" title="${t('list.clear_source')}">
              <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
            </button>
          </span>
        </div>
        ` : ''}

        <div class="flex items-center justify-between">
          <span id="result-count" class="text-xs text-muted/60 tabular-nums">${resp.total} ${tp('list.result_count', resp.total)}</span>
          ${viewMode === 'table' && resp.total_pages > 1 ? `
          <div id="pagination" class="flex items-center gap-1.5">
            <button id="prev-page" class="btn-ghost p-1.5 ${resp.page <= 1 ? 'opacity-30 pointer-events-none' : ''}" title="${t('list.page_prev')}" ${resp.page <= 1 ? 'disabled' : ''}>
              ${icons.chevronLeft}
            </button>
            <span id="page-indicator" class="text-xs text-muted tabular-nums px-1">${resp.page} / ${resp.total_pages}</span>
            <button id="next-page" class="btn-ghost p-1.5 ${resp.page >= resp.total_pages ? 'opacity-30 pointer-events-none' : ''}" title="${t('list.page_next')}" ${resp.page >= resp.total_pages ? 'disabled' : ''}>
              ${icons.chevronRight}
            </button>
          </div>
          ` : ''}
        </div>

        <div id="apps-content">
          ${viewMode === 'kanban' ? renderKanban(apps, resp.total) : renderTable(apps)}
        </div>

        <div id="bulk-bar" class="fixed bottom-0 left-0 right-0 sm:bottom-6 sm:left-1/2 sm:-translate-x-1/2 sm:right-auto z-50 hidden">
          <div class="flex items-center gap-2 sm:gap-3 px-4 py-3 sm:px-5 sm:rounded-xl border-t sm:border border-border shadow-elevated" style="background: rgb(var(--color-surface-1));">
            <span id="bulk-count" class="text-sm font-medium text-primary whitespace-nowrap shrink-0"></span>
            <div class="hidden sm:block w-px h-5 bg-border"></div>
            <select id="bulk-status-select" class="select text-sm py-2 px-2 pr-7 min-w-0 flex-1 sm:flex-none sm:w-auto">
              ${ALL_STATUSES.map(s => `<option value="${s}">${statusLabel(s)}</option>`).join('')}
            </select>
            <button id="bulk-status-btn" class="btn-primary text-sm py-2 px-3" title="${t('list.bulk_status')}"><span class="sm:hidden">OK</span><span class="hidden sm:inline">${t('list.bulk_status')}</span></button>
            <div class="hidden sm:block w-px h-5 bg-border"></div>
            <button id="bulk-delete-btn" class="btn-ghost text-red-500 dark:text-red-400 p-2 sm:px-3 sm:py-2" title="${t('list.bulk_delete')}"><span class="sm:hidden">${icons.trash}</span><span class="hidden sm:inline">${t('list.bulk_delete')}</span></button>
          </div>
        </div>
      `

      content.querySelector('#view-table')?.addEventListener('click', () => {
        viewMode = 'table'
        localStorage.setItem('jc-view', 'table')
        currentPage = 1
        const selBtn = content.querySelector('#select-mode-btn') as HTMLElement
        if (selBtn) selBtn.classList.remove('hidden')
        load()
      })
      content.querySelector('#view-kanban')?.addEventListener('click', () => {
        viewMode = 'kanban'
        localStorage.setItem('jc-view', 'kanban')
        toggleSelectMode(false)
        const selBtn = content.querySelector('#select-mode-btn') as HTMLElement
        if (selBtn) selBtn.classList.add('hidden')
        load()
      })
      content.querySelector('#select-mode-btn')?.addEventListener('click', () => {
        toggleSelectMode(!selectMode)
      })

      content.querySelector('#search-input')?.addEventListener('input', (e) => {
        searchQuery = (e.target as HTMLInputElement).value
        currentPage = 1
        if (debounceTimer) clearTimeout(debounceTimer)
        debounceTimer = setTimeout(() => load(), 200)
      })
      content.querySelector('#status-filter')?.addEventListener('change', (e) => {
        statusFilter = (e.target as HTMLSelectElement).value
        currentPage = 1
        load()
      })
      content.querySelector('#sort-select')?.addEventListener('change', (e) => {
        sortValue = (e.target as HTMLSelectElement).value
        localStorage.setItem('jc-sort', sortValue)
        currentPage = 1
        load()
      })
      content.querySelector('#prev-page')?.addEventListener('click', () => {
        if (currentPage > 1) { currentPage--; load() }
      })
      content.querySelector('#next-page')?.addEventListener('click', () => {
        currentPage++; load()
      })
      content.querySelector('#clear-source')?.addEventListener('click', () => {
        sourceFilter = ''
        currentPage = 1
        window.history.replaceState({}, '', '/applications')
        const chip = content.querySelector('#source-chip')
        if (chip) {
          chip.classList.replace('chip-enter', 'chip-exit')
          chip.addEventListener('animationend', () => chip.remove(), { once: true })
        }
        load()
      })

      // Bulk actions
      content.querySelector('#bulk-status-btn')?.addEventListener('click', async () => {
        const status = (content.querySelector('#bulk-status-select') as HTMLSelectElement).value
        const ids = [...selectedIds]
        const result = await api.applications.bulkStatus(ids, status)
        toast(t('list.bulk_done').replace('{count}', String(result.updated)), 'success')
        toggleSelectMode(false)
        load()
      })
      content.querySelector('#bulk-delete-btn')?.addEventListener('click', async () => {
        const ids = [...selectedIds]
        if (!confirm(t('list.bulk_delete_confirm').replace('{count}', String(ids.length)))) return
        const result = await api.applications.bulkDelete(ids)
        toast(t('list.bulk_deleted').replace('{count}', String(result.deleted)), 'info')
        toggleSelectMode(false)
        load()
      })
    } else {
      const el = appsContainer as HTMLElement
      el.classList.add('content-swap')
      const countEl = content.querySelector('#result-count')
      if (countEl) countEl.textContent = `${resp.total} ${tp('list.result_count', resp.total)}`
      const pageEl = content.querySelector('#page-indicator')
      if (pageEl) pageEl.textContent = `${resp.page} / ${resp.total_pages}`
      const prevBtn = content.querySelector('#prev-page') as HTMLButtonElement | null
      const nextBtn = content.querySelector('#next-page') as HTMLButtonElement | null
      if (prevBtn) { prevBtn.disabled = resp.page <= 1; prevBtn.className = `btn-ghost p-1.5 ${resp.page <= 1 ? 'opacity-30 pointer-events-none' : ''}` }
      if (nextBtn) { nextBtn.disabled = resp.page >= resp.total_pages; nextBtn.className = `btn-ghost p-1.5 ${resp.page >= resp.total_pages ? 'opacity-30 pointer-events-none' : ''}` }
      const paginationEl = content.querySelector('#pagination')
      if (paginationEl) (paginationEl as HTMLElement).style.display = (viewMode === 'table' && resp.total_pages > 1) ? '' : 'none'
      await new Promise(r => setTimeout(r, 120))
      el.innerHTML = viewMode === 'kanban' ? renderKanban(apps, resp.total) : renderTable(apps)
      el.classList.remove('content-swap')
      updateViewToggle()
    }

    attachCardListeners()
  }

  await load()
  return createLayout(content)
}
