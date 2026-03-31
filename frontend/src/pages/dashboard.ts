import { api } from '../api'
import { createLayout } from '../components/layout'
import { t, tp } from '../i18n'
import { esc } from '../sanitize'
import { statusLabel, statusLabelCount, ALL_STATUSES } from '../types'
import type { Stats } from '../types'

function barChart(data: Array<{ period: string; count: number }>): string {
  if (!data.length) {
    return `<div class="flex items-center justify-center py-12 text-muted/60 text-sm">${t('dashboard.no_data')}</div>`
  }
  const W = 600
  const H = 140
  const padL = 28
  const padR = 8
  const padT = 12
  const padB = 32
  const chartW = W - padL - padR
  const chartH = H - padT - padB
  const maxCount = Math.max(...data.map(d => d.count), 1)
  const slotW = chartW / data.length
  const barW = Math.min(Math.max(slotW * 0.5, 4), 24)

  // Grid lines
  const gridVals = [Math.ceil(maxCount / 2), maxCount]
  const gridLines = gridVals.map(v => {
    const y = padT + chartH - (v / maxCount) * chartH
    return `<line x1="${padL}" y1="${y}" x2="${W - padR}" y2="${y}" stroke="currentColor" stroke-opacity="0.06" stroke-width="1" stroke-dasharray="4 4"/>`
      + `<text x="${padL - 6}" y="${y + 3}" text-anchor="end" font-size="10" fill="currentColor" opacity="0.35" font-family="Inter, system-ui">${v}</text>`
  }).join('')

  // Bars with rounded tops and gradient
  const bars = data.map((d, i) => {
    const barH = Math.max((d.count / maxCount) * chartH, d.count > 0 ? 3 : 0)
    const x = padL + i * slotW + (slotW - barW) / 2
    const y = padT + chartH - barH
    const label = d.period.includes('-') ? d.period.slice(d.period.lastIndexOf('-') + 1) : d.period
    const showLabel = data.length <= 12 || i % Math.ceil(data.length / 12) === 0
    return [
      `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${barW.toFixed(1)}" height="${barH.toFixed(1)}" rx="1" fill="url(#barGrad)" opacity="0.9"/>`,
      d.count > 0
        ? `<text x="${(x + barW / 2).toFixed(1)}" y="${(y - 5).toFixed(1)}" text-anchor="middle" font-size="10" fill="currentColor" opacity="0.6" font-family="Inter, system-ui" font-weight="500">${d.count}</text>`
        : '',
      showLabel
        ? `<text x="${(x + barW / 2).toFixed(1)}" y="${H - 8}" text-anchor="middle" font-size="10" fill="currentColor" opacity="0.4" font-family="Inter, system-ui">${label}</text>`
        : '',
    ].join('')
  }).join('')

  return `<svg viewBox="0 0 ${W} ${H}" width="100%" preserveAspectRatio="xMidYMid meet" role="img" aria-label="${t('dashboard.over_time')}">
    <defs>
      <linearGradient id="barGrad" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stop-color="rgb(var(--color-accent-hover))"/>
        <stop offset="100%" stop-color="rgb(var(--color-accent))"/>
      </linearGradient>
    </defs>
    <line x1="${padL}" y1="${padT + chartH}" x2="${W - padR}" y2="${padT + chartH}" stroke="currentColor" stroke-opacity="0.08" stroke-width="1"/>
    ${gridLines}
    ${bars}
  </svg>`
}

function statusChart(byStatus: Record<string, number>, total: number): string {
  if (!total) {
    return `<div class="flex items-center justify-center py-12 text-muted/60 text-sm">${t('dashboard.no_data')}</div>`
  }
  const active = ALL_STATUSES.filter(s => (byStatus[s] ?? 0) > 0)
  if (!active.length) {
    return `<div class="flex items-center justify-center py-12 text-muted/60 text-sm">${t('dashboard.no_data')}</div>`
  }

  const statusBarColors: Record<string, string> = {
    Wishlist: 'bg-stone-400 dark:bg-stone-500',
    Applied: 'bg-sky-500 dark:bg-sky-400',
    Screening: 'bg-amber-500 dark:bg-amber-400',
    Interviewing: 'bg-orange-500 dark:bg-orange-400',
    Offer: 'bg-emerald-500 dark:bg-emerald-400',
    Accepted: 'bg-teal-500 dark:bg-teal-400',
    Rejected: 'bg-rose-400 dark:bg-rose-400',
    Withdrawn: 'bg-stone-400 dark:bg-stone-500',
  }

  return active.map(s => {
    const count = byStatus[s] ?? 0
    const pct = ((count / total) * 100).toFixed(0)
    const barPct = (count / total) * 100
    return `
      <div class="flex items-center gap-3 group">
        <span class="w-24 shrink-0 text-xs text-right text-muted group-hover:text-primary transition-colors font-medium">${statusLabel(s)}</span>
        <div class="flex-1 bg-surface-2 h-1.5 overflow-hidden">
          <div class="h-full ${statusBarColors[s] || 'bg-accent'} transition-all duration-500 ease-out" style="width:0%" data-bar-width="${barPct.toFixed(1)}%"></div>
        </div>
        <span class="w-14 shrink-0 text-xs text-muted tabular-nums font-medium">${count} <span class="opacity-50">${pct}%</span></span>
      </div>`
  }).join('')
}

const prefersReducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches

function animateCounters(container: HTMLElement) {
  const skip = prefersReducedMotion()
  container.querySelectorAll<HTMLElement>('[data-count-to]').forEach(el => {
    const target = el.dataset.countTo!
    if (skip) { el.textContent = target; return }

    const isPercent = target.endsWith('%')
    const end = parseInt(target)
    if (!end || isNaN(end)) { el.textContent = target; return }

    const duration = 600
    const start = performance.now()
    el.textContent = isPercent ? '0%' : '0'

    function tick(now: number) {
      const elapsed = now - start
      const progress = Math.min(elapsed / duration, 1)
      const eased = 1 - Math.pow(1 - progress, 3)
      const current = Math.round(eased * end)
      el.textContent = isPercent ? `${current}%` : String(current)
      if (progress < 1) requestAnimationFrame(tick)
    }
    requestAnimationFrame(tick)
  })
}

export async function DashboardPage(): Promise<HTMLElement> {
  const stats = await api.stats().catch(() => null) as Stats | null

  const content = document.createElement('div')
  content.className = 'space-y-10 stagger'

  const total = stats?.total ?? 0
  const overTime = stats?.over_time ?? []

  const activeInterviews = stats?.active_interviews ?? 0
  const offers = (stats?.by_status?.['Offer'] ?? 0) + (stats?.by_status?.['Accepted'] ?? 0)

  const statCards = [
    { label: t('dashboard.total'), value: total, color: 'accent' },
    { label: tp('dashboard.active_interviews', activeInterviews), value: activeInterviews, color: 'violet' },
    { label: t('dashboard.response_rate'), value: `${stats?.response_rate?.toFixed(0) ?? 0}%`, color: 'sky' },
    { label: tp('dashboard.offers', offers), value: offers, color: 'emerald' },
  ]

  const colorMap: Record<string, { dot: string; text: string }> = {
    accent: { dot: 'bg-accent', text: 'text-primary' },
    violet: { dot: 'bg-orange-500', text: 'text-orange-600 dark:text-orange-400' },
    sky: { dot: 'bg-amber-500', text: 'text-amber-600 dark:text-amber-400' },
    emerald: { dot: 'bg-emerald-500', text: 'text-emerald-600 dark:text-emerald-400' },
  }

  const pipelineColors: Record<string, { bg: string; border: string; text: string; label: string; bar: string }> = {
    Wishlist: { bg: 'bg-stone-500/[0.06] dark:bg-stone-400/[0.08]', border: 'border-l-stone-400 dark:border-l-stone-500', text: 'text-stone-600 dark:text-stone-400', label: 'text-stone-500 dark:text-stone-500', bar: 'bg-stone-400 dark:bg-stone-500' },
    Applied: { bg: 'bg-sky-500/[0.07] dark:bg-sky-400/10', border: 'border-l-sky-500 dark:border-l-sky-400', text: 'text-sky-700 dark:text-sky-400', label: 'text-sky-600/80 dark:text-sky-400/70', bar: 'bg-sky-500 dark:bg-sky-400' },
    Screening: { bg: 'bg-amber-500/[0.07] dark:bg-amber-400/10', border: 'border-l-amber-500 dark:border-l-amber-400', text: 'text-amber-700 dark:text-amber-400', label: 'text-amber-600/80 dark:text-amber-400/70', bar: 'bg-amber-500 dark:bg-amber-400' },
    Interviewing: { bg: 'bg-orange-500/[0.07] dark:bg-orange-400/10', border: 'border-l-orange-500 dark:border-l-orange-400', text: 'text-orange-700 dark:text-orange-400', label: 'text-orange-600/80 dark:text-orange-400/70', bar: 'bg-orange-500 dark:bg-orange-400' },
    Offer: { bg: 'bg-emerald-500/[0.07] dark:bg-emerald-400/10', border: 'border-l-emerald-500 dark:border-l-emerald-400', text: 'text-emerald-700 dark:text-emerald-400', label: 'text-emerald-600/80 dark:text-emerald-400/70', bar: 'bg-emerald-500 dark:bg-emerald-400' },
    Accepted: { bg: 'bg-teal-500/[0.07] dark:bg-teal-400/10', border: 'border-l-teal-500 dark:border-l-teal-400', text: 'text-teal-700 dark:text-teal-400', label: 'text-teal-600/80 dark:text-teal-400/70', bar: 'bg-teal-500 dark:bg-teal-400' },
    Rejected: { bg: 'bg-rose-500/[0.07] dark:bg-rose-400/10', border: 'border-l-rose-400', text: 'text-rose-600 dark:text-rose-400', label: 'text-rose-500/80 dark:text-rose-400/70', bar: 'bg-rose-400' },
    Withdrawn: { bg: 'bg-stone-500/[0.06] dark:bg-stone-400/[0.08]', border: 'border-l-stone-400 dark:border-l-stone-500', text: 'text-stone-500 dark:text-stone-500', label: 'text-stone-400 dark:text-stone-500/70', bar: 'bg-stone-400 dark:bg-stone-500' },
  }

  content.innerHTML = `
    <div>
      <h1 class="text-2xl font-bold text-primary tracking-tight">${t('dashboard.title')}</h1>
    </div>

    <div class="grid grid-cols-2 sm:grid-cols-4 gap-10">
      ${statCards.map(({ label, value, color }) => {
        const c = colorMap[color]
        return `
        <div class="text-center">
          <p class="text-4xl font-bold ${c.text} tabular-nums font-mono tracking-tight" data-count-to="${value}">${value}</p>
          <p class="text-sm text-muted font-medium mt-2">${label}</p>
        </div>`
      }).join('')}
    </div>

    <div>
      <h2 class="text-sm font-semibold text-primary mb-5">${t('dashboard.pipeline')}</h2>
      ${total > 0 ? `
      <div class="flex h-2 overflow-hidden bg-surface-2 mb-5" role="img" aria-label="${t('dashboard.pipeline')}">
        ${ALL_STATUSES.map(s => {
          const count = stats?.by_status?.[s] ?? 0
          if (count === 0) return ''
          const pct = (count / total) * 100
          const c = pipelineColors[s]
          return `<div class="${c.bar} transition-all duration-700 ease-out" style="width:0%" data-bar-width="${pct.toFixed(1)}%"></div>`
        }).join('')}
      </div>` : ''}
      <div class="grid grid-cols-4 sm:grid-cols-8 gap-1.5">
        ${ALL_STATUSES.map(s => {
          const count = stats?.by_status?.[s] ?? 0
          const c = pipelineColors[s]
          return `
          <a href="/applications?status=${s}" data-link class="block text-center py-4 px-1 border-l-[3px] ${c.border} ${c.bg} backdrop-blur-sm rounded-sm no-underline ${count === 0 ? 'opacity-30 hover:opacity-60' : 'hover:brightness-110 cursor-pointer'}" style="transition: filter 0.1s ease, opacity 0.1s ease;">
            <div class="text-2xl font-bold ${c.text} tabular-nums font-mono leading-none">${count}</div>
            <div class="text-xs ${c.label} mt-2 font-semibold">${statusLabelCount(s, count)}</div>
          </a>`
        }).join('')}
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-5 gap-6">
      <div class="card lg:col-span-3">
        <h2 class="text-sm font-semibold text-primary mb-5">${t('dashboard.over_time')}</h2>
        ${barChart(overTime)}
      </div>

      <div class="card lg:col-span-2">
        <h2 class="text-sm font-semibold text-primary mb-5">${t('dashboard.status_distribution')}</h2>
        <div class="space-y-3">
          ${statusChart(stats?.by_status ?? {}, total)}
        </div>
      </div>
    </div>

    ${stats?.top_sources?.length ? `
    <div class="card">
      <h2 class="text-sm font-semibold text-primary mb-5">${t('dashboard.top_sources')}</h2>
      <div class="space-y-0 divide-y divide-border/60">
        ${stats.top_sources.map(s => `
          <div class="flex items-center justify-between py-2.5 group">
            <span class="text-sm text-primary/80 group-hover:text-primary transition-colors">${esc(s.source)}</span>
            <span class="text-sm font-semibold text-primary tabular-nums">${s.count}</span>
          </div>
        `).join('')}
      </div>
    </div>
    ` : ''}
  `

  requestAnimationFrame(() => {
    animateCounters(content)
    // Animate pipeline and distribution bars from 0 → target width
    content.querySelectorAll<HTMLElement>('[data-bar-width]').forEach(el => {
      el.style.width = el.dataset.barWidth!
    })
  })

  return createLayout(content)
}
