import { api } from '../api'
import { createLayout } from '../components/layout'
import { navigate, setNavigationGuard, setNavigationCleanup } from '../router'
import { t } from '../i18n'
import { icons } from '../icons'
import { toast } from '../components/toast'
import { esc } from '../sanitize'
import { ALL_STATUSES, statusLabel, type Application } from '../types'
import { setupSourceAutocomplete } from '../components/source-autocomplete'

export async function FormPage(id?: string): Promise<HTMLElement> {
  const isEdit = Boolean(id)
  const existing = id ? await api.applications.get(id).catch(() => null) : null

  const v = (field: keyof Application) => esc(String(existing?.[field] ?? ''))

  const content = document.createElement('div')
  content.className = 'max-w-4xl mx-auto space-y-6 stagger'

  content.innerHTML = `
    <div class="flex items-center gap-3">
      <button id="back-btn" class="btn-ghost p-1.5" aria-label="${t('form.back')}">${icons.arrowLeft}</button>
      <h1 class="text-2xl font-bold text-primary tracking-tight">${isEdit ? t('form.title_edit') : t('form.title_new')}</h1>
    </div>

    <form id="app-form" class="space-y-5">
      ${!isEdit ? `<div class="card">
        <div class="flex gap-2 items-end">
          <div class="flex-1">
            <label for="f-extract-url" class="label">${t('form.extract_url')}</label>
            <input id="f-extract-url" class="input" type="url" placeholder="${t('form.extract_url_placeholder')}" />
          </div>
          <button type="button" id="extract-btn" class="btn-primary whitespace-nowrap mb-px">${icons.globe} ${t('form.extract_btn')}</button>
        </div>
      </div>` : ''}

      <div class="card space-y-4">
        <h2 class="text-xs font-semibold text-muted uppercase tracking-wider">${t('form.company')}</h2>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label for="f-company-name" class="label">${t('form.company_name')} *</label>
            <input id="f-company-name" name="company_name" class="input" required value="${v('company_name')}" />
          </div>
          <div>
            <label for="f-company-website" class="label">${t('form.company_website')}</label>
            <input id="f-company-website" name="company_website" class="input" type="url" value="${v('company_website')}" />
          </div>
          <div>
            <label for="f-company-industry" class="label">${t('form.company_industry')}</label>
            <input id="f-company-industry" name="company_industry" class="input" value="${v('company_industry')}" />
          </div>
          <div>
            <label for="f-company-size" class="label">${t('form.company_size')}</label>
            <input id="f-company-size" name="company_size" class="input" placeholder="${t('form.company_size_placeholder')}" value="${v('company_size')}" />
          </div>
          <div>
            <label for="f-company-location" class="label">${t('form.company_location')}</label>
            <input id="f-company-location" name="company_location" class="input" value="${v('company_location')}" />
          </div>
        </div>
      </div>

      <div class="card space-y-4">
        <h2 class="text-xs font-semibold text-muted uppercase tracking-wider">${t('form.position')}</h2>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label for="f-job-title" class="label">${t('form.job_title')} *</label>
            <input id="f-job-title" name="job_title" class="input" required value="${v('job_title')}" />
          </div>
          <div>
            <label for="f-job-url" class="label">${t('form.job_url')}</label>
            <input id="f-job-url" name="job_url" class="input" type="url" value="${v('job_url')}" />
          </div>
          <div>
            <label for="f-contract-type" class="label">${t('form.contract_type')}</label>
            <select id="f-contract-type" name="contract_type" class="select">
              ${['CDI', 'CDD', 'Freelance', 'Internship', 'Other'].map(ct =>
                `<option value="${ct}" ${v('contract_type') === ct ? 'selected' : ''}>${ct}</option>`
              ).join('')}
            </select>
          </div>
          <div id="duration-field" class="${v('contract_type') === 'CDD' ? '' : 'hidden'}">
            <label for="f-contract-duration" class="label">${t('form.contract_duration')}</label>
            <input id="f-contract-duration" name="contract_duration" class="input" type="number" min="1" placeholder="${t('form.contract_duration_placeholder')}" value="${v('contract_duration')}" />
          </div>
          <div>
            <label for="f-work-mode" class="label">${t('form.work_mode')}</label>
            <select id="f-work-mode" name="work_mode" class="select">
              ${['On-site', 'Hybrid', 'Remote'].map(m =>
                `<option value="${m}" ${v('work_mode') === m ? 'selected' : ''}>${m}</option>`
              ).join('')}
            </select>
          </div>
          <div>
            <label for="f-location" class="label">${t('form.location')}</label>
            <input id="f-location" name="location" class="input" value="${v('location')}" />
          </div>
          <div>
            <label for="f-source" class="label">${t('form.source')}</label>
            <input id="f-source" name="source" class="input" placeholder="${t('form.source_placeholder')}" value="${v('source')}" />
          </div>
        </div>
        <div>
          <label for="f-job-description" class="label">${t('form.description')}</label>
          <textarea id="f-job-description" name="job_description" class="input min-h-[100px]">${v('job_description')}</textarea>
        </div>
      </div>

      <div class="card space-y-4">
        <h2 class="text-xs font-semibold text-muted uppercase tracking-wider">${t('form.salary_status')}</h2>
        <div class="grid grid-cols-1 xs:grid-cols-2 gap-4">
          <div>
            <label for="f-salary" class="label">${t('form.salary')}</label>
            <input id="f-salary" name="salary" class="input" type="number" step="1000" value="${v('salary')}" />
          </div>
          <div>
            <label for="f-status" class="label">${t('form.status')}</label>
            <select id="f-status" name="status" class="select">
              ${ALL_STATUSES.map(s =>
                `<option value="${s}" ${v('status') === s ? 'selected' : ''}>${statusLabel(s)}</option>`
              ).join('')}
            </select>
          </div>
        </div>
        <div>
          <label class="label">${t('form.rating')}</label>
          <div class="flex items-center gap-3">
            <div class="flex" id="star-picker" role="radiogroup" aria-label="${t('form.rating')}">
              ${[1,2,3,4,5].map(n => `
                <button type="button" data-star="${n}" role="radio" aria-checked="${Number(v('rating')) === n}"
                  class="star-btn text-2xl leading-none transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 rounded p-1.5
                    ${Number(v('rating')) >= n ? 'text-amber-400' : 'text-surface-3 hover:text-amber-300'}"
                  aria-label="${t('form.rating_' + n)}">&#9733;</button>
              `).join('')}
            </div>
            <span id="rating-hint" class="text-sm text-muted transition-colors">${Number(v('rating')) ? t('form.rating_' + v('rating')) : ''}</span>
          </div>
          <input type="hidden" name="rating" id="rating-input" value="${v('rating')}" />
        </div>
        <div>
          <label class="label">${t('form.confidence')}</label>
          <div class="flex gap-1.5" id="confidence-picker" role="radiogroup" aria-label="${t('form.confidence')}">
            ${[1,2,3,4].map(n => {
              const active = Number(v('confidence')) === n
              const colors: Record<number, { active: string; idle: string }> = {
                1: { active: 'bg-stone-500 text-white dark:bg-stone-400 dark:text-stone-950', idle: 'text-stone-500 border-stone-300 dark:border-stone-600' },
                2: { active: 'bg-amber-500 text-white dark:bg-amber-400 dark:text-amber-950', idle: 'text-amber-600 border-amber-300 dark:text-amber-400 dark:border-amber-700' },
                3: { active: 'bg-emerald-500 text-white dark:bg-emerald-400 dark:text-emerald-950', idle: 'text-emerald-600 border-emerald-300 dark:text-emerald-400 dark:border-emerald-700' },
                4: { active: 'bg-teal-500 text-white dark:bg-teal-400 dark:text-teal-950', idle: 'text-teal-600 border-teal-300 dark:text-teal-400 dark:border-teal-700' },
              }
              const c = colors[n]
              return `<button type="button" data-confidence="${n}" role="radio" aria-checked="${active}"
                class="conf-btn rounded border px-3 py-1.5 text-xs font-semibold transition-colors duration-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50
                  ${active ? c.active + ' border-transparent' : c.idle + ' hover:bg-surface-2'}"
                aria-label="${t('form.confidence_' + n)}">${t('form.confidence_' + n)}</button>`
            }).join('')}
          </div>
          <input type="hidden" name="confidence" id="confidence-input" value="${v('confidence')}" />
        </div>
        <div>
          <label for="f-applied-at" class="label">${t('form.applied_at')}</label>
          <input id="f-applied-at" name="applied_at" class="input" type="date" value="${v('applied_at') ? new Date(v('applied_at')).toISOString().slice(0, 10) : ''}" />
        </div>
      </div>

      <div class="card space-y-4">
        <h2 class="text-xs font-semibold text-muted uppercase tracking-wider">${t('form.notes_section')}</h2>
        <div>
          <label for="f-notes" class="label">${t('form.notes')}</label>
          <textarea id="f-notes" name="notes" class="input min-h-[80px]" placeholder="${t('form.notes_placeholder')}">${v('notes')}</textarea>
        </div>
        <div>
          <label for="f-speech" class="label">${t('form.speech')}</label>
          <textarea id="f-speech" name="speech" class="input min-h-[80px]" placeholder="${t('form.speech_placeholder')}">${v('speech')}</textarea>
        </div>
      </div>

      <div class="flex gap-3 justify-end pb-8">
        <button type="button" id="cancel-btn" class="btn-ghost">${t('form.cancel')}</button>
        <button type="submit" class="btn-primary">${isEdit ? t('form.save') : t('form.create')}</button>
      </div>
    </form>
  `

  // Dirty tracking & navigation guard
  const form = content.querySelector('#app-form') as HTMLFormElement
  let formSubmitted = false
  const getSnapshot = () => {
    const fd = new FormData(form)
    return JSON.stringify(Object.fromEntries(fd))
  }
  const initialSnapshot = getSnapshot()
  const isDirty = () => !formSubmitted && getSnapshot() !== initialSnapshot

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
  })

  content.querySelector('#back-btn')?.addEventListener('click', () => navigate(isEdit ? '/applications/' + id : '/applications'))
  content.querySelector('#cancel-btn')?.addEventListener('click', () => navigate(isEdit ? '/applications/' + id : '/applications'))

  const sourceInput = content.querySelector('#f-source') as HTMLInputElement | null
  if (sourceInput) setupSourceAutocomplete(sourceInput, () => api.sources())

  // Show/hide duration field based on contract type
  const contractSelect = content.querySelector('#f-contract-type') as HTMLSelectElement | null
  const durationField = content.querySelector('#duration-field') as HTMLElement | null
  if (contractSelect && durationField) {
    contractSelect.addEventListener('change', () => {
      durationField.classList.toggle('hidden', contractSelect.value !== 'CDD')
      if (contractSelect.value !== 'CDD') {
        const input = durationField.querySelector('input') as HTMLInputElement
        if (input) input.value = ''
      }
    })
  }

  // URL extraction
  const extractBtn = content.querySelector('#extract-btn') as HTMLButtonElement | null
  const extractInput = content.querySelector('#f-extract-url') as HTMLInputElement | null
  if (extractBtn && extractInput) {
    // Track which fields were filled by extraction (not manually edited)
    const extractedFields = new Set<string>()

    const setField = (id: string, value: string | undefined) => {
      const el = content.querySelector('#' + id) as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null
      if (!el) return
      if (!value) {
        // Clear if this field was previously filled by extraction
        if (extractedFields.has(id)) {
          if (el.tagName === 'SELECT') {
            (el as HTMLSelectElement).selectedIndex = 0
          } else {
            el.value = ''
          }
          extractedFields.delete(id)
        }
        return
      }
      // Skip if field has user-typed content (not from a previous extraction)
      if (el.value && !extractedFields.has(id) && el.value !== 'CDI' && el.value !== 'Hybrid' && el.value !== 'Wishlist') return
      if (el.tagName === 'SELECT') {
        const option = el.querySelector(`option[value="${value}"]`) as HTMLOptionElement | null
        if (option) { el.value = value; extractedFields.add(id) }
      } else {
        el.value = value
        extractedFields.add(id)
      }
    }

    extractBtn.addEventListener('click', async () => {
      const url = extractInput.value.trim()
      if (!url) { extractInput.focus(); return }

      extractBtn.disabled = true
      extractBtn.innerHTML = `<span class="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin"></span> ${t('form.extracting')}`

      try {
        const data = await api.extract(url)
        // Count meaningful fields (not just source/url)
        const meaningful = [data.company_name, data.job_title, data.location, data.company_website].filter(Boolean).length

        // Fill (or clear previously extracted) fields
        setField('f-company-name', data.company_name)
        setField('f-company-website', data.company_website)
        setField('f-company-location', data.company_location)
        setField('f-job-title', data.job_title)
        setField('f-job-url', url)
        setField('f-job-description', data.job_description)
        setField('f-location', data.location)
        setField('f-source', data.source)
        setField('f-contract-type', data.contract_type)
        setField('f-work-mode', data.work_mode)
        setField('f-salary', data.salary ? String(data.salary) : undefined)

        if (meaningful === 0) {
          toast(t('form.extract_partial'), 'info')
        } else {
          toast(t('form.extract_success'), 'success')
        }
      } catch {
        toast(t('form.extract_error'), 'error')
      } finally {
        extractBtn.disabled = false
        extractBtn.innerHTML = `${icons.globe} ${t('form.extract_btn')}`
      }
    })

    // Allow Enter in the URL field to trigger extraction
    extractInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); extractBtn.click() }
    })
  }

  const starPicker = content.querySelector('#star-picker')
  const ratingInput = content.querySelector('#rating-input') as HTMLInputElement | null
  const ratingHint = content.querySelector('#rating-hint')
  if (starPicker && ratingInput) {
    const updateStars = (value: number) => {
      ratingInput.value = value > 0 ? String(value) : ''
      starPicker.querySelectorAll<HTMLButtonElement>('.star-btn').forEach(btn => {
        const n = Number(btn.dataset.star)
        const active = n <= value
        btn.className = btn.className
          .replace(/text-amber-\w+|text-surface-\d+|hover:text-amber-\w+/g, '')
          .trim()
        btn.classList.add(active ? 'text-amber-400' : 'text-surface-3', 'hover:text-amber-300')
        btn.setAttribute('aria-checked', String(value === n))
      })
      if (ratingHint) {
        ratingHint.textContent = value > 0 ? t('form.rating_' + value) : ''
      }
    }
    starPicker.querySelectorAll<HTMLButtonElement>('.star-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const n = Number(btn.dataset.star)
        const current = Number(ratingInput.value)
        const newVal = current === n ? 0 : n
        updateStars(newVal)
        if (newVal > 0) {
          btn.classList.remove('star-pop')
          void btn.offsetWidth // force reflow to restart animation
          btn.classList.add('star-pop')
        }
      })
    })
  }

  // Confidence picker
  const confPicker = content.querySelector('#confidence-picker')
  const confInput = content.querySelector('#confidence-input') as HTMLInputElement | null
  if (confPicker && confInput) {
    const confColors: Record<number, { active: string; idle: string }> = {
      1: { active: 'bg-stone-500 text-white dark:bg-stone-400 dark:text-stone-950 border-transparent', idle: 'text-stone-500 border-stone-300 dark:border-stone-600' },
      2: { active: 'bg-amber-500 text-white dark:bg-amber-400 dark:text-amber-950 border-transparent', idle: 'text-amber-600 border-amber-300 dark:text-amber-400 dark:border-amber-700' },
      3: { active: 'bg-emerald-500 text-white dark:bg-emerald-400 dark:text-emerald-950 border-transparent', idle: 'text-emerald-600 border-emerald-300 dark:text-emerald-400 dark:border-emerald-700' },
      4: { active: 'bg-teal-500 text-white dark:bg-teal-400 dark:text-teal-950 border-transparent', idle: 'text-teal-600 border-teal-300 dark:text-teal-400 dark:border-teal-700' },
    }
    confPicker.querySelectorAll<HTMLButtonElement>('.conf-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const n = Number(btn.dataset.confidence)
        const current = Number(confInput.value)
        const value = current === n ? 0 : n
        confInput.value = value > 0 ? String(value) : ''
        confPicker.querySelectorAll<HTMLButtonElement>('.conf-btn').forEach(b => {
          const bn = Number(b.dataset.confidence)
          const isActive = bn === value
          const c = confColors[bn]
          b.className = `conf-btn rounded border px-3 py-1.5 text-xs font-semibold transition-colors duration-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 ${isActive ? c.active : c.idle + ' hover:bg-surface-2'}`
          b.setAttribute('aria-checked', String(isActive))
        })
      })
    })
  }

  content.querySelector('#app-form')?.addEventListener('submit', async (e) => {
    e.preventDefault()
    const form = e.target as HTMLFormElement
    const submitBtn = form.querySelector<HTMLButtonElement>('[type="submit"]')
    if (submitBtn) {
      submitBtn.disabled = true
      submitBtn.innerHTML = `<span class="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin"></span> ${isEdit ? t('form.save') : t('form.create')}`
    }

    const data = Object.fromEntries(new FormData(form)) as Record<string, string>

    const payload: Partial<Application> = {
      company_name: data.company_name,
      company_website: data.company_website || undefined,
      company_industry: data.company_industry || undefined,
      company_size: data.company_size || undefined,
      company_location: data.company_location || undefined,
      job_title: data.job_title,
      job_url: data.job_url || undefined,
      job_description: data.job_description || undefined,
      contract_type: data.contract_type as Application['contract_type'],
      contract_duration: data.contract_duration ? Number(data.contract_duration) : undefined,
      work_mode: data.work_mode as Application['work_mode'],
      location: data.location || undefined,
      salary: data.salary ? Number(data.salary) : undefined,
      salary_currency: 'EUR',
      status: data.status as Application['status'],
      applied_at: data.applied_at ? data.applied_at + 'T00:00:00Z' : undefined,
      source: data.source || undefined,
      notes: data.notes || undefined,
      speech: data.speech || undefined,
      rating: data.rating ? Number(data.rating) : undefined,
      confidence: data.confidence ? Number(data.confidence) : undefined,
    }

    try {
      const result = isEdit
        ? await api.applications.update(id!, payload)
        : await api.applications.create(payload)

      toast(isEdit ? t('form.saved') : t('form.created'), 'success')
      formSubmitted = true
      navigate('/applications/' + result.id)
    } catch (err) {
      toast(err instanceof Error ? err.message : t('form.error'), 'error')
      if (submitBtn) {
        submitBtn.disabled = false
        submitBtn.textContent = isEdit ? t('form.save') : t('form.create')
      }
    }
  })

  return createLayout(content)
}
