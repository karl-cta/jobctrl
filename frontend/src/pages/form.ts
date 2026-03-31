import { api } from '../api'
import { createLayout } from '../components/layout'
import { navigate } from '../router'
import { t } from '../i18n'
import { icons } from '../icons'
import { toast } from '../components/toast'
import { esc } from '../sanitize'
import { ALL_STATUSES, statusLabel, type Application } from '../types'

export async function FormPage(id?: string): Promise<HTMLElement> {
  const isEdit = Boolean(id)
  const existing = id ? await api.applications.get(id).catch(() => null) : null

  const v = (field: keyof Application) => esc(String(existing?.[field] ?? ''))

  const content = document.createElement('div')
  content.className = 'max-w-2xl space-y-6 stagger'

  const sectionColors: Record<string, string> = {
    company: 'bg-accent/40',
    position: 'bg-violet-500/40',
    salary: 'bg-emerald-500/40',
    notes: 'bg-sky-500/40',
  }

  content.innerHTML = `
    <div class="flex items-center gap-3">
      <button id="back-btn" class="btn-ghost p-1.5" aria-label="${t('form.back')}">${icons.arrowLeft}</button>
      <h1 class="text-2xl font-bold text-primary tracking-tight">${isEdit ? t('form.title_edit') : t('form.title_new')}</h1>
    </div>

    <form id="app-form" class="space-y-5">
      <div class="card space-y-4 relative overflow-hidden">
        <div class="absolute left-0 top-0 bottom-0 w-0.5 ${sectionColors.company}"></div>
        <h2 class="text-sm font-semibold text-primary/70">${t('form.company')}</h2>
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

      <div class="card space-y-4 relative overflow-hidden">
        <div class="absolute left-0 top-0 bottom-0 w-0.5 ${sectionColors.position}"></div>
        <h2 class="text-sm font-semibold text-primary/70">${t('form.position')}</h2>
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

      <div class="card space-y-4 relative overflow-hidden">
        <div class="absolute left-0 top-0 bottom-0 w-0.5 ${sectionColors.salary}"></div>
        <h2 class="text-sm font-semibold text-primary/70">${t('form.salary_status')}</h2>
        <div class="grid grid-cols-1 xs:grid-cols-3 gap-4">
          <div>
            <label for="f-salary-min" class="label">${t('form.salary_min')}</label>
            <input id="f-salary-min" name="salary_min" class="input" type="number" step="1000" value="${v('salary_min')}" />
          </div>
          <div>
            <label for="f-salary-max" class="label">${t('form.salary_max')}</label>
            <input id="f-salary-max" name="salary_max" class="input" type="number" step="1000" value="${v('salary_max')}" />
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
                class="conf-btn rounded-lg border px-3 py-1.5 text-xs font-semibold transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50
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

      <div class="card space-y-4 relative overflow-hidden">
        <div class="absolute left-0 top-0 bottom-0 w-0.5 ${sectionColors.notes}"></div>
        <h2 class="text-sm font-semibold text-primary/70">${t('form.notes_section')}</h2>
        <div>
          <label for="f-notes" class="label">${t('form.notes')}</label>
          <textarea id="f-notes" name="notes" class="input min-h-[80px]" placeholder="${t('form.notes_placeholder')}">${v('notes')}</textarea>
        </div>
        <div>
          <label for="f-speech" class="label">${t('form.speech')}</label>
          <textarea id="f-speech" name="speech" class="input min-h-[80px] font-mono text-sm" placeholder="${t('form.speech_placeholder')}">${v('speech')}</textarea>
        </div>
      </div>

      <div class="flex gap-3 justify-end pb-8">
        <button type="button" id="cancel-btn" class="btn-ghost">${t('form.cancel')}</button>
        <button type="submit" class="btn-primary">${isEdit ? t('form.save') : t('form.create')}</button>
      </div>
    </form>
  `

  content.querySelector('#back-btn')?.addEventListener('click', () => window.history.back())
  content.querySelector('#cancel-btn')?.addEventListener('click', () => window.history.back())

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
        updateStars(current === n ? 0 : n)
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
          b.className = `conf-btn rounded-lg border px-3 py-1.5 text-xs font-semibold transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 ${isActive ? c.active : c.idle + ' hover:bg-surface-2'}`
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
      work_mode: data.work_mode as Application['work_mode'],
      location: data.location || undefined,
      salary_min: data.salary_min ? Number(data.salary_min) : undefined,
      salary_max: data.salary_max ? Number(data.salary_max) : undefined,
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
