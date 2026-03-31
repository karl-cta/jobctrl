import { api } from '../api'
import { createLayout } from '../components/layout'
import { t } from '../i18n'
import { esc } from '../sanitize'
import { statusLabel, ALL_STATUSES } from '../types'
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
    const rx = Math.min(barW / 2, 4)
    return [
      `<rect x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${barW.toFixed(1)}" height="${barH.toFixed(1)}" rx="${rx}" fill="url(#barGrad)" opacity="0.9"/>`,
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
        <div class="flex-1 bg-surface-2 rounded-full h-2 overflow-hidden">
          <div class="h-full rounded-full ${statusBarColors[s] || 'bg-accent'} transition-all duration-500" style="width:${barPct.toFixed(1)}%"></div>
        </div>
        <span class="w-14 shrink-0 text-xs text-muted tabular-nums font-medium">${count} <span class="opacity-50">${pct}%</span></span>
      </div>`
  }).join('')
}

export async function DashboardPage(): Promise<HTMLElement> {
  const stats = await api.stats().catch(() => null) as Stats | null

  const content = document.createElement('div')
  content.className = 'space-y-8 stagger'

  const total = stats?.total ?? 0
  const overTime = stats?.over_time ?? []

  const statCards = [
    { label: t('dashboard.total'), value: total, color: 'accent' },
    { label: t('dashboard.active_interviews'), value: stats?.active_interviews ?? 0, color: 'violet' },
    { label: t('dashboard.response_rate'), value: `${stats?.response_rate?.toFixed(0) ?? 0}%`, color: 'sky' },
    { label: t('dashboard.offers'), value: (stats?.by_status?.['Offer'] ?? 0) + (stats?.by_status?.['Accepted'] ?? 0), color: 'emerald' },
  ]

  const colorMap: Record<string, { dot: string; text: string }> = {
    accent: { dot: 'bg-accent', text: 'text-primary' },
    violet: { dot: 'bg-orange-500', text: 'text-orange-600 dark:text-orange-400' },
    sky: { dot: 'bg-amber-500', text: 'text-amber-600 dark:text-amber-400' },
    emerald: { dot: 'bg-emerald-500', text: 'text-emerald-600 dark:text-emerald-400' },
  }

  content.innerHTML = `
    <div>
      <h1 class="text-2xl font-bold text-primary tracking-tight">${t('dashboard.title')}</h1>
    </div>

    <div class="grid grid-cols-2 sm:grid-cols-4 gap-8">
      ${statCards.map(({ label, value, color }) => {
        const c = colorMap[color]
        return `
        <div>
          <p class="text-4xl font-bold ${c.text} tabular-nums font-mono tracking-tight">${value}</p>
          <p class="text-xs text-muted font-medium uppercase tracking-wider mt-2">${label}</p>
        </div>`
      }).join('')}
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-5 gap-5">
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

    <div>
      <h2 class="text-sm font-semibold text-primary mb-4">${t('dashboard.pipeline')}</h2>
      <div class="grid grid-cols-4 sm:grid-cols-8 gap-px bg-border rounded-lg overflow-hidden">
        ${ALL_STATUSES.map(s => {
          const count = stats?.by_status?.[s] ?? 0
          return `
          <div class="text-center py-4 bg-surface-1 hover:bg-surface-2/50 transition-colors">
            <div class="text-xl font-bold text-primary tabular-nums font-mono">${count}</div>
            <div class="text-[11px] text-muted mt-1.5 font-medium">${statusLabel(s)}</div>
          </div>`
        }).join('')}
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

  return createLayout(content)
}
