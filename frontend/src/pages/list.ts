import { api } from '../api'
import { createLayout } from '../components/layout'
import { navigate } from '../router'
import { t, tp, getDateLocale } from '../i18n'
import { icons } from '../icons'
import { esc } from '../sanitize'
import { toast } from '../components/toast'
import { statusLabel, STATUS_COLORS, ALL_STATUSES, type Application, type ApplicationStatus, type PaginatedResponse } from '../types'
import { faviconUrl, domainFromUrl } from '../job-boards'

const STATUS_BORDER: Record<string, string> = {
  Wishlist: 'border-l-stone-400 dark:border-l-stone-500',
  Applied: 'border-l-sky-500 dark:border-l-sky-400',
  Screening: 'border-l-amber-500 dark:border-l-amber-400',
  Interviewing: 'border-l-orange-500 dark:border-l-orange-400',
  Offer: 'border-l-emerald-500 dark:border-l-emerald-400',
  Accepted: 'border-l-teal-500 dark:border-l-teal-400',
  Rejected: 'border-l-rose-400',
  NoReply: 'border-l-indigo-400 dark:border-l-indigo-400',
}

const STATUS_TEXT: Record<string, string> = {
  Wishlist: 'text-stone-500 dark:text-stone-400',
  Applied: 'text-sky-600 dark:text-sky-400',
  Screening: 'text-amber-600 dark:text-amber-400',
  Interviewing: 'text-orange-600 dark:text-orange-400',
  Offer: 'text-emerald-600 dark:text-emerald-400',
  Accepted: 'text-teal-600 dark:text-teal-400',
  Rejected: 'text-rose-500 dark:text-rose-400',
  NoReply: 'text-indigo-600 dark:text-indigo-400',
}

const CONF_FILL: Record<number, string> = {
  1: 'bg-stone-400 dark:bg-stone-500',
  2: 'bg-amber-500 dark:bg-amber-400',
  3: 'bg-emerald-500 dark:bg-emerald-400',
  4: 'bg-teal-500 dark:bg-teal-400',
}

/** `?period=` values the list understands; "all time" is the absence of the param. */
const LIST_PERIODS = ['30', '90', '365']

/** Dot between the details of a list card. */
const META_SEP = '<span class="text-muted" aria-hidden="true">·</span>'

/** Decorative star: the rating is spelled out in an sr-only span next to it. */
const STAR = icons.star.replace('<svg ', '<svg aria-hidden="true" ')

/** `?status=` holds one status or a comma-separated list (the dashboard's
 *  Offers tile sends `Offer,Accepted`). Unknown values are dropped: the value
 *  ends up in the kanban markup and in the API query. */
function parseStatuses(raw: string | null): ApplicationStatus[] {
  if (!raw) return []
  const wanted = raw.split(',').map(s => s.trim())
  return ALL_STATUSES.filter(s => wanted.includes(s))
}

function companyFavicon(app: Application, cls = 'w-5 h-5'): string {
  const domain = app.company_website ? domainFromUrl(app.company_website) : null
  if (!domain) return ''
  return `<img src="${esc(faviconUrl(domain, 64))}" alt="" loading="lazy" decoding="async" referrerpolicy="no-referrer" class="source-favicon rounded shrink-0 ${cls}" data-hide-on-error />`
}

function confidenceMeter(level: number): string {
  return `<span class="inline-flex gap-1 items-center" role="img" aria-label="${t('form.confidence')}: ${level}/4" title="${t('form.confidence_' + level)}">
    <span class="text-sm text-muted">${t('form.confidence')}</span>
    <span class="inline-flex gap-px items-center">${
    [1,2,3,4].map(n =>
      `<span class="w-1.5 h-3 rounded-sm ${n <= level ? (CONF_FILL[level] || 'bg-muted') : 'bg-surface-3/50'}"></span>`
    ).join('')
  }</span></span>`
}

function ratingStars(rating: number, cls: string): string {
  return `<span class="text-amber-400 ${cls} flex gap-px"><span class="sr-only">${t('form.rating')}: ${rating}/5</span>${Array.from({ length: rating }, () => STAR).join('')}</span>`
}

/** Compact, locale-aware salary ("45,5 k€" / "€45.5K"). The currency comes
 *  from the API or an import, so an unusable code falls back to euros rather
 *  than throwing inside the list template. */
function formatSalary(amount: number, currency?: string): string {
  const cur = currency && /^[A-Z]{3}$/.test(currency) ? currency : 'EUR'
  try {
    return new Intl.NumberFormat(getDateLocale(), {
      style: 'currency', currency: cur, notation: 'compact', maximumFractionDigits: 1,
    }).format(amount)
  } catch {
    return `${amount} ${cur}`
  }
}

/** `applied_at` is a calendar date stored as midnight UTC: read it back in UTC
 *  so it does not slide to the previous day west of Greenwich. */
function appliedDate(appliedAt: string): string {
  return new Date(appliedAt).toLocaleDateString(getDateLocale(), { timeZone: 'UTC' })
}

// View and sort preferences are conveniences: blocked storage (private mode,
// strict privacy settings) must not keep the list from rendering.
function readPref(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function writePref(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    // Not persisted, the choice still applies to this visit.
  }
}

export async function ListPage(): Promise<HTMLElement> {
  const urlParams = new URLSearchParams(window.location.search)
  let statusFilter = parseStatuses(urlParams.get('status'))
  let sourceFilter = urlParams.get('source') || ''
  let hasInterviewsFilter = urlParams.get('has_interviews') === '1'
  let hasReplyFilter = urlParams.get('has_reply') === '1'
  let sentFilter = urlParams.get('sent') === '1'
  const rawPeriod = urlParams.get('period') || ''
  let periodFilter = LIST_PERIODS.includes(rawPeriod) ? rawPeriod : ''
  let searchQuery = urlParams.get('q') || ''
  let sortValue = readPref('jc-sort') || 'created_at:desc'
  const rawPage = parseInt(urlParams.get('page') || '', 10)
  let currentPage = rawPage > 1 ? rawPage : 1
  let totalPages = 1
  let viewMode: 'table' | 'kanban' = readPref('jc-view') === 'kanban' ? 'kanban' : 'table'
  let debounceTimer: ReturnType<typeof setTimeout> | null = null
  const selectedIds = new Set<string>()
  let selectMode = false
  // Each load() takes a ticket; a response whose ticket is no longer the
  // latest is dropped, so overlapping filter changes cannot paint stale rows.
  let loadSeq = 0

  const content = document.createElement('div')
  content.className = 'space-y-6 stagger'

  const hasActiveFilters = () => !!(statusFilter.length || sourceFilter || hasInterviewsFilter || hasReplyFilter || sentFilter || periodFilter || searchQuery)

  /** Rewrite the querystring from the live filter state. Clearing one chip used
   *  to reset the URL to a bare `/applications`, silently dropping the other
   *  filters from the address bar even though they stayed applied. The status,
   *  search and page are kept too, so coming back from a detail page restores
   *  the same list. */
  const syncUrl = () => {
    // A pending search debounce can fire after a click opened another page:
    // never rewrite that page's URL.
    if (window.location.pathname !== '/applications') return
    const q = new URLSearchParams()
    if (statusFilter.length) q.set('status', statusFilter.join(','))
    if (sourceFilter) q.set('source', sourceFilter)
    if (hasInterviewsFilter) q.set('has_interviews', '1')
    if (hasReplyFilter) q.set('has_reply', '1')
    if (sentFilter) q.set('sent', '1')
    if (periodFilter) q.set('period', periodFilter)
    if (searchQuery) q.set('q', searchQuery)
    if (viewMode === 'table' && currentPage > 1) q.set('page', String(currentPage))
    const qs = q.toString()
    window.history.replaceState({}, '', `/applications${qs ? '?' + qs : ''}`)
  }

  function renderEmpty(): string {
    if (hasActiveFilters()) return `
      <div class="text-center py-24">
        <div class="text-muted/15 mb-6 flex justify-center">${icons.search}</div>
        <p class="text-primary text-base font-semibold mb-2">${t('list.empty_filtered')}</p>
        <p class="text-muted text-sm">${t('list.empty_filtered_hint')}</p>
      </div>
    `
    return `
      <div class="text-center py-24">
        <div class="text-muted/15 mb-6 flex justify-center">${icons.briefcaseLg}</div>
        <p class="text-primary text-base font-semibold mb-2">${t('list.empty')}</p>
        <p class="text-muted text-sm mb-8">${t('list.empty_hint')}</p>
        <a href="/applications/new" data-link class="btn-primary gap-1.5">${icons.plus} ${t('list.add_first')}</a>
      </div>
    `
  }

  /** First load failed: say so, instead of the "no applications yet" state. */
  function renderLoadError() {
    content.innerHTML = `
      <h1 class="text-2xl font-bold text-primary tracking-tight">${t('list.title')}</h1>
      <div class="text-center py-24" role="alert">
        <p class="text-primary text-base font-semibold mb-2">${t('list.load_error')}</p>
        <p class="text-muted text-sm mb-8">${t('common.load_error_hint')}</p>
        <button type="button" id="list-retry" class="btn-primary">${t('common.retry')}</button>
      </div>
    `
    content.querySelector('#list-retry')?.addEventListener('click', () => {
      // Empty the page so load() takes the first-render path and rebuilds
      // the toolbar, pagination and their listeners.
      content.innerHTML = ''
      void load()
    })
  }

  function renderTable(apps: Application[]): string {
    if (apps.length === 0) return renderEmpty()
    return `<div class="space-y-2">
      ${apps.map(app => `
        <div
          class="card card-hover flex items-center justify-between gap-4 cursor-pointer group border-l-[3px] ${STATUS_BORDER[app.status] || 'border-l-border'}"
          data-app-id="${esc(app.id)}"
        >
          <button data-select-id="${esc(app.id)}" class="shrink-0 w-6 h-6 rounded-full border-2 flex items-center justify-center transition-colors ${selectedIds.has(app.id) ? 'bg-accent border-accent text-white' : 'border-border hover:border-accent/50'} ${selectMode ? '' : 'hidden'}" aria-pressed="${selectedIds.has(app.id)}" aria-label="${t('list.select')} ${esc(app.company_name)}">${selectedIds.has(app.id) ? '<svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/></svg>' : ''}</button>
          <div class="flex-1 min-w-0">
            <a href="/applications/${esc(app.id)}" data-link class="block mb-1.5 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50">
              <span class="font-semibold text-primary block break-words sm:truncate">${companyFavicon(app, 'w-5 h-5 sm:w-6 sm:h-6 inline-block -mt-0.5 mr-1.5')}${esc(app.company_name)}</span>
              <span class="text-muted text-sm block break-words sm:truncate">${esc(app.job_title)}</span>
            </a>
            <div class="flex items-center gap-2 flex-wrap">
              ${[
                `<span class="text-sm font-semibold ${STATUS_TEXT[app.status] || 'text-muted'}">${esc(statusLabel(app.status as ApplicationStatus))}</span>`,
                app.confidence ? confidenceMeter(app.confidence) : '',
                app.location ? `<span class="text-sm text-muted flex items-center gap-1"><span aria-hidden="true" class="opacity-60">${icons.pin}</span> ${esc(app.location)}</span>` : '',
                app.salary ? `<span class="text-sm text-muted tabular-nums font-medium">${esc(formatSalary(app.salary, app.salary_currency))}</span>` : '',
                app.applied_at ? `<span class="text-sm text-muted tabular-nums">${appliedDate(app.applied_at)}</span>` : '',
              ].filter(Boolean).join(META_SEP)}
            </div>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            ${app.rating ? ratingStars(app.rating, 'text-sm') : ''}
            <button data-delete-id="${esc(app.id)}" class="btn-ghost sm:opacity-0 sm:group-hover:opacity-100 focus-visible:opacity-100 text-red-500 dark:text-red-400 hover:bg-red-500/10 p-1.5 min-w-[44px] min-h-[44px] transition-all duration-150" title="${t('detail.delete')}">
              ${icons.trash}
            </button>
          </div>
        </div>
      `).join('')}
    </div>`
  }

  function renderKanban(apps: Application[], total: number): string {
    const truncated = total > apps.length
    const columns = statusFilter.length ? statusFilter : ALL_STATUSES
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
                  <span class="badge ${STATUS_COLORS[status]} text-xs">${esc(statusLabel(status))}</span>
                  <span class="text-xs text-muted font-medium tabular-nums">${colApps.length}</span>
                </div>
                <div class="space-y-2 flex-1 min-h-24 bg-surface-2/30 rounded p-2" data-kanban-col="${esc(status)}">
                  ${colApps.length === 0 ? `
                    <div class="border border-dashed border-border/60 h-20 flex items-center justify-center">
                      <span class="text-xs text-muted">${t('list.kanban_empty')}</span>
                    </div>
                  ` : colApps.map(app => `
                    <div
                      class="card !p-3.5 cursor-pointer card-hover"
                      data-app-id="${esc(app.id)}"
                    >
                      <a href="/applications/${esc(app.id)}" data-link class="block rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50">
                        <div class="flex items-center gap-2 mb-0.5">${companyFavicon(app)}<span class="font-semibold text-primary text-sm truncate">${esc(app.company_name)}</span></div>
                        <div class="text-muted text-xs truncate mb-2.5">${esc(app.job_title)}</div>
                      </a>
                      <div class="flex items-center justify-between gap-1">
                        <span class="text-xs text-muted tabular-nums font-medium">${app.salary ? esc(formatSalary(app.salary, app.salary_currency)) : ''}</span>
                        ${app.rating ? ratingStars(app.rating, 'text-xs shrink-0') : ''}
                      </div>
                      ${app.applied_at ? `<div class="text-xs text-muted mt-1.5 tabular-nums">${appliedDate(app.applied_at)}</div>` : ''}
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
    const countText = count === 1 ? t('list.selected_one') : t('list.selected_other').replace('{count}', String(count))
    // The bar is display:none at 0, so the count is announced from a live
    // region that stays in the page.
    const live = content.querySelector('#bulk-live')
    if (live) live.textContent = count === 0 ? '' : countText
    if (count === 0) {
      bar.classList.add('hidden')
      return
    }
    bar.classList.remove('hidden')
    const countEl = bar.querySelector('#bulk-count')
    if (countEl) countEl.textContent = countText
  }

  function attachCardListeners() {
    // The title is a real link (handled by the router); a click anywhere else
    // on the card opens the application too.
    content.querySelectorAll('[data-app-id]').forEach(el => {
      el.addEventListener('click', (e) => {
        const target = e.target as HTMLElement
        if (target.closest('a, button')) return
        navigate('/applications/' + el.getAttribute('data-app-id'))
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
          // One toggle per card, so no id-based selector is needed.
          const btn = el.querySelector<HTMLButtonElement>('[data-select-id]')
          if (btn) btn.click()
          longPressTimer = null
        }, 500)
      }, { passive: true })
      el.addEventListener('touchend', () => { if (longPressTimer) clearTimeout(longPressTimer) })
      el.addEventListener('touchmove', () => { if (longPressTimer) clearTimeout(longPressTimer) })
    })

    content.querySelectorAll('[data-delete-id]').forEach(el => {
      el.addEventListener('click', async () => {
        const card = el.closest('[data-app-id]') as HTMLElement | null
        // Already on its way out: ignore a second activation.
        if (card?.classList.contains('card-exit')) return
        if (!confirm(t('list.confirm_delete'))) return
        if (card) {
          card.classList.add('card-exit')
          await new Promise(r => setTimeout(r, 200))
        }
        try {
          await api.applications.delete(el.getAttribute('data-delete-id')!)
        } catch {
          // Bring the card back: it was not deleted.
          card?.classList.remove('card-exit')
          toast(t('form.error'), 'error')
          return
        }
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

  async function load(): Promise<void> {
    const seq = ++loadSeq
    const [sortField, sortDir] = sortValue.split(':')
    let resp: PaginatedResponse<Application>
    try {
      resp = await api.applications.list({
        status: statusFilter.join(','),
        source: sourceFilter,
        has_interviews: hasInterviewsFilter,
        has_reply: hasReplyFilter,
        sent: sentFilter,
        period: periodFilter,
        search: searchQuery,
        sort: sortField,
        dir: sortDir,
        page: viewMode === 'table' ? currentPage : undefined,
        per_page: viewMode === 'kanban' ? 200 : 20,
      })
    } catch {
      if (seq !== loadSeq) return
      const shown = content.querySelector('#apps-content')
      if (shown) {
        // A failed refresh keeps the list on screen rather than looking
        // like an empty tracker.
        shown.classList.remove('content-swap')
        toast(t('list.load_error'), 'error')
      } else {
        renderLoadError()
      }
      return
    }
    if (seq !== loadSeq) return

    // Deleting the last rows of the last page leaves us past the end: step
    // back to the new last page instead of showing the empty state.
    if (resp.data.length === 0 && resp.total > 0 && currentPage > resp.total_pages) {
      currentPage = resp.total_pages
      syncUrl()
      return load()
    }

    totalPages = resp.total_pages
    const apps = resp.data

    // Bulk actions only ever apply to what is on screen: drop selected ids
    // that a filter, search or page change has hidden.
    let pruned = false
    for (const id of [...selectedIds]) {
      if (!apps.some(a => a.id === id)) { selectedIds.delete(id); pruned = true }
    }
    if (pruned) updateBulkBar()

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

      const showPagination = viewMode === 'table' && resp.total_pages > 1

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
            <span class="absolute left-3.5 top-1/2 -translate-y-1/2 text-muted pointer-events-none" aria-hidden="true">${icons.search}</span>
            <input
              type="search"
              id="search-input"
              placeholder="${t('list.search')}"
              class="input pl-10"
              value="${esc(searchQuery)}"
            />
          </div>
          <div class="flex gap-3 sm:contents">
            <div class="flex-1 sm:flex-none sm:w-48">
              <label for="status-filter" class="sr-only">${t('common.filter_status')}</label>
              <select id="status-filter" class="select">
                <option value="">${t('list.all_statuses')}</option>
                ${statusFilter.length > 1 ? `<option value="${esc(statusFilter.join(','))}" selected hidden data-multi-status>${t('list.multiple_statuses')}</option>` : ''}
                ${ALL_STATUSES.map(s => `<option value="${s}" ${statusFilter.length === 1 && statusFilter[0] === s ? 'selected' : ''}>${esc(statusLabel(s))}</option>`).join('')}
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

        ${sentFilter || sourceFilter || hasInterviewsFilter || hasReplyFilter || statusFilter.length > 1 || periodFilter ? `
        <div class="flex items-center gap-2 flex-wrap">
          ${sentFilter ? `
          <div id="sent-chip" class="flex items-center chip-enter">
            <span class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-accent/10 text-accent text-sm font-medium">
              ${t('list.sent_filter')}
              <button id="clear-sent" class="hover:bg-accent/20 rounded-full p-0.5 transition-colors" title="${t('list.clear_sent')}">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
              </button>
            </span>
          </div>
          ` : ''}
          ${sourceFilter ? `
          <div id="source-chip" class="flex items-center chip-enter">
            <span class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-accent/10 text-accent text-sm font-medium">
              ${t('list.source_filter')}: ${esc(sourceFilter)}
              <button id="clear-source" class="hover:bg-accent/20 rounded-full p-0.5 transition-colors" title="${t('list.clear_source')}">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
              </button>
            </span>
          </div>
          ` : ''}
          ${hasReplyFilter ? `
          <div id="has-reply-chip" class="flex items-center chip-enter">
            <span class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-accent/10 text-accent text-sm font-medium">
              ${t('list.has_reply_filter')}
              <button id="clear-has-reply" class="hover:bg-accent/20 rounded-full p-0.5 transition-colors" title="${t('list.clear_has_reply')}">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
              </button>
            </span>
          </div>
          ` : ''}
          ${hasInterviewsFilter ? `
          <div id="has-interviews-chip" class="flex items-center chip-enter">
            <span class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-accent/10 text-accent text-sm font-medium">
              ${t('list.has_interviews_filter')}
              <button id="clear-has-interviews" class="hover:bg-accent/20 rounded-full p-0.5 transition-colors" title="${t('list.clear_has_interviews')}">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
              </button>
            </span>
          </div>
          ` : ''}
          ${statusFilter.length > 1 ? `
          <div id="status-chip" class="flex items-center chip-enter">
            <span class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-accent/10 text-accent text-sm font-medium">
              ${t('form.status')}: ${statusFilter.map(s => esc(statusLabel(s))).join(', ')}
              <button id="clear-status" class="hover:bg-accent/20 rounded-full p-0.5 transition-colors" title="${t('list.clear_status')}">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
              </button>
            </span>
          </div>
          ` : ''}
          ${periodFilter ? `
          <div id="period-chip" class="flex items-center chip-enter">
            <span class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-accent/10 text-accent text-sm font-medium">
              ${t('list.period_filter').replace('{n}', periodFilter)}
              <button id="clear-period" class="hover:bg-accent/20 rounded-full p-0.5 transition-colors" title="${t('list.clear_period')}">
                <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
              </button>
            </span>
          </div>
          ` : ''}
        </div>
        ` : ''}

        <div class="flex items-center justify-between">
          <span id="result-count" class="text-xs text-muted tabular-nums">${resp.total} ${tp('list.result_count', resp.total)}</span>
          <div id="pagination" class="flex items-center gap-1.5"${showPagination ? '' : ' style="display:none"'}>
            <button id="prev-page" class="btn-ghost p-1.5 ${resp.page <= 1 ? 'opacity-30 pointer-events-none' : ''}" title="${t('list.page_prev')}" ${resp.page <= 1 ? 'disabled' : ''}>
              ${icons.chevronLeft}
            </button>
            <span id="page-indicator" class="text-xs text-muted tabular-nums px-1">${resp.page} / ${resp.total_pages}</span>
            <button id="next-page" class="btn-ghost p-1.5 ${resp.page >= resp.total_pages ? 'opacity-30 pointer-events-none' : ''}" title="${t('list.page_next')}" ${resp.page >= resp.total_pages ? 'disabled' : ''}>
              ${icons.chevronRight}
            </button>
          </div>
        </div>

        <div id="apps-content">
          ${viewMode === 'kanban' ? renderKanban(apps, resp.total) : renderTable(apps)}
        </div>

        <div id="bulk-bar" class="fixed bottom-0 left-0 right-0 sm:bottom-6 sm:left-1/2 sm:-translate-x-1/2 sm:right-auto z-50 hidden">
          <div class="flex items-center gap-2 sm:gap-3 px-4 py-3 sm:px-5 sm:rounded-xl border-t sm:border border-border shadow-elevated" style="background: rgb(var(--color-surface-1));">
            <span id="bulk-count" class="text-sm font-medium text-primary whitespace-nowrap shrink-0"></span>
            <div class="hidden sm:block w-px h-5 bg-border"></div>
            <select id="bulk-status-select" class="select text-sm py-2 px-2 pr-7 min-w-0 flex-1 sm:flex-none sm:w-auto" aria-label="${t('form.status')}">
              ${ALL_STATUSES.map(s => `<option value="${s}">${esc(statusLabel(s))}</option>`).join('')}
            </select>
            <button id="bulk-status-btn" class="btn-primary text-sm py-2 px-3" title="${t('list.bulk_status')}"><span class="sm:hidden">OK</span><span class="hidden sm:inline">${t('list.bulk_status')}</span></button>
            <div class="hidden sm:block w-px h-5 bg-border"></div>
            <button id="bulk-delete-btn" class="btn-ghost text-red-500 dark:text-red-400 p-2 sm:px-3 sm:py-2" title="${t('list.bulk_delete')}"><span class="sm:hidden">${icons.trash}</span><span class="hidden sm:inline">${t('list.bulk_delete')}</span></button>
          </div>
        </div>
        <div id="bulk-live" class="sr-only" role="status"></div>
      `

      content.querySelector('#view-table')?.addEventListener('click', () => {
        viewMode = 'table'
        writePref('jc-view', 'table')
        currentPage = 1
        syncUrl()
        const selBtn = content.querySelector('#select-mode-btn') as HTMLElement
        if (selBtn) selBtn.classList.remove('hidden')
        load()
      })
      content.querySelector('#view-kanban')?.addEventListener('click', () => {
        viewMode = 'kanban'
        writePref('jc-view', 'kanban')
        syncUrl()
        toggleSelectMode(false)
        const selBtn = content.querySelector('#select-mode-btn') as HTMLElement
        if (selBtn) selBtn.classList.add('hidden')
        load()
      })
      content.querySelector('#select-mode-btn')?.addEventListener('click', () => {
        toggleSelectMode(!selectMode)
      })

      const dismissChip = (selector: string) => {
        const chip = content.querySelector(selector)
        if (!chip) return
        chip.classList.replace('chip-enter', 'chip-exit')
        chip.addEventListener('animationend', () => chip.remove(), { once: true })
      }

      content.querySelector('#search-input')?.addEventListener('input', (e) => {
        searchQuery = (e.target as HTMLInputElement).value
        currentPage = 1
        if (debounceTimer) clearTimeout(debounceTimer)
        debounceTimer = setTimeout(() => { syncUrl(); load() }, 200)
      })
      content.querySelector('#status-filter')?.addEventListener('change', (e) => {
        statusFilter = parseStatuses((e.target as HTMLSelectElement).value)
        // Picking a single status replaces a multi-status filter.
        content.querySelector('#status-filter [data-multi-status]')?.remove()
        dismissChip('#status-chip')
        currentPage = 1
        syncUrl()
        load()
      })
      content.querySelector('#sort-select')?.addEventListener('change', (e) => {
        sortValue = (e.target as HTMLSelectElement).value
        writePref('jc-sort', sortValue)
        currentPage = 1
        syncUrl()
        load()
      })
      content.querySelector('#prev-page')?.addEventListener('click', () => {
        if (currentPage > 1) { currentPage--; syncUrl(); load() }
      })
      content.querySelector('#next-page')?.addEventListener('click', () => {
        if (currentPage < totalPages) { currentPage++; syncUrl(); load() }
      })
      content.querySelector('#clear-source')?.addEventListener('click', () => {
        sourceFilter = ''
        currentPage = 1
        syncUrl()
        dismissChip('#source-chip')
        load()
      })
      content.querySelector('#clear-has-reply')?.addEventListener('click', () => {
        hasReplyFilter = false
        currentPage = 1
        syncUrl()
        dismissChip('#has-reply-chip')
        load()
      })
      content.querySelector('#clear-has-interviews')?.addEventListener('click', () => {
        hasInterviewsFilter = false
        currentPage = 1
        syncUrl()
        dismissChip('#has-interviews-chip')
        load()
      })
      content.querySelector('#clear-sent')?.addEventListener('click', () => {
        sentFilter = false
        currentPage = 1
        syncUrl()
        dismissChip('#sent-chip')
        load()
      })
      content.querySelector('#clear-period')?.addEventListener('click', () => {
        periodFilter = ''
        currentPage = 1
        syncUrl()
        dismissChip('#period-chip')
        load()
      })
      content.querySelector('#clear-status')?.addEventListener('click', () => {
        statusFilter = []
        content.querySelector('#status-filter [data-multi-status]')?.remove()
        const select = content.querySelector('#status-filter') as HTMLSelectElement | null
        if (select) select.value = ''
        currentPage = 1
        syncUrl()
        dismissChip('#status-chip')
        load()
      })

      // Bulk actions
      content.querySelector('#bulk-status-btn')?.addEventListener('click', async (e) => {
        const btn = e.currentTarget as HTMLButtonElement
        const status = (content.querySelector('#bulk-status-select') as HTMLSelectElement).value
        const ids = [...selectedIds]
        btn.disabled = true
        try {
          const result = await api.applications.bulkStatus(ids, status)
          toast(t('list.bulk_done').replace('{count}', String(result.updated)), 'success')
          toggleSelectMode(false)
          load()
        } catch {
          // Keep the selection so the user can try again.
          toast(t('form.error'), 'error')
        } finally {
          btn.disabled = false
        }
      })
      content.querySelector('#bulk-delete-btn')?.addEventListener('click', async (e) => {
        const btn = e.currentTarget as HTMLButtonElement
        const ids = [...selectedIds]
        if (!confirm(t('list.bulk_delete_confirm').replace('{count}', String(ids.length)))) return
        btn.disabled = true
        try {
          const result = await api.applications.bulkDelete(ids)
          toast(t('list.bulk_deleted').replace('{count}', String(result.deleted)), 'info')
          toggleSelectMode(false)
          load()
        } catch {
          toast(t('form.error'), 'error')
        } finally {
          btn.disabled = false
        }
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
      // A newer load started during the fade: it owns the list now.
      if (seq !== loadSeq) return
      el.innerHTML = viewMode === 'kanban' ? renderKanban(apps, resp.total) : renderTable(apps)
      el.classList.remove('content-swap')
      updateViewToggle()
    }

    attachCardListeners()
  }

  await load()
  return createLayout(content)
}
