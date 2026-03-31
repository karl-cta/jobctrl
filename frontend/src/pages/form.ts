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

  const v = (field: keyof Application) => esc((existing?.[field] as string | undefined) ?? '')

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
        <h2 class="text-xs font-semibold text-muted uppercase tracking-wider">${t('form.company')}</h2>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">${t('form.company_name')} *</label>
            <input name="company_name" class="input" required value="${v('company_name')}" />
          </div>
          <div>
            <label class="label">${t('form.company_website')}</label>
            <input name="company_website" class="input" type="url" value="${v('company_website')}" />
          </div>
          <div>
            <label class="label">${t('form.company_industry')}</label>
            <input name="company_industry" class="input" value="${v('company_industry')}" />
          </div>
          <div>
            <label class="label">${t('form.company_size')}</label>
            <input name="company_size" class="input" placeholder="${t('form.company_size_placeholder')}" value="${v('company_size')}" />
          </div>
          <div>
            <label class="label">${t('form.company_location')}</label>
            <input name="company_location" class="input" value="${v('company_location')}" />
          </div>
        </div>
      </div>

      <div class="card space-y-4 relative overflow-hidden">
        <div class="absolute left-0 top-0 bottom-0 w-0.5 ${sectionColors.position}"></div>
        <h2 class="text-xs font-semibold text-muted uppercase tracking-wider">${t('form.position')}</h2>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">${t('form.job_title')} *</label>
            <input name="job_title" class="input" required value="${v('job_title')}" />
          </div>
          <div>
            <label class="label">${t('form.job_url')}</label>
            <input name="job_url" class="input" type="url" value="${v('job_url')}" />
          </div>
          <div>
            <label class="label">${t('form.contract_type')}</label>
            <select name="contract_type" class="select">
              ${['CDI', 'CDD', 'Freelance', 'Internship', 'Other'].map(ct =>
                `<option value="${ct}" ${v('contract_type') === ct ? 'selected' : ''}>${ct}</option>`
              ).join('')}
            </select>
          </div>
          <div>
            <label class="label">${t('form.work_mode')}</label>
            <select name="work_mode" class="select">
              ${['On-site', 'Hybrid', 'Remote'].map(m =>
                `<option value="${m}" ${v('work_mode') === m ? 'selected' : ''}>${m}</option>`
              ).join('')}
            </select>
          </div>
          <div>
            <label class="label">${t('form.location')}</label>
            <input name="location" class="input" value="${v('location')}" />
          </div>
          <div>
            <label class="label">${t('form.source')}</label>
            <input name="source" class="input" placeholder="${t('form.source_placeholder')}" value="${v('source')}" />
          </div>
        </div>
        <div>
          <label class="label">${t('form.description')}</label>
          <textarea name="job_description" class="input min-h-[100px]">${v('job_description')}</textarea>
        </div>
      </div>

      <div class="card space-y-4 relative overflow-hidden">
        <div class="absolute left-0 top-0 bottom-0 w-0.5 ${sectionColors.salary}"></div>
        <h2 class="text-xs font-semibold text-muted uppercase tracking-wider">${t('form.salary_status')}</h2>
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
          <div>
            <label class="label">${t('form.salary_min')}</label>
            <input name="salary_min" class="input" type="number" step="1000" value="${v('salary_min')}" />
          </div>
          <div>
            <label class="label">${t('form.salary_max')}</label>
            <input name="salary_max" class="input" type="number" step="1000" value="${v('salary_max')}" />
          </div>
          <div>
            <label class="label">${t('form.status')}</label>
            <select name="status" class="select">
              ${ALL_STATUSES.map(s =>
                `<option value="${s}" ${v('status') === s ? 'selected' : ''}>${statusLabel(s)}</option>`
              ).join('')}
            </select>
          </div>
          <div>
            <label class="label">${t('form.rating')}</label>
            <div class="flex gap-1 mt-1.5" id="star-picker" role="radiogroup" aria-label="${t('form.rating')}">
              ${[1,2,3,4,5].map(n => `
                <button type="button" data-star="${n}" role="radio" aria-checked="${Number(v('rating')) === n}"
                  class="star-btn text-2xl leading-none transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 rounded
                    ${Number(v('rating')) >= n ? 'text-amber-400' : 'text-surface-3 hover:text-amber-300'}"
                  aria-label="${n}">&#9733;</button>
              `).join('')}
            </div>
            <input type="hidden" name="rating" id="rating-input" value="${v('rating')}" />
          </div>
        </div>
        <div>
          <label class="label">${t('form.applied_at')}</label>
          <input name="applied_at" class="input" type="date" value="${v('applied_at') ? new Date(v('applied_at')).toISOString().slice(0, 10) : ''}" />
        </div>
      </div>

      <div class="card space-y-4 relative overflow-hidden">
        <div class="absolute left-0 top-0 bottom-0 w-0.5 ${sectionColors.notes}"></div>
        <h2 class="text-xs font-semibold text-muted uppercase tracking-wider">${t('form.notes_section')}</h2>
        <div>
          <label class="label">${t('form.notes')}</label>
          <textarea name="notes" class="input min-h-[80px]" placeholder="${t('form.notes_placeholder')}">${v('notes')}</textarea>
        </div>
        <div>
          <label class="label">${t('form.speech')}</label>
          <textarea name="speech" class="input min-h-[80px] font-mono text-sm" placeholder="${t('form.speech_placeholder')}">${v('speech')}</textarea>
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
    }
    starPicker.querySelectorAll<HTMLButtonElement>('.star-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        const n = Number(btn.dataset.star)
        const current = Number(ratingInput.value)
        updateStars(current === n ? 0 : n)
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
