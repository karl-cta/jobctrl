import { api } from '../api'
import { createLayout } from '../components/layout'
import { openModal } from '../components/modal'
import { navigate } from '../router'
import { toast } from '../components/toast'
import { t, getDateLocale, translateTimelineEvent } from '../i18n'
import { icons } from '../icons'
import { esc, sanitizeUrl, safeHostname } from '../sanitize'
import {
  statusLabel,
  STATUS_COLORS,
  ALL_STATUSES,
  type ApplicationStatus,
  type Interview,
  type Contact,
} from '../types'

const OUTCOME_COLORS: Record<string, string> = {
  Passed: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  Failed: 'bg-rose-50 text-rose-600 dark:bg-rose-900/40 dark:text-rose-300',
  Pending: 'bg-slate-100 text-slate-600 dark:bg-slate-800/80 dark:text-slate-300',
  Cancelled: 'bg-zinc-100 text-zinc-500 dark:bg-zinc-800/60 dark:text-zinc-400',
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
          ${['Phone', 'Video', 'On-site', 'Technical', 'HR', 'Culture', 'Final'].map(type =>
            `<option value="${type}" ${iv?.type === type ? 'selected' : ''}>${type}</option>`
          ).join('')}
        </select>
      </div>
    </div>
    <div>
      <label class="label">${t('detail.interview_scheduled')}</label>
      <div class="grid grid-cols-2 gap-3">
        <input id="iv-scheduled-date" name="scheduled_date" class="input" type="date" aria-label="${t('detail.interview_scheduled')} (date)"
          value="${iv?.scheduled_at ? new Date(iv.scheduled_at).toISOString().slice(0, 10) : ''}" />
        <input id="iv-scheduled-time" name="scheduled_time" class="input" type="time" aria-label="${t('detail.interview_scheduled')} (time)"
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
          ${['Passed', 'Failed', 'Pending', 'Cancelled'].map(o =>
            `<option value="${o}" ${iv?.outcome === o ? 'selected' : ''}>${o}</option>`
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
      <textarea id="iv-notes" name="notes" class="input min-h-[60px]">${esc(iv?.notes)}</textarea>
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
      <textarea id="ct-notes" name="notes" class="input min-h-[60px]">${esc(c?.notes)}</textarea>
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

function makeTabs(tabs: Array<{ id: string; label: string; panel: HTMLElement }>): HTMLElement {
  const wrapper = document.createElement('div')
  wrapper.className = 'space-y-0'

  const bar = document.createElement('div')
  bar.className = 'flex gap-0 border-b border-border overflow-x-auto'
  bar.setAttribute('role', 'tablist')

  const panels: HTMLElement[] = []

  const tabClass = (active: boolean) =>
    `px-4 py-3 text-sm font-medium border-b-2 transition-all duration-150 whitespace-nowrap focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:ring-inset ${
      active
        ? 'border-accent text-accent'
        : 'border-transparent text-muted hover:text-primary hover:border-surface-3'
    }`

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

  function activateTab(activeId: string) {
    bar.querySelectorAll<HTMLElement>('[data-tab]').forEach(b => {
      const isActive = b.dataset.tab === activeId
      b.setAttribute('aria-selected', isActive ? 'true' : 'false')
      b.setAttribute('tabindex', isActive ? '0' : '-1')
      b.className = tabClass(isActive)
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

  wrapper.appendChild(bar)
  panels.forEach(p => wrapper.appendChild(p))
  return wrapper
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

  const content = document.createElement('div')
  content.className = 'space-y-6 max-w-5xl stagger'

  const header = document.createElement('div')
  header.className = 'flex flex-col sm:flex-row sm:items-start justify-between gap-4 relative z-10'

  // Status dropdown
  const statusWrapper = document.createElement('div')
  statusWrapper.className = 'relative'

  const statusBtn = document.createElement('button')
  statusBtn.className = `badge ${STATUS_COLORS[app.status as ApplicationStatus]} cursor-pointer hover:opacity-80 transition-all duration-150`
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
    Withdrawn: 'bg-stone-400',
  }

  const checkSvg = '<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="2.5" stroke="currentColor" class="w-3.5 h-3.5 text-accent shrink-0"><path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5"/></svg>'

  const statusDropdown = document.createElement('div')
  statusDropdown.className = 'hidden absolute top-full left-0 mt-2 z-40 bg-surface-1 border border-border rounded-xl py-1 min-w-[200px]'
  statusDropdown.style.boxShadow = 'var(--shadow-elevated)'
  statusDropdown.setAttribute('role', 'menu')

  function renderStatusItems() {
    statusDropdown.innerHTML = ''
    ALL_STATUSES.forEach(s => {
      const isCurrent = s === app!.status
      const item = document.createElement('button')
      item.className = `w-full text-left px-3 py-2 text-sm rounded-lg mx-0 hover:bg-surface-2 focus:bg-surface-2 focus:outline-none transition-colors flex items-center gap-2.5 ${isCurrent ? 'font-semibold text-accent' : 'text-primary'}`
      item.setAttribute('role', 'menuitem')
      item.innerHTML = `
        <span class="w-2 h-2 rounded-full ${statusDotColors[s] || 'bg-accent'} shrink-0"></span>
        <span class="flex-1">${statusLabel(s)}</span>
        ${isCurrent ? checkSvg : '<span class="w-3.5"></span>'}
      `
      item.addEventListener('click', async () => {
        statusDropdown.classList.add('hidden')
        statusBtn.setAttribute('aria-expanded', 'false')
        try {
          await api.applications.update(id, { ...app!, status: s })
          app!.status = s
          statusBtn.className = `badge ${STATUS_COLORS[s]} cursor-pointer hover:opacity-80 transition-all duration-150`
          statusBtn.textContent = statusLabel(s)
          renderStatusItems()
          const celebrate: Record<string, string> = { Offer: t('list.celebrate_offer'), Accepted: t('list.celebrate_accepted') }
          toast(celebrate[s] || statusLabel(s), 'success')
        } catch {
          toast(t('form.error'), 'error')
        }
      })
      statusDropdown.appendChild(item)
    })
  }
  renderStatusItems()

  statusBtn.addEventListener('click', (e) => {
    e.stopPropagation()
    const open = !statusDropdown.classList.contains('hidden')
    statusDropdown.classList.toggle('hidden', open)
    statusBtn.setAttribute('aria-expanded', open ? 'false' : 'true')
    if (!open) {
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
        statusDropdown.classList.add('hidden')
        statusBtn.setAttribute('aria-expanded', 'false')
        statusBtn.focus()
        e.stopPropagation()
        return
      default: return
    }
    e.preventDefault()
    items[next]?.focus()
  })
  document.addEventListener('click', () => {
    statusDropdown.classList.add('hidden')
    statusBtn.setAttribute('aria-expanded', 'false')
  }, { capture: true })

  statusWrapper.appendChild(statusBtn)
  statusWrapper.appendChild(statusDropdown)

  // Rating stars
  const ratingEl = document.createElement('div')
  ratingEl.className = 'flex items-center gap-0.5'
  if (app.rating) {
    for (let i = 1; i <= 5; i++) {
      const star = document.createElement('span')
      star.className = i <= app.rating ? 'text-amber-400' : 'text-surface-3'
      star.setAttribute('aria-hidden', 'true')
      star.innerHTML = icons.star
      ratingEl.appendChild(star)
    }
    ratingEl.setAttribute('aria-label', `${app.rating}/5`)
  }

  const titleGroup = document.createElement('div')
  titleGroup.innerHTML = `
    <div class="flex items-center gap-3 mb-1">
      <a href="/applications" data-link class="text-muted hover:text-primary transition-colors" aria-label="${t('form.back')}">${icons.arrowLeft}</a>
      <h1 class="text-2xl font-bold text-primary tracking-tight">${esc(app.company_name)}</h1>
    </div>
    <p class="text-muted text-base ml-7 mb-2">${esc(app.job_title)}</p>
    ${app.location ? `<p class="text-muted/70 text-sm flex items-center gap-1.5 ml-7"><span aria-hidden="true" class="opacity-60">${icons.pin}</span> ${esc(app.location)}</p>` : ''}
  `
  const confidenceColors: Record<number, string> = {
    1: 'bg-stone-100 text-stone-600 dark:bg-stone-800/60 dark:text-stone-400',
    2: 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-400',
    3: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-400',
    4: 'bg-teal-50 text-teal-700 dark:bg-teal-500/15 dark:text-teal-400',
  }

  const statusRatingRow = document.createElement('div')
  statusRatingRow.className = 'flex items-center gap-3 mt-3 ml-7 flex-wrap'
  statusRatingRow.appendChild(statusWrapper)
  if (app.rating) statusRatingRow.appendChild(ratingEl)
  if (app.confidence && app.confidence >= 1 && app.confidence <= 4) {
    const confBadge = document.createElement('span')
    confBadge.className = `badge ${confidenceColors[app.confidence]}`
    confBadge.textContent = t('form.confidence_' + app.confidence)
    statusRatingRow.appendChild(confBadge)
  }
  titleGroup.appendChild(statusRatingRow)

  const actions = document.createElement('div')
  actions.className = 'flex gap-2 shrink-0'
  actions.innerHTML = `
    <a href="/applications/${app.id}/edit" data-link class="btn-ghost gap-1.5 text-sm"><span aria-hidden="true">${icons.edit}</span> ${t('detail.edit')}</a>
    <button id="delete-btn" class="btn-danger gap-1.5 text-sm"><span aria-hidden="true">${icons.trash}</span> ${t('detail.delete')}</button>
  `

  header.appendChild(titleGroup)
  header.appendChild(actions)
  content.appendChild(header)

  const chips = document.createElement('div')
  chips.className = 'flex flex-wrap gap-3 ml-7'

  const chip = (label: string, value: string) => {
    const d = document.createElement('div')
    d.className = 'bg-surface-2/60 rounded-lg px-3.5 py-2.5'
    d.innerHTML = `<p class="text-xs text-muted mb-0.5 font-medium">${esc(label)}</p><p class="text-sm text-primary font-medium">${esc(value)}</p>`
    return d
  }

  if (app.contract_type) chips.appendChild(chip(t('detail.contract'), app.contract_type))
  if (app.work_mode) chips.appendChild(chip(t('detail.mode'), app.work_mode))
  if (app.salary) {
    chips.appendChild(chip(
      t('detail.salary'),
      `${app.salary / 1000}k \u20ac`
    ))
  }
  if (app.applied_at) {
    chips.appendChild(chip(t('detail.applied_at'), new Date(app.applied_at).toLocaleDateString(dateFmt)))
  }
  if (app.source) chips.appendChild(chip(t('detail.source'), app.source))
  if (app.job_url && sanitizeUrl(app.job_url)) {
    const linkChip = document.createElement('div')
    linkChip.className = 'bg-surface-2/60 rounded-lg px-3.5 py-2.5'
    linkChip.innerHTML = `
      <p class="text-xs text-muted mb-0.5 font-medium">${t('detail.job_link')}</p>
      <a href="${esc(sanitizeUrl(app.job_url))}" target="_blank" rel="noopener noreferrer"
         class="text-sm text-accent hover:text-accent-hover font-medium flex items-center gap-1.5 transition-colors">
        ${icons.globe} ${safeHostname(app.job_url)}
      </a>
    `
    chips.appendChild(linkChip)
  }
  content.appendChild(chips)

  const layout = document.createElement('div')
  layout.className = 'flex flex-col lg:flex-row gap-5'


  // Notes tab
  const notesPanel = document.createElement('div')
  notesPanel.className = 'p-5 space-y-3'
  {
    const notesTA = document.createElement('textarea')
    notesTA.className = 'input min-h-[150px] w-full'
    notesTA.value = app.notes ?? ''
    notesTA.placeholder = t('detail.no_notes')
    notesTA.setAttribute('aria-label', t('detail.tab_notes'))
    const notesSaveBtn = document.createElement('button')
    notesSaveBtn.className = 'btn-primary text-sm'
    notesSaveBtn.textContent = t('detail.save')
    notesSaveBtn.addEventListener('click', async () => {
      try {
        await api.applications.update(id, { ...app, notes: notesTA.value || undefined })
        app.notes = notesTA.value || undefined
        toast(t('detail.save'), 'success')
      } catch {
        toast(t('form.error'), 'error')
      }
    })
    notesPanel.appendChild(notesTA)
    notesPanel.appendChild(notesSaveBtn)
  }

  // Prep tab
  const prepPanel = document.createElement('div')
  prepPanel.className = 'p-5 space-y-3'
  const prepTA = document.createElement('textarea')
  prepTA.className = 'input min-h-[150px] w-full font-mono text-sm'
  prepTA.value = app.speech ?? ''
  prepTA.placeholder = t('detail.no_prep')
  prepTA.setAttribute('aria-label', t('detail.tab_prep'))
  const prepSaveBtn = document.createElement('button')
  prepSaveBtn.className = 'btn-primary text-sm'
  prepSaveBtn.textContent = t('detail.save')
  prepSaveBtn.addEventListener('click', async () => {
    try {
      await api.applications.update(id, { ...app, speech: prepTA.value || undefined })
      app.speech = prepTA.value || undefined
      toast(t('detail.save'), 'success')
    } catch {
      toast(t('form.error'), 'error')
    }
  })
  prepPanel.appendChild(prepTA)
  prepPanel.appendChild(prepSaveBtn)

  // Offer tab
  const offerPanel = document.createElement('div')
  offerPanel.className = 'p-5 space-y-3'
  const offerTA = document.createElement('textarea')
  offerTA.className = 'input min-h-[150px] w-full whitespace-pre-wrap'
  offerTA.value = app.job_description ?? ''
  offerTA.placeholder = t('detail.no_offer')
  offerTA.setAttribute('aria-label', t('detail.tab_offer'))
  const offerSaveBtn = document.createElement('button')
  offerSaveBtn.className = 'btn-primary text-sm'
  offerSaveBtn.textContent = t('detail.save')
  offerSaveBtn.addEventListener('click', async () => {
    try {
      await api.applications.update(id, { ...app, job_description: offerTA.value || undefined })
      app.job_description = offerTA.value || undefined
      toast(t('detail.save'), 'success')
    } catch {
      toast(t('form.error'), 'error')
    }
  })
  offerPanel.appendChild(offerTA)
  offerPanel.appendChild(offerSaveBtn)

  // Interviews tab
  const interviewsPanel = document.createElement('div')
  interviewsPanel.className = 'p-5 space-y-3'

  const renderInterviews = (list: Interview[]) => {
    interviewsPanel.innerHTML = ''
    const addBtn = document.createElement('button')
    addBtn.className = 'btn-ghost text-sm gap-1.5 mb-4'
    addBtn.innerHTML = `${icons.plus} ${t('detail.add')}`
    addBtn.addEventListener('click', () => {
      const { el, getData } = buildInterviewForm()
      const overlay = openModal({ title: t('detail.interview_add_title'), content: el })
      el.querySelector('[data-save]')?.addEventListener('click', async () => {
        try {
          const data = getData()
          const created = await api.interviews.create(id, data)
          list.push(created)
          overlay.remove()
          renderInterviews(list)
          toast(t('detail.interview_add_title'), 'success')
        } catch {
          toast(t('form.error'), 'error')
        }
      })
    })
    interviewsPanel.appendChild(addBtn)

    if (!list.length) {
      const empty = document.createElement('p')
      empty.className = 'text-sm text-muted/60'
      empty.textContent = t('detail.no_interviews')
      interviewsPanel.appendChild(empty)
      return
    }

    const ivList = document.createElement('div')
    ivList.className = 'space-y-3'
    list.forEach(iv => {
      const card = document.createElement('div')
      card.className = 'border border-border rounded-xl p-4 hover:border-border/80 transition-all duration-150'
      card.innerHTML = `
        <div class="flex items-start justify-between gap-2">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 flex-wrap mb-1.5">
              <span class="text-sm font-semibold text-primary">${t('detail.round')} ${iv.round}</span>
              <span class="text-xs text-muted bg-surface-2 rounded-md px-2 py-0.5 font-medium">${iv.type}</span>
              ${iv.outcome ? `<span class="badge ${OUTCOME_COLORS[iv.outcome] ?? ''}">${iv.outcome}</span>` : ''}
            </div>
            ${iv.scheduled_at ? `<p class="text-xs text-muted mt-1 tabular-nums">${new Date(iv.scheduled_at).toLocaleString(dateFmt)}${iv.duration_minutes ? ` \u00b7 ${iv.duration_minutes} min` : ''}</p>` : ''}
            ${iv.interviewer_name ? `<p class="text-xs text-muted">${esc(iv.interviewer_name)}${iv.interviewer_role ? ` \u00b7 ${esc(iv.interviewer_role)}` : ''}</p>` : ''}
            ${iv.notes ? `<p class="text-xs text-muted/70 mt-2 line-clamp-2">${esc(iv.notes)}</p>` : ''}
          </div>
          <div class="flex gap-1 shrink-0">
            <button class="btn-ghost p-1.5 min-w-[44px] min-h-[44px]" data-edit-iv="${iv.id}" aria-label="${t('detail.edit')}">${icons.edit}</button>
            <button class="btn-danger p-1.5 min-w-[44px] min-h-[44px]" data-del-iv="${iv.id}" aria-label="${t('detail.delete')}">${icons.trash}</button>
          </div>
        </div>
      `
      card.querySelector(`[data-edit-iv="${iv.id}"]`)?.addEventListener('click', () => {
        const { el, getData } = buildInterviewForm(iv)
        const overlay = openModal({ title: t('detail.interview_edit_title'), content: el })
        el.querySelector('[data-save]')?.addEventListener('click', async () => {
          try {
            const data = getData()
            const updated = await api.interviews.update(iv.id, data)
            Object.assign(iv, updated)
            overlay.remove()
            renderInterviews(list)
            toast(t('detail.interview_edit_title'), 'success')
          } catch {
            toast(t('form.error'), 'error')
          }
        })
      })
      card.querySelector(`[data-del-iv="${iv.id}"]`)?.addEventListener('click', () => {
        const confirmEl = document.createElement('div')
        confirmEl.className = 'space-y-5'
        confirmEl.innerHTML = `
          <p class="text-sm text-muted">${t('detail.confirm_delete_interview')}</p>
          <div class="flex justify-end gap-2">
            <button data-cancel class="btn-ghost">${t('form.cancel')}</button>
            <button data-confirm class="btn-danger">${t('detail.delete')}</button>
          </div>
        `
        const overlay = openModal({ title: t('detail.confirm_delete_interview'), content: confirmEl })
        confirmEl.querySelector('[data-cancel]')?.addEventListener('click', () => overlay.remove())
        confirmEl.querySelector('[data-confirm]')?.addEventListener('click', async () => {
          try {
            await api.interviews.delete(iv.id)
            const idx = list.findIndex(x => x.id === iv.id)
            if (idx !== -1) list.splice(idx, 1)
            overlay.remove()
            renderInterviews(list)
          } catch {
            toast(t('form.error'), 'error')
          }
        })
      })
      ivList.appendChild(card)
    })
    interviewsPanel.appendChild(ivList)
  }

  renderInterviews(app.interviews ?? [])

  // Contacts tab
  const contactsPanel = document.createElement('div')
  contactsPanel.className = 'p-5 space-y-3'

  const renderContacts = (list: Contact[]) => {
    contactsPanel.innerHTML = ''
    const addBtn = document.createElement('button')
    addBtn.className = 'btn-ghost text-sm gap-1.5 mb-4'
    addBtn.innerHTML = `${icons.plus} ${t('detail.add')}`
    addBtn.addEventListener('click', () => {
      const { el, getData } = buildContactForm()
      const overlay = openModal({ title: t('detail.contact_add_title'), content: el })
      el.querySelector('[data-save]')?.addEventListener('click', async () => {
        const data = getData()
        if (!data) { toast(t('form.field_required'), 'error'); return }
        try {
          const created = await api.contacts.create(id, data)
          list.push(created)
          overlay.remove()
          renderContacts(list)
          toast(t('detail.contact_add_title'), 'success')
        } catch {
          toast(t('form.error'), 'error')
        }
      })
    })
    contactsPanel.appendChild(addBtn)

    if (!list.length) {
      const empty = document.createElement('p')
      empty.className = 'text-sm text-muted/60'
      empty.textContent = t('detail.no_contacts')
      contactsPanel.appendChild(empty)
      return
    }

    const cList = document.createElement('div')
    cList.className = 'space-y-0 divide-y divide-border/60'
    list.forEach(c => {
      const row = document.createElement('div')
      row.className = 'flex items-center justify-between gap-2 py-3.5 first:pt-0 last:pb-0'
      row.innerHTML = `
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <span class="text-sm font-medium text-primary">${esc(c.name)}</span>
            ${c.role ? `<span class="text-xs text-muted bg-surface-2 rounded-md px-2 py-0.5">${esc(c.role)}</span>` : ''}
          </div>
          <div class="flex items-center gap-3 mt-1 flex-wrap">
            ${c.email ? `<a href="mailto:${esc(c.email)}" class="text-xs text-accent hover:text-accent-hover transition-colors">${esc(c.email)}</a>` : ''}
            ${c.phone ? `<span class="text-xs text-muted">${esc(c.phone)}</span>` : ''}
            ${c.linkedin && sanitizeUrl(c.linkedin) ? `<a href="${esc(sanitizeUrl(c.linkedin))}" target="_blank" rel="noopener noreferrer" class="text-xs text-accent hover:text-accent-hover transition-colors">LinkedIn</a>` : ''}
          </div>
        </div>
        <div class="flex gap-1 shrink-0">
          <button class="btn-ghost p-1.5 min-w-[44px] min-h-[44px]" data-edit-c="${c.id}" aria-label="${t('detail.edit')}">${icons.edit}</button>
          <button class="btn-danger p-1.5 min-w-[44px] min-h-[44px]" data-del-c="${c.id}" aria-label="${t('detail.delete')}">${icons.trash}</button>
        </div>
      `
      row.querySelector(`[data-edit-c="${c.id}"]`)?.addEventListener('click', () => {
        const { el, getData } = buildContactForm(c)
        const overlay = openModal({ title: t('detail.contact_edit_title'), content: el })
        el.querySelector('[data-save]')?.addEventListener('click', async () => {
          const data = getData()
          if (!data) { toast(t('form.field_required'), 'error'); return }
          try {
            const updated = await api.contacts.update(c.id, data)
            Object.assign(c, updated)
            overlay.remove()
            renderContacts(list)
            toast(t('detail.contact_edit_title'), 'success')
          } catch {
            toast(t('form.error'), 'error')
          }
        })
      })
      row.querySelector(`[data-del-c="${c.id}"]`)?.addEventListener('click', () => {
        const confirmEl = document.createElement('div')
        confirmEl.className = 'space-y-5'
        confirmEl.innerHTML = `
          <p class="text-sm text-muted">${t('detail.confirm_delete_contact')}</p>
          <div class="flex justify-end gap-2">
            <button data-cancel class="btn-ghost">${t('form.cancel')}</button>
            <button data-confirm class="btn-danger">${t('detail.delete')}</button>
          </div>
        `
        const overlay = openModal({ title: t('detail.confirm_delete_contact'), content: confirmEl })
        confirmEl.querySelector('[data-cancel]')?.addEventListener('click', () => overlay.remove())
        confirmEl.querySelector('[data-confirm]')?.addEventListener('click', async () => {
          try {
            await api.contacts.delete(c.id)
            const idx = list.findIndex(x => x.id === c.id)
            if (idx !== -1) list.splice(idx, 1)
            overlay.remove()
            renderContacts(list)
          } catch {
            toast(t('form.error'), 'error')
          }
        })
      })
      cList.appendChild(row)
    })
    contactsPanel.appendChild(cList)
  }

  renderContacts(app.contacts ?? [])

  // Timeline tab
  const timelinePanel = document.createElement('div')
  timelinePanel.className = 'p-5 space-y-3'
  if (!app.timeline_events?.length) {
    timelinePanel.innerHTML = `<p class="text-sm text-muted/60">${t('detail.no_timeline')}</p>`
  } else {
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
        <div>
          <p class="text-sm text-primary/80">${esc(translateTimelineEvent(e.event_type, e.description))}</p>
          <p class="text-xs text-muted/60 mt-0.5 tabular-nums">${new Date(e.created_at).toLocaleString(dateFmt)}</p>
        </div>
      `
      list.appendChild(row)
    })
    timelinePanel.appendChild(list)
  }

  // Assemble tabs
  const tabs = makeTabs([
    { id: 'notes', label: t('detail.tab_notes'), panel: notesPanel },
    { id: 'prep', label: t('detail.tab_prep'), panel: prepPanel },
    { id: 'offer', label: t('detail.tab_offer'), panel: offerPanel },
    { id: 'interviews', label: `${t('detail.tab_interviews')}${app.interviews?.length ? ` (${app.interviews.length})` : ''}`, panel: interviewsPanel },
    { id: 'contacts', label: `${t('detail.tab_contacts')}${app.contacts?.length ? ` (${app.contacts.length})` : ''}`, panel: contactsPanel },
    { id: 'timeline', label: t('detail.tab_timeline'), panel: timelinePanel },
  ])

  const tabCard = document.createElement('div')
  tabCard.className = 'card !p-0 flex-1 min-w-0 overflow-hidden'
  tabCard.appendChild(tabs)

  const sidebar = document.createElement('div')
  sidebar.className = 'lg:w-64 shrink-0 space-y-4'

  if (app.company_website || app.company_industry || app.company_size || app.company_location) {
    const companyCard = document.createElement('div')
    companyCard.className = 'card space-y-3'
    companyCard.innerHTML = `<h3 class="text-sm font-semibold text-primary/70">${t('detail.company_info')}</h3>`

    if (app.company_website && sanitizeUrl(app.company_website)) {
      const row = document.createElement('div')
      row.innerHTML = `
        <p class="text-xs text-muted mb-0.5 font-medium">${t('form.company_website')}</p>
        <a href="${esc(sanitizeUrl(app.company_website))}" target="_blank" rel="noopener noreferrer"
           class="text-sm text-accent hover:text-accent-hover flex items-center gap-1.5 transition-colors font-medium">
          ${icons.globe} ${safeHostname(app.company_website)}
        </a>
      `
      companyCard.appendChild(row)
    }
    if (app.company_industry) {
      const row = document.createElement('div')
      row.innerHTML = `<p class="text-xs text-muted mb-0.5 font-medium">${t('form.company_industry')}</p><p class="text-sm text-primary">${esc(app.company_industry)}</p>`
      companyCard.appendChild(row)
    }
    if (app.company_size) {
      const row = document.createElement('div')
      row.innerHTML = `<p class="text-xs text-muted mb-0.5 font-medium">${t('form.company_size')}</p><p class="text-sm text-primary">${esc(app.company_size)}</p>`
      companyCard.appendChild(row)
    }
    if (app.company_location) {
      const row = document.createElement('div')
      row.innerHTML = `<p class="text-xs text-muted mb-0.5 font-medium">${t('form.company_location')}</p><p class="text-sm text-primary">${esc(app.company_location)}</p>`
      companyCard.appendChild(row)
    }
    sidebar.appendChild(companyCard)
  }

  layout.appendChild(tabCard)
  if (sidebar.children.length) layout.appendChild(sidebar)
  content.appendChild(layout)

  actions.querySelector('#delete-btn')?.addEventListener('click', () => {
    const confirmEl = document.createElement('div')
    confirmEl.className = 'space-y-5'
    confirmEl.innerHTML = `
      <p class="text-sm text-muted">${t('detail.confirm_delete')}</p>
      <div class="flex justify-end gap-2">
        <button data-cancel class="btn-ghost">${t('form.cancel')}</button>
        <button data-confirm class="btn-danger">${t('detail.delete')}</button>
      </div>
    `
    const overlay = openModal({ title: t('detail.confirm_delete'), content: confirmEl })
    confirmEl.querySelector('[data-cancel]')?.addEventListener('click', () => overlay.remove())
    confirmEl.querySelector('[data-confirm]')?.addEventListener('click', async () => {
      try {
        await api.applications.delete(app.id)
        overlay.remove()
        navigate('/applications')
      } catch {
        toast(t('form.error'), 'error')
      }
    })
  })

  return createLayout(content)
}
