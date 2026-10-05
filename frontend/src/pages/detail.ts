import { api } from '../api'
import { createLayout } from '../components/layout'
import { openModal } from '../components/modal'
import { navigate, setNavigationGuard, setNavigationCleanup } from '../router'
import { toast, celebrate } from '../components/toast'
import { t, getDateLocale, translateTimelineEvent } from '../i18n'
import { icons } from '../icons'
import { esc, sanitizeUrl, safeHostname } from '../sanitize'
import { faviconUrl, getSourceDomain, domainFromUrl } from '../job-boards'
import {
  statusLabel,
  STATUS_COLORS,
  ALL_STATUSES,
  interviewTypeLabel,
  interviewOutcomeLabel,
  contractLabel,
  workModeLabel,
  type Application,
  type ApplicationStatus,
  type Interview,
  type Contact,
} from '../types'

const OUTCOME_COLORS: Record<string, string> = {
  Passed: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  Failed: 'bg-rose-50 text-rose-600 dark:bg-rose-900/40 dark:text-rose-300',
  Pending: 'bg-stone-100 text-stone-600 dark:bg-stone-800/60 dark:text-stone-300',
  Cancelled: 'bg-stone-100 text-stone-500 dark:bg-stone-800/40 dark:text-stone-400',
  Rejected: 'bg-rose-50 text-rose-600 dark:bg-rose-900/40 dark:text-rose-300',
}

// Literal class names: Tailwind only generates classes it can find verbatim in the source.
const LG_COLS: Record<number, string> = {
  1: 'lg:grid-cols-1',
  2: 'lg:grid-cols-2',
  3: 'lg:grid-cols-3',
  4: 'lg:grid-cols-4',
  5: 'lg:grid-cols-5',
  6: 'lg:grid-cols-6',
  7: 'lg:grid-cols-7',
}

/** Today's LOCAL calendar date in the `YYYY-MM-DDT00:00:00Z` form used for applied_at. */
function localCalendarDate(d = new Date()): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}T00:00:00Z`
}

/** A mailto: URL, or '' when the value does not look like a single address (the caller then
 *  shows it as plain text). Both halves are percent-encoded so `?`, `&` or `#` cannot add
 *  headers (cc, bcc, body) to the draft. */
function mailtoHref(email: string): string {
  const parts = email.split('@')
  if (parts.length !== 2 || !parts[0] || !parts[1] || /\s/.test(email)) return ''
  return `mailto:${encodeURIComponent(parts[0])}@${encodeURIComponent(parts[1])}`
}

/** A tel: URL from the digits of a phone number, keeping a leading +, or '' when it has
 *  no digits (the caller then shows it as plain text). */
function telHref(phone: string): string {
  const digits = phone.replace(/\D/g, '')
  if (!digits) return ''
  return `tel:${phone.trim().startsWith('+') ? '+' : ''}${digits}`
}

/** Salary as shown on the detail page: 45000 -> "45k €", 550 -> "550 €", 60000 GBP -> "60k GBP". */
function formatSalary(amount: number, currency: string | undefined, locale: string): string {
  const n = amount >= 1000
    ? `${(amount / 1000).toLocaleString(locale, { maximumFractionDigits: 1 })}k`
    : amount.toLocaleString(locale)
  return `${n} ${!currency || currency === 'EUR' ? '\u20ac' : currency}`
}

function buildInterviewForm(iv?: Partial<Interview>): {
  el: HTMLElement
  getData: () => Partial<Interview>
} {
  const el = document.createElement('div')
  el.className = 'space-y-4'
  el.innerHTML = `
    <div class="grid grid-cols-2 gap-3">
      <div>
        <label for="iv-round" class="label">${t('detail.interview_round')}</label>
        <input id="iv-round" name="round" class="input" type="number" min="1" value="${iv?.round ?? 1}" />
      </div>
      <div>
        <label for="iv-type" class="label">${t('detail.interview_type')}</label>
        <select id="iv-type" name="type" class="select">
          ${['Screening', 'Phone', 'Video', 'On-site', 'Technical', 'HR', 'Culture', 'Final'].map(type =>
            `<option value="${type}" ${iv?.type === type ? 'selected' : ''}>${esc(interviewTypeLabel(type))}</option>`
          ).join('')}
        </select>
      </div>
    </div>
    <div role="group" aria-labelledby="iv-scheduled-label">
      <span id="iv-scheduled-label" class="label">${t('detail.interview_scheduled')}</span>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <input id="iv-scheduled-date" name="scheduled_date" class="input" type="date" aria-label="${t('detail.interview_date')}"
          value="${iv?.scheduled_at ? new Date(iv.scheduled_at).toISOString().slice(0, 10) : ''}" />
        <input id="iv-scheduled-time" name="scheduled_time" class="input" type="time" aria-label="${t('detail.interview_time')}"
          value="${iv?.scheduled_at ? new Date(iv.scheduled_at).toISOString().slice(11, 16) : ''}" />
      </div>
    </div>
    <div class="grid grid-cols-2 gap-3">
      <div>
        <label for="iv-duration" class="label">${t('detail.interview_duration')}</label>
        <input id="iv-duration" name="duration_minutes" class="input" type="number" min="0" step="15"
          value="${iv?.duration_minutes ?? ''}" />
      </div>
      <div>
        <label for="iv-outcome" class="label">${t('detail.interview_outcome')}</label>
        <select id="iv-outcome" name="outcome" class="select">
          <option value="">${t('detail.interview_outcome_none')}</option>
          ${['Passed', 'Failed', 'Pending', 'Cancelled', 'Rejected'].map(o =>
            `<option value="${o}" ${iv?.outcome === o ? 'selected' : ''}>${esc(interviewOutcomeLabel(o))}</option>`
          ).join('')}
        </select>
      </div>
    </div>
    <div class="grid grid-cols-2 gap-3">
      <div>
        <label for="iv-interviewer" class="label">${t('detail.interview_interviewer')}</label>
        <input id="iv-interviewer" name="interviewer_name" class="input" value="${esc(iv?.interviewer_name)}" />
      </div>
      <div>
        <label for="iv-role" class="label">${t('detail.interview_role')}</label>
        <input id="iv-role" name="interviewer_role" class="input" value="${esc(iv?.interviewer_role)}" />
      </div>
    </div>
    <div>
      <label for="iv-notes" class="label">${t('detail.interview_notes')}</label>
      <textarea id="iv-notes" name="notes" class="input h-32 resize-y">${esc(iv?.notes)}</textarea>
    </div>
    <div class="flex justify-end pt-2">
      <button type="button" data-save class="btn-primary">${t('detail.save')}</button>
    </div>
  `
  const getData = (): Partial<Interview> => ({
    round: Number((el.querySelector<HTMLInputElement>('[name="round"]'))?.value) || 1,
    type: (el.querySelector<HTMLSelectElement>('[name="type"]'))?.value as Interview['type'],
    scheduled_at: (() => {
      const d = (el.querySelector<HTMLInputElement>('[name="scheduled_date"]'))?.value
      if (!d) return undefined
      const t = (el.querySelector<HTMLInputElement>('[name="scheduled_time"]'))?.value
      return d + 'T' + (t || '00:00') + ':00Z'
    })(),
    duration_minutes: (el.querySelector<HTMLInputElement>('[name="duration_minutes"]'))?.value
      ? Number((el.querySelector<HTMLInputElement>('[name="duration_minutes"]'))!.value)
      : undefined,
    outcome: ((el.querySelector<HTMLSelectElement>('[name="outcome"]'))?.value || undefined) as Interview['outcome'] | undefined,
    interviewer_name: (el.querySelector<HTMLInputElement>('[name="interviewer_name"]'))?.value || undefined,
    interviewer_role: (el.querySelector<HTMLInputElement>('[name="interviewer_role"]'))?.value || undefined,
    notes: (el.querySelector<HTMLTextAreaElement>('[name="notes"]'))?.value || undefined,
    // Not editable here, but the PUT is a full replace: carry it through so an edit keeps it.
    prep_notes: iv?.prep_notes,
  })
  return { el, getData }
}

function buildContactForm(c?: Partial<Contact>): {
  el: HTMLElement
  getData: () => Partial<Contact> | null
} {
  const el = document.createElement('div')
  el.className = 'space-y-4'
  el.innerHTML = `
    <div>
      <label for="ct-name" class="label">${t('detail.contact_name')} *</label>
      <input id="ct-name" name="name" class="input" required value="${esc(c?.name)}" />
    </div>
    <div class="grid grid-cols-2 gap-3">
      <div>
        <label for="ct-role" class="label">${t('detail.contact_role')}</label>
        <input id="ct-role" name="role" class="input" value="${esc(c?.role)}" />
      </div>
      <div>
        <label for="ct-email" class="label">${t('detail.contact_email')}</label>
        <input id="ct-email" name="email" class="input" type="email" value="${esc(c?.email)}" />
      </div>
    </div>
    <div class="grid grid-cols-2 gap-3">
      <div>
        <label for="ct-phone" class="label">${t('detail.contact_phone')}</label>
        <input id="ct-phone" name="phone" class="input" value="${esc(c?.phone)}" />
      </div>
      <div>
        <label for="ct-linkedin" class="label">${t('detail.contact_linkedin')}</label>
        <input id="ct-linkedin" name="linkedin" class="input" value="${esc(c?.linkedin)}" />
      </div>
    </div>
    <div>
      <label for="ct-notes" class="label">${t('detail.contact_notes')}</label>
      <textarea id="ct-notes" name="notes" class="input h-32 resize-y">${esc(c?.notes)}</textarea>
    </div>
    <div class="flex justify-end pt-2">
      <button type="button" data-save class="btn-primary">${t('detail.save')}</button>
    </div>
  `
  const getData = (): Partial<Contact> | null => {
    const name = (el.querySelector<HTMLInputElement>('[name="name"]'))?.value?.trim()
    if (!name) return null
    return {
      name,
      role: (el.querySelector<HTMLInputElement>('[name="role"]'))?.value || undefined,
      email: (el.querySelector<HTMLInputElement>('[name="email"]'))?.value || undefined,
      phone: (el.querySelector<HTMLInputElement>('[name="phone"]'))?.value || undefined,
      linkedin: (el.querySelector<HTMLInputElement>('[name="linkedin"]'))?.value || undefined,
      notes: (el.querySelector<HTMLTextAreaElement>('[name="notes"]'))?.value || undefined,
    }
  }
  return { el, getData }
}

function makeTabs(tabs: Array<{ id: string; label: string; panel: HTMLElement }>): {
  el: HTMLElement
  setLabel: (id: string, label: string) => void
} {
  const wrapper = document.createElement('div')
  wrapper.className = 'space-y-0'

  const bar = document.createElement('div')
  bar.className = 'relative flex gap-0 border-b border-border overflow-x-auto no-scrollbar'
  bar.setAttribute('role', 'tablist')

  const indicator = document.createElement('div')
  indicator.className = 'tab-indicator'
  bar.appendChild(indicator)

  const panels: HTMLElement[] = []

  const tabClass = (active: boolean) =>
    `px-5 py-3.5 text-sm font-medium border-b-2 border-transparent transition-colors duration-150 whitespace-nowrap focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:ring-inset ${
      active
        ? 'text-accent'
        : 'text-muted hover:text-primary'
    }`

  function moveIndicator(btn: HTMLElement) {
    indicator.style.left = `${btn.offsetLeft}px`
    indicator.style.width = `${btn.offsetWidth}px`
  }

  tabs.forEach((tab, i) => {
    const btn = document.createElement('button')
    btn.className = tabClass(i === 0)
    btn.setAttribute('role', 'tab')
    btn.id = `tab-${tab.id}`
    btn.setAttribute('aria-selected', i === 0 ? 'true' : 'false')
    btn.setAttribute('aria-controls', `tab-panel-${tab.id}`)
    btn.setAttribute('tabindex', i === 0 ? '0' : '-1')
    btn.dataset.tab = tab.id
    btn.textContent = tab.label

    bar.appendChild(btn)

    tab.panel.id = `tab-panel-${tab.id}`
    tab.panel.setAttribute('role', 'tabpanel')
    tab.panel.setAttribute('aria-labelledby', `tab-${tab.id}`)
    if (i !== 0) tab.panel.hidden = true
    panels.push(tab.panel)
  })

  // Position indicator on first tab after layout
  requestAnimationFrame(() => {
    const first = bar.querySelector<HTMLElement>('[aria-selected="true"]')
    if (first) moveIndicator(first)
  })

  function activateTab(activeId: string) {
    bar.querySelectorAll<HTMLElement>('[data-tab]').forEach(b => {
      const isActive = b.dataset.tab === activeId
      b.setAttribute('aria-selected', isActive ? 'true' : 'false')
      b.setAttribute('tabindex', isActive ? '0' : '-1')
      b.className = tabClass(isActive)
      if (isActive) moveIndicator(b)
    })
    panels.forEach(p => {
      p.hidden = p.id !== `tab-panel-${activeId}`
    })
  }

  bar.addEventListener('click', (e) => {
    const btn = (e.target as HTMLElement).closest('[data-tab]') as HTMLElement | null
    if (!btn) return
    activateTab(btn.dataset.tab!)
  })

  bar.addEventListener('keydown', (e) => {
    const tabBtns = Array.from(bar.querySelectorAll<HTMLElement>('[data-tab]'))
    const current = tabBtns.findIndex(b => b.getAttribute('aria-selected') === 'true')
    let next = -1

    switch (e.key) {
      case 'ArrowRight': next = (current + 1) % tabBtns.length; break
      case 'ArrowLeft': next = (current - 1 + tabBtns.length) % tabBtns.length; break
      case 'Home': next = 0; break
      case 'End': next = tabBtns.length - 1; break
      default: return
    }

    e.preventDefault()
    activateTab(tabBtns[next].dataset.tab!)
    tabBtns[next].focus()
  })

  // Relabel a tab (e.g. a changed count) and keep the indicator on the active tab,
  // whose position or width may have moved with the new text.
  function setLabel(id: string, label: string) {
    const btn = bar.querySelector<HTMLElement>(`[data-tab="${id}"]`)
    if (!btn) return
    btn.textContent = label
    const active = bar.querySelector<HTMLElement>('[aria-selected="true"]')
    if (active) moveIndicator(active)
  }

  wrapper.appendChild(bar)
  panels.forEach(p => wrapper.appendChild(p))
  return { el: wrapper, setLabel }
}

export async function DetailPage(id: string): Promise<HTMLElement> {
  const app = await api.applications.get(id).catch(() => null)

  if (!app) {
    const err = document.createElement('div')
    err.className = 'flex flex-col items-center justify-center h-64 text-muted gap-2'
    err.innerHTML = `
      <div class="text-muted/20">${icons.briefcaseLg}</div>
      <p>${t('detail.not_found')}</p>
      <a href="/applications" data-link class="btn-ghost text-sm mt-2">${icons.arrowLeft} ${t('nav.applications')}</a>
    `
    return createLayout(err)
  }

  const dateFmt = getDateLocale()

  // After each PUT, copy back the fields the server may set itself (applied_at on the move
  // to Applied), so a later save from this page never sends a stale applied_at. A status
  // picked while this request was in flight is kept: its own save is queued behind it.
  const syncApp = (updated: Application, sent: Partial<Application>) => {
    app.applied_at = updated.applied_at ?? undefined
    if (app.status === sent.status) app.status = updated.status
    app.updated_at = updated.updated_at
  }

  // The in-place saves (status, interest, confidence, the three text tabs) each PUT the
  // whole application. They go out one at a time and each payload is built from `app`
  // when its turn comes, so a quick second change never sends a value the first one is
  // still changing. A caller's own follow-up (rollback, re-render) runs before the next
  // payload is built.
  let saveQueue: Promise<unknown> = Promise.resolve()
  const saveApp = (patch: Partial<Application>): Promise<Application> => {
    const run = saveQueue.then(async () => {
      const payload = { ...app, ...patch }
      const updated = await api.applications.update(id, payload)
      syncApp(updated, payload)
      return updated
    })
    saveQueue = run.catch(() => {})
    return run
  }

  // Document listeners added below, removed on the next navigation so visits do not
  // pile them up.
  const docListeners = new AbortController()

  const content = document.createElement('div')
  content.className = 'space-y-10 stagger'

  const header = document.createElement('div')
  header.className = 'space-y-5 relative z-10'

  // Status dropdown
  const statusWrapper = document.createElement('div')
  statusWrapper.className = 'relative'

  const statusBtn = document.createElement('button')
  statusBtn.className = `badge ${STATUS_COLORS[app.status as ApplicationStatus]} cursor-pointer hover:opacity-80 transition-opacity duration-100`
  statusBtn.textContent = statusLabel(app.status as ApplicationStatus)
  statusBtn.setAttribute('aria-haspopup', 'true')
  statusBtn.setAttribute('aria-expanded', 'false')

  const statusDotColors: Record<string, string> = {
    Wishlist: 'bg-stone-400',
    Applied: 'bg-sky-500',
    Screening: 'bg-amber-500',
    Interviewing: 'bg-orange-500',
    Offer: 'bg-emerald-500',
    Accepted: 'bg-teal-500',
    Rejected: 'bg-rose-400',
    NoReply: 'bg-indigo-400',
  }

  const checkSvg = '<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2.5" stroke="currentColor" class="w-3.5 h-3.5 text-accent shrink-0"><path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5"/></svg>'

  const statusDropdown = document.createElement('div')
  statusDropdown.className = 'hidden absolute top-full left-0 mt-2 z-40 bg-surface-1 border border-border rounded py-1 min-w-[200px]'
  statusDropdown.style.boxShadow = 'var(--shadow-elevated)'
  statusDropdown.setAttribute('role', 'menu')

  function openDropdown() {
    statusDropdown.classList.remove('hidden', 'dropdown-exit')
    statusDropdown.classList.add('dropdown-enter')
    statusBtn.setAttribute('aria-expanded', 'true')
  }
  function closeDropdown() {
    if (statusDropdown.classList.contains('hidden')) return
    statusDropdown.classList.remove('dropdown-enter')
    statusDropdown.classList.add('dropdown-exit')
    statusBtn.setAttribute('aria-expanded', 'false')
    statusDropdown.addEventListener('animationend', () => {
      if (statusDropdown.classList.contains('dropdown-exit')) {
        statusDropdown.classList.add('hidden')
        statusDropdown.classList.remove('dropdown-exit')
      }
    }, { once: true })
  }

  // The badge and the menu's check mark follow app.status.
  function paintStatus() {
    statusBtn.className = `badge ${STATUS_COLORS[app!.status as ApplicationStatus]} cursor-pointer hover:opacity-80 transition-opacity duration-100`
    statusBtn.textContent = statusLabel(app!.status as ApplicationStatus)
    renderStatusItems()
  }

  function renderStatusItems() {
    statusDropdown.innerHTML = ''
    ALL_STATUSES.forEach(s => {
      const isCurrent = s === app!.status
      const item = document.createElement('button')
      item.className = `w-full text-left px-3 py-2 text-sm hover:bg-surface-2 focus:bg-surface-2 focus:outline-none transition-colors flex items-center gap-2.5 ${isCurrent ? 'font-semibold text-accent' : 'text-primary'}`
      item.setAttribute('role', 'menuitem')
      item.innerHTML = `
        <span class="w-2 h-2 rounded-full ${statusDotColors[s] || 'bg-accent'} shrink-0"></span>
        <span class="flex-1">${statusLabel(s)}</span>
        ${isCurrent ? checkSvg : '<span class="w-3.5"></span>'}
      `
      item.addEventListener('click', async () => {
        closeDropdown()
        statusBtn.focus()
        const previous = app!.status
        const patch: Partial<Application> = { status: s }
        // applied_at is a calendar date: on the move to Applied, send the user's local
        // day instead of letting the server stamp the current UTC instant.
        if (s === 'Applied' && previous !== 'Applied' && !app!.applied_at) patch.applied_at = localCalendarDate()
        app!.status = s
        paintStatus()
        try {
          await saveApp(patch)
        } catch {
          // A newer pick owns the badge now: leave it.
          if (app!.status === s) {
            app!.status = previous
            paintStatus()
          }
          toast(t('form.error'), 'error')
          return
        }
        // renderDetails detaches and reattaches the status cell, which drops its focus.
        const hadFocus = document.activeElement === statusBtn
        renderDetails()
        if (hadFocus) statusBtn.focus()
        void refreshTimeline()
        const celebrateMsg: Record<string, string> = { Offer: t('list.celebrate_offer'), Accepted: t('list.celebrate_accepted') }
        toast(celebrateMsg[s] || statusLabel(s), 'success')
        if (s === 'Offer' || s === 'Accepted') celebrate(statusBtn)
      })
      statusDropdown.appendChild(item)
    })
  }
  renderStatusItems()

  statusBtn.addEventListener('click', (e) => {
    e.stopPropagation()
    const isOpen = !statusDropdown.classList.contains('hidden')
    if (isOpen) {
      closeDropdown()
    } else {
      openDropdown()
      const current = statusDropdown.querySelector<HTMLElement>('.font-semibold') || statusDropdown.querySelector<HTMLElement>('[role="menuitem"]')
      current?.focus()
    }
  })
  statusDropdown.addEventListener('keydown', (e) => {
    const items = Array.from(statusDropdown.querySelectorAll<HTMLElement>('[role="menuitem"]'))
    const current = items.indexOf(document.activeElement as HTMLElement)
    let next = -1
    switch (e.key) {
      case 'ArrowDown': next = current < items.length - 1 ? current + 1 : 0; break
      case 'ArrowUp': next = current > 0 ? current - 1 : items.length - 1; break
      case 'Home': next = 0; break
      case 'End': next = items.length - 1; break
      case 'Escape':
        closeDropdown()
        statusBtn.focus()
        e.stopPropagation()
        return
      default: return
    }
    e.preventDefault()
    items[next]?.focus()
  })
  document.addEventListener('click', () => closeDropdown(), { capture: true, signal: docListeners.signal })

  statusWrapper.appendChild(statusBtn)
  statusWrapper.appendChild(statusDropdown)

  // Interest: five stars, editable in place like the status and the confidence.
  // Clicking the current level clears it. The negative margins keep the first star under
  // the label and the row as tall as before, with 24px hit areas.
  const ratingEl = document.createElement('div')
  ratingEl.className = 'flex items-center -ml-1 -my-0.5'
  ratingEl.setAttribute('role', 'group')
  ratingEl.setAttribute('aria-label', t('form.rating'))
  const renderRating = (preview = 0) => {
    const level = preview || app.rating || 0
    ratingEl.querySelectorAll<HTMLElement>('[data-star]').forEach(star => {
      const n = Number(star.dataset.star)
      star.classList.toggle('text-amber-400', n <= level)
      star.classList.toggle('text-surface-3', n > level)
      star.setAttribute('aria-pressed', String(n === app.rating))
    })
  }
  for (let n = 1; n <= 5; n++) {
    const star = document.createElement('button')
    star.type = 'button'
    star.dataset.star = String(n)
    star.className = 'p-1 rounded transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50'
    star.setAttribute('aria-label', `${n}/5 · ${t('form.rating_' + n)}`)
    star.title = t('form.rating_' + n)
    star.innerHTML = icons.star
    star.addEventListener('mouseenter', () => renderRating(n))
    star.addEventListener('click', async () => {
      const previous = app.rating
      const next = n === app.rating ? undefined : n
      app.rating = next
      renderRating()
      try {
        await saveApp({ rating: next })
      } catch {
        // A newer click owns the stars now: leave them.
        if (app.rating === next) {
          app.rating = previous
          renderRating()
        }
        toast(t('form.error'), 'error')
      }
    })
    ratingEl.appendChild(star)
  }
  ratingEl.addEventListener('mouseleave', () => renderRating())
  renderRating()

  // — Top bar: back + actions
  const topBar = document.createElement('div')
  topBar.className = 'flex items-center justify-between'
  topBar.innerHTML = `
    <a href="/applications" data-link class="text-muted hover:text-primary transition-colors text-sm flex items-center gap-1.5" aria-label="${t('form.back')}">${icons.arrowLeft} ${t('nav.applications')}</a>
    <div class="flex items-center gap-1 shrink-0">
      <a href="/applications/${esc(encodeURIComponent(app.id))}/edit" data-link class="text-muted hover:text-primary transition-colors text-sm px-2.5 py-1.5 inline-flex items-center gap-1.5 whitespace-nowrap"><span aria-hidden="true">${icons.edit}</span>${t('detail.edit')}</a>
      <button id="delete-btn" class="text-muted hover:text-red-500 dark:hover:text-red-400 transition-colors text-sm px-2.5 py-1.5 inline-flex items-center gap-1.5 whitespace-nowrap"><span aria-hidden="true">${icons.trash}</span>${t('detail.delete')}</button>
    </div>
  `
  header.appendChild(topBar)

  // — Title block
  const titleGroup = document.createElement('div')
  titleGroup.innerHTML = `
    <h1 class="text-2xl font-bold text-primary tracking-tight">${
      app.company_website && domainFromUrl(app.company_website)
        ? `<img src="${esc(faviconUrl(domainFromUrl(app.company_website)!, 64))}" alt="" class="inline-block w-7 h-7 rounded -mt-1 mr-2" loading="lazy" data-hide-on-error />`
        : ''
    }${esc(app.company_name)}</h1>
    <p class="text-base text-muted mt-1">${esc(app.job_title)}</p>
    ${app.location ? `<p class="text-muted text-sm flex items-center gap-1.5 mt-2"><span aria-hidden="true">${icons.pin}</span> ${esc(app.location)}</p>` : ''}
  `
  header.appendChild(titleGroup)

  // — Status / Rating / Confidence — clean separated row
  const confidenceColors: Record<number, string> = {
    1: 'text-stone-500 dark:text-stone-400',
    2: 'text-amber-600 dark:text-amber-400',
    3: 'text-emerald-600 dark:text-emerald-400',
    4: 'text-teal-600 dark:text-teal-400',
  }

  // Status, interest and confidence open the properties grid below, in the same
  // label-above-value form as the other fields.
  const propCell = (label: string, control: HTMLElement): HTMLElement => {
    const cell = document.createElement('div')
    cell.className = 'min-w-0'
    const labelEl = document.createElement('span')
    labelEl.className = 'block text-sm text-muted mb-1'
    labelEl.textContent = label
    cell.append(labelEl, control)
    return cell
  }
  const propCells: HTMLElement[] = [propCell(t('form.status'), statusWrapper)]
  propCells.push(propCell(t('form.rating'), ratingEl))
  // Confidence picker (always show, even if not set)
  {
    const confWrapper = document.createElement('div')
    confWrapper.className = 'relative'

    const confBtn = document.createElement('button')
    const confLevel = () => (app.confidence && app.confidence >= 1 && app.confidence <= 4) ? app.confidence : 0
    const updateConfBtn = (level: number) => {
      if (level > 0) {
        confBtn.className = `text-sm font-medium cursor-pointer hover:opacity-80 transition-opacity ${confidenceColors[level] || 'text-muted'}`
        confBtn.textContent = t('form.confidence_' + level)
        confBtn.setAttribute('aria-label', t('detail.confidence_label').replace('{level}', t('form.confidence_' + level)))
      } else {
        confBtn.className = 'text-sm text-muted cursor-pointer hover:text-primary transition-colors'
        confBtn.textContent = t('detail.confidence_none')
        confBtn.setAttribute('aria-label', t('detail.confidence_unset'))
      }
    }
    updateConfBtn(confLevel())
    confBtn.setAttribute('aria-haspopup', 'true')
    confBtn.setAttribute('aria-expanded', 'false')
    confWrapper.appendChild(confBtn)

    const confDrop = document.createElement('div')
    confDrop.className = 'hidden absolute top-full left-0 mt-2 z-40 bg-surface-1 border border-border rounded py-1 min-w-[160px]'
    confDrop.style.boxShadow = 'var(--shadow-elevated)'
    confDrop.setAttribute('role', 'menu')

    const openConf = () => {
      confDrop.classList.remove('hidden')
      confBtn.setAttribute('aria-expanded', 'true')
      const current = confDrop.querySelector<HTMLElement>('[aria-checked="true"]') || confDrop.querySelector<HTMLElement>('[role="menuitemradio"]')
      current?.focus()
    }
    const closeConf = () => {
      confDrop.classList.add('hidden')
      confBtn.setAttribute('aria-expanded', 'false')
    }

    const confLevels = [1, 2, 3, 4] as const
    function renderConfItems() {
      confDrop.innerHTML = ''
      confLevels.forEach(n => {
        const isCurrent = n === app!.confidence
        const item = document.createElement('button')
        item.className = `w-full text-left px-3 py-2 text-sm hover:bg-surface-2 focus:bg-surface-2 focus:outline-none transition-colors ${isCurrent ? 'font-semibold ' + (confidenceColors[n] || '') : 'text-primary'}`
        item.setAttribute('role', 'menuitemradio')
        item.setAttribute('aria-checked', String(isCurrent))
        item.textContent = t('form.confidence_' + n)
        item.addEventListener('click', async () => {
          closeConf()
          confBtn.focus()
          const previous = app!.confidence
          const next = isCurrent ? undefined : n
          app!.confidence = next
          updateConfBtn(confLevel())
          renderConfItems()
          try {
            await saveApp({ confidence: next })
          } catch {
            // A newer pick owns the button now: leave it.
            if (app!.confidence === next) {
              app!.confidence = previous
              updateConfBtn(confLevel())
              renderConfItems()
            }
            toast(t('form.error'), 'error')
          }
        })
        confDrop.appendChild(item)
      })
    }
    renderConfItems()

    confBtn.addEventListener('click', (e) => {
      e.stopPropagation()
      if (confDrop.classList.contains('hidden')) openConf()
      else closeConf()
    })
    confDrop.addEventListener('keydown', (e) => {
      const items = Array.from(confDrop.querySelectorAll<HTMLElement>('[role="menuitemradio"]'))
      const current = items.indexOf(document.activeElement as HTMLElement)
      let next = -1
      switch (e.key) {
        case 'ArrowDown': next = current < items.length - 1 ? current + 1 : 0; break
        case 'ArrowUp': next = current > 0 ? current - 1 : items.length - 1; break
        case 'Home': next = 0; break
        case 'End': next = items.length - 1; break
        case 'Escape':
          closeConf()
          confBtn.focus()
          e.stopPropagation()
          return
        default: return
      }
      e.preventDefault()
      items[next]?.focus()
    })
    // Safari and Firefox do not focus a button on click: focus would jump to the nearest
    // focusable ancestor (<main tabindex="-1">), the focusout below would close the menu,
    // and the item's click would never land. Keeping focus where it is avoids that.
    confDrop.addEventListener('pointerdown', (e) => e.preventDefault())
    // Tabbing out closes the menu. Clicks outside are left to the document listener below.
    confWrapper.addEventListener('focusout', (e) => {
      const next = e.relatedTarget as Node | null
      if (next && !confWrapper.contains(next) && !next.contains(confWrapper)) closeConf()
    })
    // Capture phase runs before confBtn's own handler, so ignore clicks inside the picker
    // or the button could never close the menu it opened.
    document.addEventListener('click', (e) => {
      if (!confWrapper.contains(e.target as Node)) closeConf()
    }, { capture: true, signal: docListeners.signal })
    confWrapper.appendChild(confDrop)
    propCells.push(propCell(t('form.confidence'), confWrapper))
  }
  content.appendChild(header)

  // Metadata: clean typographic row, no boxes. Rebuilt after a status change, which can
  // set applied_at.
  const detailsRow = document.createElement('div')
  const renderDetails = () => {
    const details: Array<{ label: string; value: string; href?: string; iconHtml?: string }> = []
    if (app.contract_type) {
      let contractValue = contractLabel(app.contract_type as string)
      if (app.contract_type === 'CDD' && app.contract_duration) {
        contractValue += ` (${app.contract_duration} ${t('detail.months')})`
      }
      details.push({ label: t('detail.contract'), value: contractValue })
    }
    if (app.work_mode) details.push({ label: t('detail.mode'), value: workModeLabel(app.work_mode) })
    if (app.salary) details.push({ label: t('detail.salary'), value: formatSalary(app.salary, app.salary_currency, dateFmt) })
    // applied_at is a calendar date stored as UTC midnight: format it in UTC so it shows the
    // same day everywhere. created_at is a real instant and stays in local time.
    if (app.applied_at) details.push({ label: t('detail.applied_at'), value: new Date(app.applied_at).toLocaleDateString(dateFmt, { timeZone: 'UTC' }) })
    details.push({ label: t('detail.created_at'), value: new Date(app.created_at).toLocaleDateString(dateFmt) })
    if (app.source) {
      const srcDomain = getSourceDomain(app.source)
      const srcIcon = srcDomain
        ? `<img src="${esc(faviconUrl(srcDomain))}" width="16" height="16" alt="" class="source-favicon" loading="lazy" data-hide-on-error />`
        : ''
      details.push({ label: t('detail.source'), value: app.source, iconHtml: srcIcon })
    }
    // safeHostname returns plain text: it is escaped below along with every other value.
    if (app.job_url && sanitizeUrl(app.job_url)) details.push({ label: t('detail.job_link'), value: safeHostname(app.job_url), href: sanitizeUrl(app.job_url) })

    const colCount = Math.min(propCells.length + details.length, 5)
    // relative z-10: the status and confidence menus open over the tabs below.
    detailsRow.className = `relative z-10 grid grid-cols-2 sm:grid-cols-3 ${LG_COLS[colCount] ?? ''} gap-y-5 gap-x-5 py-6 border-t border-b border-border/50`
    detailsRow.innerHTML = ''
    // The property cells keep their menus and listeners: they are moved, not rebuilt.
    detailsRow.append(...propCells)
    details.forEach(d => {
      const item = document.createElement('div')
      item.className = 'min-w-0'
      const icon = d.iconHtml || ''
      item.innerHTML = d.href
        ? `<span class="block text-sm text-muted mb-1">${esc(d.label)}</span>
           <a href="${esc(d.href)}" target="_blank" rel="noopener noreferrer" class="text-sm text-accent hover:text-accent-hover font-medium transition-colors inline-flex items-center gap-1 max-w-full truncate">${esc(d.value)}</a>`
        : `<span class="block text-sm text-muted mb-1">${esc(d.label)}</span>
           <span class="flex items-center gap-2 text-sm text-primary font-medium truncate">${icon}<span class="truncate">${esc(d.value)}</span></span>`
      detailsRow.appendChild(item)
    })
  }
  renderDetails()
  content.appendChild(detailsRow)

  const layout = document.createElement('div')
  layout.className = 'flex flex-col lg:flex-row gap-6'

  // Notes tab
  const notesPanel = document.createElement('div')
  notesPanel.className = 'p-4 sm:p-6 space-y-4'
  const notesTA = document.createElement('textarea')
  notesTA.className = 'input h-72 resize-y w-full'
  notesTA.value = app.notes ?? ''
  notesTA.placeholder = t('detail.no_notes')
  notesTA.setAttribute('aria-label', t('detail.tab_notes'))
  const notesSaveBtn = document.createElement('button')
  notesSaveBtn.className = 'btn-primary text-sm'
  notesSaveBtn.textContent = t('detail.save')
  notesSaveBtn.addEventListener('click', async () => {
    const value = notesTA.value
    try {
      await saveApp({ notes: value || undefined })
      app.notes = value || undefined
      notesTA.dataset.saved = value
      toast(t('detail.saved'), 'success')
    } catch {
      toast(t('form.error'), 'error')
    }
  })
  notesPanel.appendChild(notesTA)
  notesPanel.appendChild(notesSaveBtn)

  // Prep tab
  const prepPanel = document.createElement('div')
  prepPanel.className = 'p-4 sm:p-6 space-y-4'
  const prepTA = document.createElement('textarea')
  prepTA.className = 'input h-72 resize-y w-full'
  prepTA.value = app.speech ?? ''
  prepTA.placeholder = t('detail.no_prep')
  prepTA.setAttribute('aria-label', t('detail.tab_prep'))
  const prepSaveBtn = document.createElement('button')
  prepSaveBtn.className = 'btn-primary text-sm'
  prepSaveBtn.textContent = t('detail.save')
  prepSaveBtn.addEventListener('click', async () => {
    const value = prepTA.value
    try {
      await saveApp({ speech: value || undefined })
      app.speech = value || undefined
      prepTA.dataset.saved = value
      toast(t('detail.saved'), 'success')
    } catch {
      toast(t('form.error'), 'error')
    }
  })
  prepPanel.appendChild(prepTA)
  prepPanel.appendChild(prepSaveBtn)

  // Offer tab
  const offerPanel = document.createElement('div')
  offerPanel.className = 'p-4 sm:p-6 space-y-4'
  const offerTA = document.createElement('textarea')
  offerTA.className = 'input h-72 resize-y w-full whitespace-pre-wrap'
  offerTA.value = app.job_description ?? ''
  offerTA.placeholder = t('detail.no_offer')
  offerTA.setAttribute('aria-label', t('detail.tab_offer'))
  const offerSaveBtn = document.createElement('button')
  offerSaveBtn.className = 'btn-primary text-sm'
  offerSaveBtn.textContent = t('detail.save')
  offerSaveBtn.addEventListener('click', async () => {
    const value = offerTA.value
    try {
      await saveApp({ job_description: value || undefined })
      app.job_description = value || undefined
      offerTA.dataset.saved = value
      toast(t('detail.saved'), 'success')
    } catch {
      toast(t('form.error'), 'error')
    }
  })
  offerPanel.appendChild(offerTA)
  offerPanel.appendChild(offerSaveBtn)

  // Unsaved text in the three tabs above is guarded like the edit form. The baseline is the
  // textarea's own value (the API normalises CRLF to LF, so comparing with the raw field
  // would always look dirty), moved forward after each successful save.
  const textAreas = [notesTA, prepTA, offerTA]
  textAreas.forEach(ta => { ta.dataset.saved = ta.value })
  const isDirty = () => textAreas.some(ta => ta.value !== ta.dataset.saved)
  const handleBeforeUnload = (e: BeforeUnloadEvent) => {
    if (isDirty()) e.preventDefault()
  }
  window.addEventListener('beforeunload', handleBeforeUnload)
  setNavigationGuard(() => {
    if (!isDirty()) return true
    return confirm(t('form.unsaved_changes'))
  })
  setNavigationCleanup(() => {
    window.removeEventListener('beforeunload', handleBeforeUnload)
    docListeners.abort()
  })

  // Interviews tab
  const interviewsPanel = document.createElement('div')
  interviewsPanel.className = 'p-4 sm:p-6 space-y-4'

  // Set once the sidebar exists: every interview or contact change refreshes it.
  let onListsChanged = () => {}

  const renderInterviews = (list: Interview[]) => {
    onListsChanged()
    interviewsPanel.innerHTML = ''
    const addBtn = document.createElement('button')
    addBtn.className = 'btn-ghost text-sm gap-1.5 mb-4'
    addBtn.innerHTML = `${icons.plus} ${t('detail.add')}`
    addBtn.addEventListener('click', () => {
      const { el, getData } = buildInterviewForm()
      const modal = openModal({ title: t('detail.interview_add_title'), content: el })
      const saveBtn = el.querySelector<HTMLButtonElement>('[data-save]')!
      saveBtn.addEventListener('click', async () => {
        // Locked while the request is in flight and left locked on success (the modal is
        // closing), so a double click cannot create the interview twice.
        if (saveBtn.disabled) return
        saveBtn.disabled = true
        let created: Interview
        try {
          created = await api.interviews.create(id, getData())
        } catch {
          saveBtn.disabled = false
          toast(t('form.error'), 'error')
          return
        }
        list.push(created)
        modal.close()
        renderInterviews(list)
        updateTabCounts()
        void refreshTimeline()
        toast(t('detail.interview_added'), 'success')
      })
    })
    interviewsPanel.appendChild(addBtn)

    if (!list.length) {
      const empty = document.createElement('p')
      empty.className = 'text-sm text-muted'
      empty.textContent = t('detail.no_interviews')
      interviewsPanel.appendChild(empty)
      return
    }

    const ivList = document.createElement('div')
    ivList.className = 'space-y-3'
    list.forEach(iv => {
      const card = document.createElement('div')
      card.className = 'border border-border/50 rounded p-4 backdrop-blur-sm'
      card.style.background = 'rgb(var(--color-surface-1) / 0.4)'
      // scheduled_at is a floating wall-clock time (the typed local time, labelled Z):
      // format it in UTC so it shows exactly what was entered.
      card.innerHTML = `
        <div class="flex items-start justify-between gap-2">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 flex-wrap mb-1.5">
              <span class="text-sm font-semibold text-primary">${t('detail.round')} ${iv.round}</span>
              <span class="text-xs text-muted bg-surface-2 rounded px-2 py-0.5 font-medium">${esc(interviewTypeLabel(iv.type))}</span>
              ${iv.outcome ? `<span class="badge ${OUTCOME_COLORS[iv.outcome] ?? ''}">${esc(interviewOutcomeLabel(iv.outcome))}</span>` : ''}
            </div>
            ${iv.scheduled_at ? `<p class="text-xs text-muted mt-1 tabular-nums">${new Date(iv.scheduled_at).toLocaleString(dateFmt, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', timeZone: 'UTC' })}${iv.duration_minutes ? ` \u00b7 ${iv.duration_minutes} min` : ''}</p>` : ''}
            ${iv.interviewer_name ? `<p class="text-xs text-muted">${esc(iv.interviewer_name)}${iv.interviewer_role ? ` \u00b7 ${esc(iv.interviewer_role)}` : ''}</p>` : ''}
            ${iv.notes ? `<p class="text-xs text-muted mt-2 line-clamp-2">${esc(iv.notes)}</p>` : ''}
          </div>
          <div class="flex gap-1 shrink-0">
            <button class="btn-ghost p-1.5 min-w-[44px] min-h-[44px]" data-edit-iv="${esc(iv.id)}" aria-label="${t('detail.edit')}">${icons.edit}</button>
            <button class="btn-danger p-1.5 min-w-[44px] min-h-[44px]" data-del-iv="${esc(iv.id)}" aria-label="${t('detail.delete')}">${icons.trash}</button>
          </div>
        </div>
      `
      // Each card holds one button of each kind: no need to put the id in the selector.
      card.querySelector('[data-edit-iv]')?.addEventListener('click', () => {
        const { el, getData } = buildInterviewForm(iv)
        const modal = openModal({ title: t('detail.interview_edit_title'), content: el })
        const saveBtn = el.querySelector<HTMLButtonElement>('[data-save]')!
        saveBtn.addEventListener('click', async () => {
          if (saveBtn.disabled) return
          saveBtn.disabled = true
          let updated: Interview
          try {
            updated = await api.interviews.update(iv.id, getData())
          } catch {
            saveBtn.disabled = false
            toast(t('form.error'), 'error')
            return
          }
          Object.assign(iv, updated)
          modal.close()
          renderInterviews(list)
          toast(t('detail.interview_updated'), 'success')
        })
      })
      card.querySelector('[data-del-iv]')?.addEventListener('click', () => {
        const confirmEl = document.createElement('div')
        confirmEl.className = 'space-y-5'
        confirmEl.innerHTML = `
          <p class="text-sm text-muted">${t('detail.confirm_delete_interview')}</p>
          <div class="flex justify-end gap-2">
            <button data-cancel class="btn-ghost">${t('form.cancel')}</button>
            <button data-confirm class="btn-danger">${t('detail.delete')}</button>
          </div>
        `
        const modal = openModal({ title: t('detail.confirm_delete_interview'), content: confirmEl })
        confirmEl.querySelector('[data-cancel]')?.addEventListener('click', () => modal.close())
        const confirmBtn = confirmEl.querySelector<HTMLButtonElement>('[data-confirm]')!
        confirmBtn.addEventListener('click', async () => {
          // Locked from the first click: a second DELETE would answer 404 after the first
          // one succeeded. Unlocked only if the delete failed.
          if (confirmBtn.disabled) return
          confirmBtn.disabled = true
          try {
            await api.interviews.delete(iv.id)
            const idx = list.findIndex(x => x.id === iv.id)
            if (idx !== -1) list.splice(idx, 1)
            modal.close()
            renderInterviews(list)
            updateTabCounts()
            void refreshTimeline()
          } catch {
            confirmBtn.disabled = false
            toast(t('form.error'), 'error')
          }
        })
      })
      ivList.appendChild(card)
    })
    interviewsPanel.appendChild(ivList)
  }

  const interviews = app.interviews ?? []
  renderInterviews(interviews)

  // Contacts tab
  const contactsPanel = document.createElement('div')
  contactsPanel.className = 'p-4 sm:p-6 space-y-4'

  const renderContacts = (list: Contact[]) => {
    onListsChanged()
    contactsPanel.innerHTML = ''
    const addBtn = document.createElement('button')
    addBtn.className = 'btn-ghost text-sm gap-1.5 mb-4'
    addBtn.innerHTML = `${icons.plus} ${t('detail.add')}`
    addBtn.addEventListener('click', () => {
      const { el, getData } = buildContactForm()
      const modal = openModal({ title: t('detail.contact_add_title'), content: el })
      const saveBtn = el.querySelector<HTMLButtonElement>('[data-save]')!
      saveBtn.addEventListener('click', async () => {
        if (saveBtn.disabled) return
        const data = getData()
        if (!data) { toast(t('form.field_required'), 'error'); return }
        saveBtn.disabled = true
        let created: Contact
        try {
          created = await api.contacts.create(id, data)
        } catch {
          saveBtn.disabled = false
          toast(t('form.error'), 'error')
          return
        }
        list.push(created)
        modal.close()
        renderContacts(list)
        updateTabCounts()
        void refreshTimeline()
        toast(t('detail.contact_added'), 'success')
      })
    })
    contactsPanel.appendChild(addBtn)

    if (!list.length) {
      const empty = document.createElement('p')
      empty.className = 'text-sm text-muted'
      empty.textContent = t('detail.no_contacts')
      contactsPanel.appendChild(empty)
      return
    }

    const cList = document.createElement('div')
    cList.className = 'space-y-0 divide-y divide-border/60'
    list.forEach(c => {
      const row = document.createElement('div')
      row.className = 'flex items-center justify-between gap-2 py-3.5 first:pt-0 last:pb-0'
      const mailto = c.email ? mailtoHref(c.email) : ''
      row.innerHTML = `
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <span class="text-sm font-medium text-primary">${esc(c.name)}</span>
            ${c.role ? `<span class="text-xs text-muted bg-surface-2 rounded px-2 py-0.5">${esc(c.role)}</span>` : ''}
          </div>
          <div class="flex items-center gap-3 mt-1 flex-wrap">
            ${c.email ? (mailto
              ? `<a href="${esc(mailto)}" class="text-xs text-accent hover:text-accent-hover transition-colors">${esc(c.email)}</a>`
              : `<span class="text-xs text-muted">${esc(c.email)}</span>`) : ''}
            ${c.phone ? `<span class="text-xs text-muted">${esc(c.phone)}</span>` : ''}
            ${c.linkedin && sanitizeUrl(c.linkedin) ? `<a href="${esc(sanitizeUrl(c.linkedin))}" target="_blank" rel="noopener noreferrer" class="text-xs text-accent hover:text-accent-hover transition-colors">LinkedIn</a>` : ''}
          </div>
        </div>
        <div class="flex gap-1 shrink-0">
          <button class="btn-ghost p-1.5 min-w-[44px] min-h-[44px]" data-edit-c="${esc(c.id)}" aria-label="${t('detail.edit')}">${icons.edit}</button>
          <button class="btn-danger p-1.5 min-w-[44px] min-h-[44px]" data-del-c="${esc(c.id)}" aria-label="${t('detail.delete')}">${icons.trash}</button>
        </div>
      `
      row.querySelector('[data-edit-c]')?.addEventListener('click', () => {
        const { el, getData } = buildContactForm(c)
        const modal = openModal({ title: t('detail.contact_edit_title'), content: el })
        const saveBtn = el.querySelector<HTMLButtonElement>('[data-save]')!
        saveBtn.addEventListener('click', async () => {
          if (saveBtn.disabled) return
          const data = getData()
          if (!data) { toast(t('form.field_required'), 'error'); return }
          saveBtn.disabled = true
          let updated: Contact
          try {
            updated = await api.contacts.update(c.id, data)
          } catch {
            saveBtn.disabled = false
            toast(t('form.error'), 'error')
            return
          }
          Object.assign(c, updated)
          modal.close()
          renderContacts(list)
          toast(t('detail.contact_updated'), 'success')
        })
      })
      row.querySelector('[data-del-c]')?.addEventListener('click', () => {
        const confirmEl = document.createElement('div')
        confirmEl.className = 'space-y-5'
        confirmEl.innerHTML = `
          <p class="text-sm text-muted">${t('detail.confirm_delete_contact')}</p>
          <div class="flex justify-end gap-2">
            <button data-cancel class="btn-ghost">${t('form.cancel')}</button>
            <button data-confirm class="btn-danger">${t('detail.delete')}</button>
          </div>
        `
        const modal = openModal({ title: t('detail.confirm_delete_contact'), content: confirmEl })
        confirmEl.querySelector('[data-cancel]')?.addEventListener('click', () => modal.close())
        const confirmBtn = confirmEl.querySelector<HTMLButtonElement>('[data-confirm]')!
        confirmBtn.addEventListener('click', async () => {
          if (confirmBtn.disabled) return
          confirmBtn.disabled = true
          try {
            await api.contacts.delete(c.id)
            const idx = list.findIndex(x => x.id === c.id)
            if (idx !== -1) list.splice(idx, 1)
            modal.close()
            renderContacts(list)
            updateTabCounts()
            void refreshTimeline()
          } catch {
            confirmBtn.disabled = false
            toast(t('form.error'), 'error')
          }
        })
      })
      cList.appendChild(row)
    })
    contactsPanel.appendChild(cList)
  }

  const contacts = app.contacts ?? []
  renderContacts(contacts)

  // Timeline tab
  const timelinePanel = document.createElement('div')
  timelinePanel.className = 'p-4 sm:p-6 space-y-4'
  const renderTimeline = () => {
    timelinePanel.innerHTML = ''
    if (!app.timeline_events?.length) {
      timelinePanel.innerHTML = `<p class="text-sm text-muted">${t('detail.no_timeline')}</p>`
      return
    }
    const list = document.createElement('div')
    list.className = 'relative ml-3'
    const lineDiv = document.createElement('div')
    lineDiv.className = 'absolute left-0 top-2 bottom-2 w-px bg-border'
    list.appendChild(lineDiv)
    const dotColors: Record<string, string> = { created: 'bg-emerald-500', status_change: 'bg-accent', interview_added: 'bg-orange-400', interview_deleted: 'bg-orange-400', contact_added: 'bg-teal-500', contact_deleted: 'bg-teal-500' }
    ;[...app.timeline_events].reverse().forEach(e => {
      const row = document.createElement('div')
      row.className = 'flex items-start gap-3 relative pl-5 pb-5 last:pb-0'
      row.innerHTML = `
        <div class="absolute left-0 top-2 -translate-x-1/2 w-2.5 h-2.5 rounded-full ${dotColors[e.event_type] || 'bg-accent'} ring-2 ring-surface-1 shrink-0"></div>
        <div class="min-w-0">
          <p class="text-sm text-primary">${esc(translateTimelineEvent(e.event_type, e.description))}</p>
          <p class="text-xs text-muted mt-0.5 tabular-nums">${new Date(e.created_at).toLocaleString(dateFmt)}</p>
        </div>
      `
      list.appendChild(row)
    })
    timelinePanel.appendChild(list)
  }
  renderTimeline()

  // The server writes the timeline events (and their text, which translateTimelineEvent
  // parses), so reload them after a change instead of guessing them locally.
  const refreshTimeline = async () => {
    try {
      const fresh = await api.applications.get(id)
      app.timeline_events = fresh.timeline_events
      renderTimeline()
    } catch { /* keep the timeline already shown */ }
  }

  // Assemble tabs
  const countLabel = (key: string, n: number) => `${t(key)}${n ? ` (${n})` : ''}`
  const tabs = makeTabs([
    { id: 'notes', label: t('detail.tab_notes'), panel: notesPanel },
    { id: 'prep', label: t('detail.tab_prep'), panel: prepPanel },
    { id: 'offer', label: t('detail.tab_offer'), panel: offerPanel },
    { id: 'interviews', label: countLabel('detail.tab_interviews', interviews.length), panel: interviewsPanel },
    { id: 'contacts', label: countLabel('detail.tab_contacts', contacts.length), panel: contactsPanel },
    { id: 'timeline', label: t('detail.tab_timeline'), panel: timelinePanel },
  ])
  // Called after adding or deleting an interview or a contact (never during the first
  // render above, before the tabs exist).
  function updateTabCounts() {
    tabs.setLabel('interviews', countLabel('detail.tab_interviews', interviews.length))
    tabs.setLabel('contacts', countLabel('detail.tab_contacts', contacts.length))
  }

  const tabCard = document.createElement('div')
  tabCard.className = 'card !p-0 flex-1 min-w-0 overflow-hidden'
  tabCard.appendChild(tabs.el)

  const sidebar = document.createElement('div')
  sidebar.className = 'lg:w-72 shrink-0 space-y-4'

  const showTab = (id: string) => {
    const btn = tabs.el.querySelector<HTMLElement>(`[data-tab="${id}"]`)
    btn?.click()
    btn?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    btn?.focus({ preventScroll: true })
  }

  // Next interview: scheduled_at is floating wall-clock time, so "now" is compared in
  // the same frame (the local clock read as UTC). An interview stays here until it is
  // over: its start plus its duration (an hour when unknown), or the whole day when it
  // has no time (stored as midnight). Cancelled ones are skipped.
  const nextCard = document.createElement('div')
  nextCard.className = 'card space-y-2'
  const isDateOnly = (at: string) => at.endsWith('T00:00:00Z')
  const renderNextInterview = () => {
    const d = new Date()
    const now = Date.UTC(d.getFullYear(), d.getMonth(), d.getDate(), d.getHours(), d.getMinutes())
    const notOver = (iv: Interview) => {
      const start = new Date(iv.scheduled_at!).getTime()
      if (isDateOnly(iv.scheduled_at!)) return start + 86_400_000 > now
      return start + (iv.duration_minutes ?? 60) * 60_000 >= now
    }
    const next = interviews
      .filter(iv => iv.scheduled_at && iv.outcome !== 'Cancelled' && notOver(iv))
      .sort((a, b) => new Date(a.scheduled_at!).getTime() - new Date(b.scheduled_at!).getTime())[0]
    nextCard.hidden = !next
    if (!next) return
    const at = new Date(next.scheduled_at!)
    const opts: Intl.DateTimeFormatOptions = { weekday: 'long', day: 'numeric', month: 'long', timeZone: 'UTC' }
    if (at.getUTCFullYear() !== d.getFullYear()) opts.year = 'numeric'
    if (!isDateOnly(next.scheduled_at!)) { opts.hour = '2-digit'; opts.minute = '2-digit' }
    const when = at.toLocaleString(dateFmt, opts)
    nextCard.innerHTML = `
      <h3 class="text-sm font-semibold text-primary">${t('detail.next_interview')}</h3>
      <p class="text-sm text-primary">${esc(when)}</p>
      <p class="text-sm text-muted">${esc(interviewTypeLabel(next.type))} · ${t('detail.round')} ${next.round}${next.duration_minutes ? ` · ${next.duration_minutes} min` : ''}</p>
      ${next.interviewer_name ? `<p class="text-sm text-muted">${esc(next.interviewer_name)}${next.interviewer_role ? ` · ${esc(next.interviewer_role)}` : ''}</p>` : ''}
      <button type="button" data-show="interviews" class="text-sm text-accent hover:text-accent-hover font-medium transition-colors">${t('detail.see_interviews')}</button>
    `
  }

  const contactsCard = document.createElement('div')
  contactsCard.className = 'card space-y-3'
  const renderContactsCard = () => {
    contactsCard.hidden = contacts.length === 0
    if (!contacts.length) return
    contactsCard.innerHTML = `
      <h3 class="text-sm font-semibold text-primary">${t('detail.tab_contacts')}</h3>
      ${contacts.map(c => {
        const mailto = c.email ? mailtoHref(c.email) : ''
        const tel = c.phone ? telHref(c.phone) : ''
        const linkedin = c.linkedin ? sanitizeUrl(c.linkedin) : ''
        return `
          <div class="min-w-0">
            <p class="text-sm font-medium text-primary truncate">${esc(c.name)}</p>
            ${c.role ? `<p class="text-sm text-muted truncate">${esc(c.role)}</p>` : ''}
            <div class="flex flex-wrap gap-x-3 gap-y-0.5 mt-0.5">
              ${c.email ? (mailto
                ? `<a href="${esc(mailto)}" class="text-sm text-accent hover:text-accent-hover transition-colors truncate max-w-full">${esc(c.email)}</a>`
                : `<span class="text-sm text-muted truncate max-w-full">${esc(c.email)}</span>`) : ''}
              ${c.phone ? (tel
                ? `<a href="${esc(tel)}" class="text-sm text-accent hover:text-accent-hover transition-colors">${esc(c.phone)}</a>`
                : `<span class="text-sm text-muted">${esc(c.phone)}</span>`) : ''}
              ${linkedin ? `<a href="${esc(linkedin)}" target="_blank" rel="noopener noreferrer" class="text-sm text-accent hover:text-accent-hover transition-colors">LinkedIn</a>` : ''}
            </div>
          </div>`
      }).join('')}
      <button type="button" data-show="contacts" class="text-sm text-accent hover:text-accent-hover font-medium transition-colors">${t('detail.manage_contacts')}</button>
    `
  }

  sidebar.addEventListener('click', (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLElement>('[data-show]')
    if (btn) showTab(btn.dataset.show!)
  })
  const renderSidebar = () => {
    renderNextInterview()
    renderContactsCard()
    sidebar.hidden = ![...sidebar.children].some(c => !(c as HTMLElement).hidden)
  }
  sidebar.append(nextCard, contactsCard)

  if (app.company_website || app.company_industry || app.company_size || app.company_location) {
    const companyCard = document.createElement('div')
    companyCard.className = 'card space-y-3'
    companyCard.innerHTML = `<h3 class="text-sm font-semibold text-primary">${t('detail.company_info')}</h3>`

    if (app.company_website && sanitizeUrl(app.company_website)) {
      const row = document.createElement('div')
      row.innerHTML = `
        <p class="text-sm text-muted mb-0.5">${t('form.company_website')}</p>
        <a href="${esc(sanitizeUrl(app.company_website))}" target="_blank" rel="noopener noreferrer"
           class="text-sm text-accent hover:text-accent-hover flex items-center gap-1.5 transition-colors font-medium">
          ${icons.globe} ${esc(safeHostname(app.company_website))}
        </a>
      `
      companyCard.appendChild(row)
    }
    if (app.company_industry) {
      const row = document.createElement('div')
      row.innerHTML = `<p class="text-sm text-muted mb-0.5">${t('form.company_industry')}</p><p class="text-sm text-primary">${esc(app.company_industry)}</p>`
      companyCard.appendChild(row)
    }
    if (app.company_size) {
      const row = document.createElement('div')
      row.innerHTML = `<p class="text-sm text-muted mb-0.5">${t('form.company_size')}</p><p class="text-sm text-primary">${esc(app.company_size)}</p>`
      companyCard.appendChild(row)
    }
    if (app.company_location) {
      const row = document.createElement('div')
      row.innerHTML = `<p class="text-sm text-muted mb-0.5">${t('form.company_location')}</p><p class="text-sm text-primary">${esc(app.company_location)}</p>`
      companyCard.appendChild(row)
    }
    sidebar.appendChild(companyCard)
  }

  layout.appendChild(tabCard)
  layout.appendChild(sidebar)
  onListsChanged = renderSidebar
  renderSidebar()
  content.appendChild(layout)

  topBar.querySelector('#delete-btn')?.addEventListener('click', () => {
    const confirmEl = document.createElement('div')
    confirmEl.className = 'space-y-5'
    confirmEl.innerHTML = `
      <p class="text-sm text-muted">${t('detail.confirm_delete')}</p>
      <div class="flex justify-end gap-2">
        <button data-cancel class="btn-ghost">${t('form.cancel')}</button>
        <button data-confirm class="btn-danger">${t('detail.delete')}</button>
      </div>
    `
    const modal = openModal({ title: t('detail.confirm_delete'), content: confirmEl })
    confirmEl.querySelector('[data-cancel]')?.addEventListener('click', () => modal.close())
    const confirmBtn = confirmEl.querySelector<HTMLButtonElement>('[data-confirm]')!
    confirmBtn.addEventListener('click', async () => {
      if (confirmBtn.disabled) return
      confirmBtn.disabled = true
      try {
        await api.applications.delete(app.id)
        modal.close()
        // The record is gone: do not ask about its unsaved notes on the way out.
        setNavigationGuard(null)
        navigate('/applications')
      } catch {
        confirmBtn.disabled = false
        toast(t('form.error'), 'error')
      }
    })
  })

  return createLayout(content)
}
