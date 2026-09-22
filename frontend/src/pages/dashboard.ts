import { api } from '../api'
import { createLayout } from '../components/layout'
import { navigate } from '../router'
import { t, tp, getDateLocale, translateTimelineEvent } from '../i18n'
import { esc } from '../sanitize'
import { toast } from '../components/toast'
import {
  statusLabel, STATUS_COLORS, ALL_STATUSES, interviewTypeLabel, interviewOutcomeLabel,
} from '../types'
import type {
  Stats, ActivityDay, ActivityItem, KPI, PeriodStats, WeeklyPoint,
  ActiveProcess, InterviewStep,
  ApplicationStatus, DashboardPeriod,
} from '../types'
import { faviconUrl, getSourceDomain } from '../job-boards'

// Panel system — each panel has a stable ID, a renderer, and a visibility test.
// Order is persisted in localStorage; drag & drop reorders them.

type PanelId =
  | 'kpis'
  | 'active'
  | 'follow-ups'
  | 'timeline'
  | 'funnel'
  | 'heatmap'
  | 'pipeline'
  | 'sources'
  | 'activity'

const DEFAULT_ORDER: PanelId[] = [
  'kpis',
  'active',
  'follow-ups',
  'timeline',
  'funnel',
  'heatmap',
  'pipeline',
  'sources',
  'activity',
]

/** Panels that always take the full grid width. */
const FULL_WIDTH: PanelId[] = ['kpis', 'active', 'follow-ups']

/** Sources and the feed share a grid row, so they share a row cap. Sources past
 *  `LIST_LIMIT` are rendered but collapsed behind a toggle. */
const FEED_LIMIT = 6
const LIST_LIMIT = 6

// v3: the panel set changed again ('active' joined the top of the dashboard),
// so old saved orders are dropped rather than migrated.
const ORDER_STORAGE_KEY = 'jobctrl:dashboard:order:v3'
const PERIOD_STORAGE_KEY = 'jobctrl:dashboard:period'

function loadOrder(): PanelId[] {
  try {
    const raw = localStorage.getItem(ORDER_STORAGE_KEY)
    if (!raw) return DEFAULT_ORDER
    const arr = JSON.parse(raw) as string[]
    if (!Array.isArray(arr)) return DEFAULT_ORDER
    const known = arr.filter((id): id is PanelId => (DEFAULT_ORDER as string[]).includes(id))
    for (const id of DEFAULT_ORDER) if (!known.includes(id)) known.push(id)
    return known
  } catch {
    return DEFAULT_ORDER
  }
}

function saveOrder(order: PanelId[]): void {
  try { localStorage.setItem(ORDER_STORAGE_KEY, JSON.stringify(order)) } catch { /* quota */ }
}

function resetOrder(): void {
  try { localStorage.removeItem(ORDER_STORAGE_KEY) } catch { /* ignore */ }
}

// Period selector

// Order drives the segmented control; 'all' leads because it is the default.
const PERIODS: DashboardPeriod[] = ['all', '30', '90', '365']
const DEFAULT_PERIOD: DashboardPeriod = 'all'

function loadPeriod(): DashboardPeriod {
  try {
    const raw = localStorage.getItem(PERIOD_STORAGE_KEY)
    if (raw && (PERIODS as string[]).includes(raw)) return raw as DashboardPeriod
  } catch { /* ignore */ }
  return DEFAULT_PERIOD
}

function savePeriod(p: DashboardPeriod): void {
  try { localStorage.setItem(PERIOD_STORAGE_KEY, p) } catch { /* quota */ }
}

const periodLabel = (p: DashboardPeriod): string => t(`dashboard.period_${p}`)

// Small formatting helpers

/** Plural key + `{n}` interpolation, e.g. "3 relances dues". */
function count(key: string, n: number): string {
  return tp(key, n).replace('{n}', String(n))
}

function interpolate(key: string, n: number | string): string {
  return t(key).replace('{n}', String(n))
}

/** `{placeholder}` interpolation for strings that carry more than one value. */
function fill(key: string, vars: Record<string, number | string>): string {
  return t(key).replace(/\{(\w+)\}/g, (m, k: string) =>
    k in vars ? String(vars[k]) : m)
}

/**
 * `YYYY-MM-DD` for a Date read in the *local* timezone. `toISOString()` would
 * shift the day back for any zone east of UTC (a local midnight is the previous
 * day in UTC), so every day key in this file goes through here.
 */
function localDayKey(d: Date): string {
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

// Charts (SVG) — sparkline, 12-week timeline, heatmap

/**
 * 60×22 sparkline. A series that is flat (or all zeros) draws a centred
 * horizontal line rather than nothing, so every KPI keeps its little curve.
 */
function sparkline(series: number[] | undefined, color: string): string {
  const raw = (series ?? []).filter(v => Number.isFinite(v))
  const pts = raw.length >= 2 ? raw : [raw[0] ?? 0, raw[0] ?? 0]
  const W = 60
  const H = 22
  const pad = 2
  const min = Math.min(...pts)
  const max = Math.max(...pts)
  const span = max - min
  const stepX = (W - pad * 2) / (pts.length - 1)
  const coords = pts.map((v, i) => {
    const x = pad + i * stepX
    const y = span === 0 ? H / 2 : H - pad - ((v - min) / span) * (H - pad * 2)
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })
  return `<svg class="kpi-spark" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}" fill="none" role="img" aria-label="${t('dashboard.kpi_spark')}">
    <polyline points="${coords.join(' ')}" fill="none" stroke="${color}" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
  </svg>`
}

/**
 * Grouped bar chart over the 12 last weeks: applications sent vs replies.
 * The SVG carries only the geometry (`preserveAspectRatio="none"` so bars fill
 * the panel at any width, strokes kept crisp with `vector-effect`); axis labels
 * are HTML so they stay legible down to 320px.
 */
function timelineChart(weekly: WeeklyPoint[]): string {
  const weeks = weekly.slice(-12)
  if (!weeks.length) return `<div class="panel-empty">${t('dashboard.timeline_empty')}</div>`

  const peak = Math.max(...weeks.map(w => Math.max(w.sent, w.replies)), 0)
  if (peak === 0) return `<div class="panel-empty">${t('dashboard.timeline_empty')}</div>`

  const gridMax = Math.max(3, Math.ceil(peak / 3) * 3)
  const n = weeks.length
  const SLOT = 10
  const H = 100
  const VW = n * SLOT

  const grid = [0, 1, 2, 3].map(k => {
    const y = ((H / 3) * k).toFixed(2)
    return `<line x1="0" y1="${y}" x2="${VW}" y2="${y}" stroke="currentColor" stroke-opacity="0.12" stroke-width="1" vector-effect="non-scaling-stroke"/>`
  }).join('')

  const bars = weeks.map((w, i) => {
    const base = i * SLOT
    const hs = (w.sent / gridMax) * H
    const hr = (w.replies / gridMax) * H
    const isLast = i === n - 1
    return [
      w.sent > 0
        ? `<rect x="${(base + 1.4).toFixed(2)}" y="${(H - hs).toFixed(2)}" width="3.4" height="${hs.toFixed(2)}" rx="0.6" fill="rgb(var(--chart-sent))" opacity="${isLast ? '1' : '0.85'}"/>`
        : '',
      w.replies > 0
        ? `<rect x="${(base + 5.2).toFixed(2)}" y="${(H - hr).toFixed(2)}" width="3.4" height="${hr.toFixed(2)}" rx="0.6" fill="rgb(var(--chart-replies))"/>`
        : '',
    ].join('')
  }).join('')

  const markerX = ((n - 1) * SLOT + 0.4).toFixed(2)
  const marker = `<line x1="${markerX}" y1="0" x2="${markerX}" y2="${H}" stroke="rgb(var(--chart-sent))" stroke-dasharray="2 3" stroke-width="1" opacity="0.45" vector-effect="non-scaling-stroke"/>`

  const yLabels = [3, 2, 1, 0].map(k => {
    const v = Math.round((gridMax / 3) * k)
    const top = (((3 - k) / 3) * 100).toFixed(2)
    return `<span style="top:${top}%">${v}</span>`
  }).join('')

  // Every other Monday as a real date, not a relative "W-11" offset.
  const loc = getDateLocale()
  const xLabels = weeks.map((w, i) => {
    const show = (n - 1 - i) % 2 === 1
    if (!show) return '<span></span>'
    const d = new Date(w.week_start + 'T00:00:00')
    return `<span>${esc(d.toLocaleDateString(loc, { day: 'numeric', month: 'short' }))}</span>`
  }).join('')

  // One invisible full-height slot per week, so empty weeks are hoverable too.
  // Copy is prepared here, where the locale and week dates are already in hand.
  const hits = weeks.map(w => {
    const from = new Date(w.week_start + 'T00:00:00')
    const to = new Date(from)
    to.setDate(from.getDate() + 6)
    const fmt = (d: Date) => d.toLocaleDateString(loc, { day: 'numeric', month: 'short' })
    const title = fill('dashboard.timeline_week_of', { from: fmt(from), to: fmt(to) })
    return `<button type="button" class="tl-hit"
      data-tip-title="${esc(title)}"
      data-tip-sent="${esc(count('dashboard.timeline_tip_sent', w.sent))}"
      data-tip-replies="${esc(count('dashboard.timeline_tip_replies', w.replies))}"
      aria-label="${esc(title)}"></button>`
  }).join('')

  const totalSent = weeks.reduce((a, w) => a + w.sent, 0)
  const totalReplies = weeks.reduce((a, w) => a + w.replies, 0)
  const label = `${t('dashboard.timeline_title')} — ${totalSent} ${t('dashboard.timeline_sent')}, ${totalReplies} ${t('dashboard.timeline_replies')}`

  return `
    <div class="tl-chart">
      <div class="tl-now">${t('dashboard.timeline_this_week')}</div>
      <div class="tl-plot">
        <div class="tl-y" aria-hidden="true">${yLabels}</div>
        <svg class="tl-svg" viewBox="0 0 ${VW} ${H}" preserveAspectRatio="none" role="img" aria-label="${esc(label)}">
          ${grid}
          ${bars}
          ${marker}
        </svg>
        <div class="tl-hits" style="grid-template-columns: repeat(${n}, 1fr)">${hits}</div>
      </div>
      <div class="tl-tip" hidden aria-hidden="true"></div>
      <div class="tl-x" style="grid-template-columns: repeat(${n}, 1fr)" aria-hidden="true">${xLabels}</div>
    </div>
  `
}

function heatmapChart(days: ActivityDay[]): string {
  // Build a 26-week × 7-day grid ending today. Locale-independent: 7 rows (Mon-Sun), 26 cols.
  const WEEKS = 26
  const today = new Date()
  today.setHours(0, 0, 0, 0)

  const dow = (today.getDay() + 6) % 7 // 0=Mon, 6=Sun
  const start = new Date(today)
  start.setDate(start.getDate() - dow - (WEEKS - 1) * 7)

  const byDate = new Map<string, number>()
  for (const d of days) byDate.set(d.date, d.count)

  const max = Math.max(1, ...days.map(d => d.count))

  const level = (c: number): number => {
    if (c === 0) return 0
    const r = c / max
    if (r > 0.75) return 4
    if (r > 0.5) return 3
    if (r > 0.25) return 2
    return 1
  }

  let cells = ''
  for (let row = 0; row < 7; row++) {
    for (let col = 0; col < WEEKS; col++) {
      const d = new Date(start)
      d.setDate(start.getDate() + col * 7 + row)
      if (d > today) { cells += `<i aria-hidden="true" class="opacity-0"></i>`; continue }
      const iso = localDayKey(d)
      const c = byDate.get(iso) ?? 0
      const lvl = level(c)
      const label = `${iso} · ${c} ${c === 1 ? t('dashboard.heatmap_event_one') : t('dashboard.heatmap_event_other')}`
      cells += `<i data-l="${lvl}" data-day="${iso}" role="button" tabindex="0" title="${esc(label)}" aria-label="${esc(label)}"></i>`
    }
  }

  const monthLabels: Array<{ col: number; label: string }> = []
  let lastMonth = -1
  let lastLabelCol = -Infinity
  for (let col = 0; col < WEEKS; col++) {
    const d = new Date(start)
    d.setDate(start.getDate() + col * 7)
    if (d.getMonth() !== lastMonth) {
      lastMonth = d.getMonth()
      // A label needs ~3 columns of room; skip the month when the previous label is too close.
      if (col - lastLabelCol >= 3) {
        monthLabels.push({ col, label: d.toLocaleDateString(getDateLocale(), { month: 'short' }) })
        lastLabelCol = col
      }
    }
  }

  const monthRow = monthLabels.map(m => `
    <span class="text-[11px] font-medium text-muted/70 font-caption" style="grid-column: ${m.col + 1} / span 1">${m.label}</span>
  `).join('')

  return `
    <div class="heatmap-wrap">
      <div class="heatmap-months" style="grid-template-columns: repeat(${WEEKS}, 1fr)">${monthRow}</div>
      <div class="heatmap" role="img" aria-label="${t('dashboard.heatmap_title')}" style="grid-template-columns: repeat(${WEEKS}, 1fr); grid-template-rows: repeat(7, 1fr)">${cells}</div>
      <div class="heatmap-legend">
        <span class="text-[11px] font-medium text-muted/70 font-caption">${t('dashboard.heatmap_less')}</span>
        <i></i>
        <i data-l="1"></i>
        <i data-l="2"></i>
        <i data-l="3"></i>
        <i data-l="4"></i>
        <span class="text-[11px] font-medium text-muted/70 font-caption">${t('dashboard.heatmap_more')}</span>
      </div>
      <div class="heatmap-day" hidden></div>
    </div>
  `
}

// Animations

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

// Panel renderers

const pipelineColors: Record<string, { bar: string; segment: string }> = {
  // `bar`: legend dot — same palette as the rest of the app.
  // `segment`: stacked-bar slice, one step darker in light mode so the label
  //            (rgb(var(--color-surface))) stays readable inside it.
  Wishlist:     { bar: 'bg-stone-400 dark:bg-stone-500',     segment: 'bg-stone-500 dark:bg-stone-400' },
  Applied:      { bar: 'bg-sky-500 dark:bg-sky-400',         segment: 'bg-sky-600 dark:bg-sky-400' },
  Screening:    { bar: 'bg-amber-500 dark:bg-amber-400',     segment: 'bg-amber-600 dark:bg-amber-400' },
  Interviewing: { bar: 'bg-orange-500 dark:bg-orange-400',   segment: 'bg-orange-600 dark:bg-orange-400' },
  Offer:        { bar: 'bg-emerald-500 dark:bg-emerald-400', segment: 'bg-emerald-600 dark:bg-emerald-400' },
  Accepted:     { bar: 'bg-teal-500 dark:bg-teal-400',       segment: 'bg-teal-600 dark:bg-teal-400' },
  Rejected:     { bar: 'bg-rose-400 dark:bg-rose-400',       segment: 'bg-rose-500 dark:bg-rose-400' },
  NoReply:      { bar: 'bg-indigo-400 dark:bg-indigo-400',   segment: 'bg-indigo-500 dark:bg-indigo-400' },
}

const eventBadgeColors: Record<string, string> = {
  created: 'bg-sky-500/15 text-sky-700 dark:text-sky-400',
  status_change: 'bg-amber-500/15 text-amber-700 dark:text-amber-400',
  interview_added: 'bg-orange-500/15 text-orange-700 dark:text-orange-400',
  interview_held: 'bg-violet-500/15 text-violet-700 dark:text-violet-400',
  interview_deleted: 'bg-stone-500/15 text-stone-600 dark:text-stone-400',
  contact_added: 'bg-violet-500/15 text-violet-700 dark:text-violet-400',
  contact_deleted: 'bg-stone-500/15 text-stone-600 dark:text-stone-400',
}

function eventLabel(type: string): string {
  const key = `dashboard.event.${type}`
  const translated = t(key)
  return translated === key ? type.replace('_', ' ') : translated
}

/** `interview_held` descriptions arrive as "<Type> · round <n>"; only the
 *  leading type token is ours to localise, the rest passes through. */
function heldDescription(description: string): string {
  const sep = description.indexOf(' · ')
  if (sep === -1) return interviewTypeLabel(description)
  return interviewTypeLabel(description.slice(0, sep)) + description.slice(sep)
}

/** "auj. 10:24" · "hier" · "14 avr." */
function feedTime(raw: string): string {
  const d = new Date(raw.replace(' ', 'T'))
  if (isNaN(d.getTime())) return ''
  const loc = getDateLocale()
  const startOfToday = new Date()
  startOfToday.setHours(0, 0, 0, 0)
  const startOfDay = new Date(d)
  startOfDay.setHours(0, 0, 0, 0)
  const diffDays = Math.round((startOfToday.getTime() - startOfDay.getTime()) / 86400000)
  if (diffDays === 0) {
    return `${t('dashboard.time_today')} ${d.toLocaleTimeString(loc, { hour: '2-digit', minute: '2-digit' })}`
  }
  if (diffDays === 1) return t('dashboard.time_yesterday')
  return d.toLocaleDateString(loc, { day: 'numeric', month: 'short' })
}

/** Panel header: title, sub, optional right-hand slot. */
function panelHead(title: string, sub?: string, right?: string): string {
  return `
    <div class="panel-head">
      <div>
        <h2 class="panel-title">${title}</h2>
        ${sub ? `<div class="panel-sub">${sub}</div>` : ''}
      </div>
      ${right ? `<div class="panel-head-right">${right}</div>` : ''}
    </div>`
}

/** Drag handle, revealed by `.dash-reorder-mode`. */
function panelHandle(): string {
  return `
    <div class="dash-panel-handle" aria-hidden="true">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" class="w-4 h-4">
        <path d="M7 4a1 1 0 110 2 1 1 0 010-2zm6 0a1 1 0 110 2 1 1 0 010-2zM7 9a1 1 0 110 2 1 1 0 010-2zm6 0a1 1 0 110 2 1 1 0 010-2zM7 14a1 1 0 110 2 1 1 0 010-2zm6 0a1 1 0 110 2 1 1 0 010-2z"/>
      </svg>
    </div>`
}

function panelOpen(id: PanelId, label: string, card = true): string {
  const span = FULL_WIDTH.includes(id) ? ' data-span="full"' : ''
  const cls = card ? 'dash-panel dash-panel-card' : 'dash-panel'
  return `<section class="${cls}" data-panel-id="${id}"${span} draggable="true" aria-label="${label}">${panelHandle()}`
}

// KPI tiles

interface KpiOptions {
  /** Where the tile leads — the matching filtered list. */
  href: string
  /** Delta unit — "pts" for the response rate, the period token otherwise. */
  deltaUnit: string
  /** True when a rise is not good news (rejections): delta stays muted. */
  neutral?: boolean
}

function kpiTile(label: string, kpi: KPI | undefined, colorVar: string, opts: KpiOptions): string {
  const value = Math.round(kpi?.value ?? 0)
  const prev = kpi?.prev
  const color = `rgb(var(--${colorVar}))`

  let delta = ''
  if (prev !== null && prev !== undefined) {
    const diff = Math.round((kpi?.value ?? 0) - prev)
    if (diff === 0) {
      delta = `<div class="kpi-delta flat">${t('dashboard.kpi_stable')}</div>`
    } else {
      const up = diff > 0
      const tone = opts.neutral ? 'flat' : (up ? 'up' : 'down')
      const arrow = up ? '▲' : '▼'
      delta = `<div class="kpi-delta ${tone}">${arrow} ${up ? '+' : '-'}${Math.abs(diff)} ${opts.deltaUnit}</div>`
    }
  }

  const aria = t('dashboard.kpi_link').replace('{label}', label)

  return `
    <a href="${opts.href}" data-link class="kpi" aria-label="${esc(aria)}">
      <div class="kpi-label">${label}</div>
      <div class="kpi-value"><span data-count-to="${value}">${value}</span></div>
      ${delta}
      ${sparkline(kpi?.series, color)}
    </a>`
}

/** "· 90j" — the delta's reference window, echoing the segmented selector. */
function periodToken(days: number | undefined): string {
  if (days === 30) return `· ${periodLabel('30')}`
  if (days === 90) return `· ${periodLabel('90')}`
  if (days === 365) return `· ${periodLabel('365')}`
  return ''
}

function kpisPanel(period: PeriodStats | undefined): string {
  const token = periodToken(period?.days)
  const tiles = [
    kpiTile(t('dashboard.kpi_sent'), period?.sent, 'chart-sent', { href: '/applications', deltaUnit: token }),
    kpiTile(t('dashboard.kpi_responded'), period?.responded, 'chart-replies', { href: '/applications?has_reply=1', deltaUnit: token }),
    kpiTile(t('dashboard.kpi_no_reply'), period?.no_reply, 'chart-no-reply', { href: '/applications?status=NoReply', deltaUnit: token, neutral: true }),
    kpiTile(t('dashboard.kpi_interviews'), period?.interviews, 'chart-interviews', { href: '/applications?has_interviews=1', deltaUnit: token }),
    kpiTile(t('dashboard.kpi_rejected'), period?.rejected, 'chart-neutral', { href: '/applications?status=Rejected', deltaUnit: token, neutral: true }),
    kpiTile(t('dashboard.kpi_offers'), period?.offers, 'chart-offers', { href: '/applications?status=Offer', deltaUnit: token }),
  ].join('')

  return `
    ${panelOpen('kpis', t('dashboard.panel_kpis'), false)}
      <div class="kpis">${tiles}</div>
    </section>`
}

// Funnel

function funnelPanel(period: PeriodStats | undefined): string {
  const periodText = !period || period.days === 0
    ? t('dashboard.funnel_all_time')
    : interpolate('dashboard.funnel_last_days', period.days)

  const f = period?.funnel
  const sent = f?.sent ?? 0
  const responded = f?.responded ?? 0
  const sub = fill('dashboard.funnel_sub', { sent, period: periodText })

  const rows: Array<{ label: string; value: number; color: string }> = [
    { label: t('dashboard.funnel_responded'), value: responded, color: 'chart-replies' },
    { label: t('dashboard.funnel_interviewing'), value: f?.interviewing ?? 0, color: 'chart-interviews' },
    { label: t('dashboard.funnel_offer'), value: f?.offers ?? 0, color: 'chart-offers' },
    { label: t('dashboard.funnel_accepted'), value: f?.accepted ?? 0, color: 'chart-accepted' },
  ]

  // Same period as the funnel, so these add up against `sent`. Clamped because
  // the three are computed independently server-side.
  const rejected = Math.round(period?.rejected?.value ?? 0)
  const noReply = Math.round(period?.no_reply?.value ?? 0)
  const pending = Math.max(0, sent - responded - noReply)

  const body = sent === 0
    ? `<div class="panel-empty">${t('dashboard.no_data')}</div>`
    : `<div class="funnel">${rows.map(r => {
        const pct = (r.value / sent) * 100
        const width = Math.max(pct, r.value > 0 ? 3 : 0)
        return `
          <div class="fun-row">
            <div class="fun-label">${r.label}</div>
            <div class="fun-bar-wrap">
              <div class="fun-bar" style="width:0%;background:rgb(var(--${r.color}))" data-bar-width="${width.toFixed(1)}%"></div>
            </div>
            <div class="fun-count">${r.value} · ${pct.toFixed(0)} %</div>
          </div>`
      }).join('')}
      <div class="fun-rest">${fill('dashboard.funnel_rest', { rejected, no_reply: noReply, pending })}</div>
    </div>`

  return `
    ${panelOpen('funnel', t('dashboard.funnel'))}
      ${panelHead(t('dashboard.funnel'), sub)}
      ${body}
    </section>`
}

// Status breakdown

function pipelinePanel(byStatus: Partial<Record<ApplicationStatus, number>>, total: number): string {
  // The bar skips empty statuses; the legend below lists them all.
  const active = ALL_STATUSES.filter(s => (byStatus[s] ?? 0) > 0)

  const body = total === 0 || !active.length
    ? `<div class="panel-empty">${t('dashboard.no_data')}</div>`
    : `
      <div class="pipeline-bar" role="img" aria-label="${t('dashboard.status_breakdown')}">
        ${active.map(s => {
          const c = byStatus[s] ?? 0
          const pct = (c / total) * 100
          return `<span class="${pipelineColors[s].segment}" style="width:0%" data-bar-width="${pct.toFixed(1)}%">${pct >= 22 ? esc(statusLabel(s)) : ''}</span>`
        }).join('')}
      </div>
      <div class="pipeline-list">
        ${ALL_STATUSES.map(s => {
          const c = byStatus[s] ?? 0
          return `
          <a href="/applications?status=${s}" data-link${c === 0 ? ' class="pipeline-zero"' : ''}>
            <i class="${pipelineColors[s].bar}"></i>${esc(statusLabel(s))}<b>${c}</b>
          </a>`
        }).join('')}
      </div>`

  return `
    ${panelOpen('pipeline', t('dashboard.status_breakdown'))}
      ${panelHead(t('dashboard.status_breakdown'), interpolate('dashboard.status_breakdown_sub', total))}
      ${body}
    </section>`
}

// Top sources

function sourcesPanel(sources: Array<{ source: string; count: number }>): string {
  const rows = sources.map((src, i) => {
    const domain = getSourceDomain(src.source)
    const favicon = domain
      ? `<img src="${faviconUrl(domain)}" width="16" height="16" alt="" class="source-favicon" onerror="this.style.display='none'" />`
      : ''
    return `
      <a href="/applications?source=${encodeURIComponent(src.source)}" data-link class="src-row${i >= LIST_LIMIT ? ' src-extra' : ''}">
        ${favicon}<span class="src-name">${esc(src.source)}</span>
        <span class="src-count">${src.count}</span>
      </a>`
  }).join('')

  const extra = sources.length - LIST_LIMIT
  const toggle = extra > 0
    ? `<button type="button" class="src-more" data-src-more="${extra}" aria-expanded="false">${interpolate('dashboard.sources_more', extra)}</button>`
    : ''

  return `
    ${panelOpen('sources', t('dashboard.sources_title'))}
      ${panelHead(t('dashboard.sources_title'), t('dashboard.sources_sub'))}
      <div class="src-list">${rows}</div>
      ${toggle}
    </section>`
}

// Active processes

/** `at` arrives as a UTC `2006-01-02 15:04:05` string. */
function parseUTC(at: string): Date {
  return new Date(at.replace(' ', 'T') + 'Z')
}

/** "21 sept." — the interview day in the reader's own timezone. */
function stepDate(at: string): string {
  const d = parseUTC(at)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleDateString(getDateLocale(), { day: 'numeric', month: 'short' })
}

/** "21 sept. 14:30" — used for a scheduled interview, where the hour matters. */
function stepDateTime(at: string): string {
  const d = parseUTC(at)
  if (isNaN(d.getTime())) return ''
  const loc = getDateLocale()
  const day = d.toLocaleDateString(loc, { day: 'numeric', month: 'short' })
  return `${day} ${d.toLocaleTimeString(loc, { hour: '2-digit', minute: '2-digit' })}`
}

/** The one-line "where this process stands" caption. */
function stepLine(p: ActiveProcess): string {
  const next: InterviewStep | null = p.next_interview
  if (next) {
    const step = [interviewTypeLabel(next.type), stepDateTime(next.at)].filter(Boolean).join(' · ')
    return t('dashboard.active_next').replace('{step}', step)
  }
  const last: InterviewStep | null = p.last_interview
  if (last) {
    // No recorded outcome reads as still pending, which is the informative case.
    const outcome = interviewOutcomeLabel(last.outcome || 'Pending')
    return [interviewTypeLabel(last.type), stepDate(last.at), outcome].filter(Boolean).join(' · ')
  }
  return t('dashboard.active_no_interview')
}

/** Silence counter — muted under a week, amber up to two, rose beyond. */
function silenceTone(days: number): string {
  if (days >= 14) return 'rose'
  if (days >= 7) return 'amber'
  return ''
}

function activePanel(processes: ActiveProcess[]): string {
  const rows = processes.map(p => {
    const badge = STATUS_COLORS[p.status] ?? ''
    const days = p.silent_days
    const silence = days === 0
      ? t('dashboard.active_silent_today')
      : t('dashboard.active_silent').replace('{n}', String(days))
    const tone = silenceTone(days)
    return `
      <a href="/applications/${p.id}" data-link class="ap-row">
        <span class="ap-head">
          <b>${esc(p.company_name)}</b>
          <span class="ap-job">${esc(p.job_title)}</span>
          <span class="badge ${badge} ap-badge">${esc(statusLabel(p.status))}</span>
        </span>
        <span class="ap-step">${esc(stepLine(p))}</span>
        <span class="ap-silence${tone ? ' ' + tone : ''}">${esc(silence)}</span>
      </a>`
  }).join('')

  return `
    ${panelOpen('active', t('dashboard.active_title'))}
      ${panelHead(t('dashboard.active_title'), count('dashboard.active_sub', processes.length))}
      <div class="ap-list">${rows}</div>
    </section>`
}

// Activity feed

function activityFeed(items: ActivityItem[]): string {
  return `
    <div class="feed">
      ${items.map(it => {
        const badge = eventBadgeColors[it.event_type] ?? 'bg-stone-500/15 text-stone-600 dark:text-stone-400'
        return `
          <a href="/applications/${it.application_id}" data-link class="feed-item">
            <div class="feed-time">${esc(feedTime(it.time))}</div>
            <div class="feed-what">
              <span class="feed-tag ${badge}">${esc(eventLabel(it.event_type))}</span><b>${esc(it.company_name)}</b> — ${esc(it.event_type === 'interview_held'
                ? heldDescription(it.description)
                : translateTimelineEvent(it.event_type, it.description))}
            </div>
          </a>`
      }).join('')}
    </div>`
}

// Panel dispatch

function renderPanel(id: PanelId, stats: Stats | null): string {
  const total = stats?.total ?? 0

  switch (id) {
    case 'kpis':
      return kpisPanel(stats?.period)

    case 'active': {
      const processes = stats?.active_processes ?? []
      if (!processes.length) return ''
      return activePanel(processes)
    }

    case 'follow-ups': {
      if (!stats?.follow_ups?.length) return ''
      return `
        <section class="dash-panel dash-panel-card border-amber-500/20 dark:border-amber-400/15" data-panel-id="follow-ups" data-span="full" draggable="true" aria-label="${t('dashboard.follow_ups_title')}">
          ${panelHandle()}
          <div class="flex items-start gap-3 mb-5">
            <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" class="w-5 h-5 text-amber-500 mt-0.5 shrink-0"><path stroke-linecap="round" stroke-linejoin="round" d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0"/></svg>
            <div>
              <h2 class="text-sm font-semibold text-primary">${t('dashboard.follow_ups_title')}</h2>
              <p class="text-xs text-muted mt-0.5">${t('dashboard.follow_ups_desc')}</p>
            </div>
          </div>
          <div class="space-y-3" id="follow-up-list">
            ${stats.follow_ups.map(f => {
              const ivDate = f.last_interview_at ? new Date(f.last_interview_at.replace(' ', 'T')) : null
              const daysAgo = ivDate ? Math.floor((Date.now() - ivDate.getTime()) / 86400000) : 0
              return `
              <div class="rounded border border-border/60 p-4" data-follow-up-id="${f.id}">
                <div class="flex items-start justify-between gap-3 mb-3">
                  <a href="/applications/${f.id}" data-link class="flex-1 min-w-0 no-underline group">
                    <span class="text-sm font-semibold text-primary group-hover:text-accent transition-colors block">${esc(f.company_name)}</span>
                    <span class="text-sm text-muted block mt-0.5">${esc(f.job_title)}</span>
                  </a>
                  <span class="text-xs text-amber-600 dark:text-amber-400 font-medium whitespace-nowrap">${t('dashboard.follow_up_days_ago').replace('{days}', String(daysAgo))}</span>
                </div>
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="text-xs text-muted mr-auto">${t('dashboard.follow_up_remind_later')}</span>
                  <button data-snooze-id="${f.id}" data-snooze-days="7" class="text-xs px-3 py-2 rounded-full border border-border text-muted hover:text-primary hover:bg-surface-2 transition-colors">${t('dashboard.follow_up_snooze_1w')}</button>
                  <button data-snooze-id="${f.id}" data-snooze-days="14" class="text-xs px-3 py-2 rounded-full border border-border text-muted hover:text-primary hover:bg-surface-2 transition-colors hidden sm:block">${t('dashboard.follow_up_snooze_2w')}</button>
                  <button data-snooze-id="${f.id}" data-snooze-days="21" class="text-xs px-3 py-2 rounded-full border border-border text-muted hover:text-primary hover:bg-surface-2 transition-colors hidden sm:block">${t('dashboard.follow_up_snooze_3w')}</button>
                  <button data-skip-id="${f.id}" class="text-xs px-3 py-2 rounded-full border border-border text-muted/50 hover:text-rose-500 hover:border-rose-300 dark:hover:border-rose-800 hover:bg-rose-50 dark:hover:bg-rose-950/30 transition-colors">${t('dashboard.follow_up_skip')}</button>
                </div>
              </div>`
            }).join('')}
          </div>
        </section>`
    }

    case 'timeline': {
      const legend = `
        <div class="chart-legend">
          <span><i style="background:rgb(var(--chart-sent))"></i>${t('dashboard.timeline_sent')}</span>
          <span><i style="background:rgb(var(--chart-replies))"></i>${t('dashboard.timeline_replies')}</span>
        </div>`
      return `
        ${panelOpen('timeline', t('dashboard.timeline_title'))}
          ${panelHead(t('dashboard.timeline_title'), t('dashboard.timeline_sub'), legend)}
          ${timelineChart(stats?.weekly ?? [])}
        </section>`
    }

    case 'funnel':
      return funnelPanel(stats?.period)

    case 'heatmap': {
      const days = stats?.activity_heatmap ?? []
      const interactions = days.reduce((a, d) => a + d.count, 0)
      return `
        ${panelOpen('heatmap', t('dashboard.heatmap_title'))}
          ${panelHead(t('dashboard.heatmap_title'), t('dashboard.heatmap_desc'), interpolate('dashboard.heatmap_total', interactions))}
          ${heatmapChart(days)}
        </section>`
    }

    case 'pipeline':
      return pipelinePanel(stats?.by_status ?? {}, total)

    case 'sources': {
      const sources = stats?.top_sources ?? []
      if (!sources.length) return ''
      return sourcesPanel(sources)
    }

    case 'activity': {
      const items = (stats?.recent_activity ?? []).slice(0, FEED_LIMIT)
      return `
        ${panelOpen('activity', t('dashboard.activity_title'))}
          ${panelHead(t('dashboard.activity_title'), t('dashboard.activity_desc'))}
          ${items.length === 0
            ? `<div class="panel-empty">${t('dashboard.no_data')}</div>`
            : activityFeed(items)}
        </section>`
    }
  }
}

function renderPanels(stats: Stats | null): string {
  return loadOrder()
    .map(id => renderPanel(id, stats))
    .filter(html => html.trim().length > 0)
    .join('')
}

// Drag & drop

function setupDragAndDrop(root: HTMLElement, setOrder: (o: PanelId[]) => void) {
  const container = root.querySelector<HTMLElement>('#dashboard-panels')
  if (!container) return

  let dragged: HTMLElement | null = null
  let placeholder: HTMLElement | null = null

  const panels = () => Array.from(container.querySelectorAll<HTMLElement>('.dash-panel'))

  const persist = () => setOrder(panels().map(p => p.dataset.panelId as PanelId))

  const onDragStart = (e: DragEvent) => {
    if (!root.classList.contains('dash-reorder-mode')) { e.preventDefault(); return }
    const target = e.target as HTMLElement
    const panel = target.closest<HTMLElement>('.dash-panel')
    if (!panel) return
    dragged = panel
    panel.classList.add('dash-dragging')
    placeholder = document.createElement('div')
    placeholder.className = 'dash-panel-placeholder'
    placeholder.style.height = panel.offsetHeight + 'px'
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', panel.dataset.panelId ?? '')
    }
  }

  const onDragOver = (e: DragEvent) => {
    if (!dragged || !placeholder) return
    e.preventDefault()
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
    const targetPanel = (e.target as HTMLElement).closest<HTMLElement>('.dash-panel')
    if (!targetPanel || targetPanel === dragged) return
    const rect = targetPanel.getBoundingClientRect()
    // Panels sit side by side from `lg` up: when the drop target shares a row
    // with where the panel currently sits, left/right decides; otherwise the
    // usual above/below rule applies.
    const from = (placeholder.isConnected ? placeholder : dragged).getBoundingClientRect()
    const sameRow = from.top < rect.bottom - 8 && from.bottom > rect.top + 8
    const before = sameRow
      ? e.clientX < rect.left + rect.width / 2
      : e.clientY < rect.top + rect.height / 2
    if (before) {
      container.insertBefore(placeholder, targetPanel)
    } else {
      container.insertBefore(placeholder, targetPanel.nextSibling)
    }
  }

  const onDrop = (e: DragEvent) => {
    e.preventDefault()
    if (!dragged || !placeholder) return
    container.insertBefore(dragged, placeholder)
    placeholder.remove()
    placeholder = null
    dragged.classList.remove('dash-dragging')
    dragged = null
    persist()
  }

  const onDragEnd = () => {
    if (placeholder) { placeholder.remove(); placeholder = null }
    if (dragged) { dragged.classList.remove('dash-dragging'); dragged = null }
  }

  // Delegate — survives re-rendering the panels on a period change.
  container.addEventListener('dragstart', onDragStart)
  container.addEventListener('dragover', onDragOver)
  container.addEventListener('drop', onDrop)
  container.addEventListener('dragend', onDragEnd)

  // Keyboard reorder: arrows move the focused panel (left/right behave like up/down).
  container.addEventListener('keydown', (e) => {
    if (!root.classList.contains('dash-reorder-mode')) return
    const active = document.activeElement as HTMLElement | null
    const panel = active?.closest<HTMLElement>('.dash-panel')
    if (!panel || !container.contains(panel)) return
    const back = e.key === 'ArrowUp' || e.key === 'ArrowLeft'
    const forward = e.key === 'ArrowDown' || e.key === 'ArrowRight'
    if (back && panel.previousElementSibling) {
      e.preventDefault()
      container.insertBefore(panel, panel.previousElementSibling)
      persist(); panel.focus()
    } else if (forward && panel.nextElementSibling) {
      e.preventDefault()
      container.insertBefore(panel.nextElementSibling, panel)
      persist(); panel.focus()
    }
  })
}

// Header

/** "12 avr. → 19 avr. 2026" — Monday to Sunday of the current week. */
function currentWeekRange(): string {
  const loc = getDateLocale()
  const now = new Date()
  const dow = (now.getDay() + 6) % 7 // 0 = Monday
  const monday = new Date(now)
  monday.setHours(0, 0, 0, 0)
  monday.setDate(now.getDate() - dow)
  const sunday = new Date(monday)
  sunday.setDate(monday.getDate() + 6)
  const from = monday.toLocaleDateString(loc, { day: 'numeric', month: 'short' })
  const to = sunday.toLocaleDateString(loc, { day: 'numeric', month: 'short', year: 'numeric' })
  return `${from} → ${to}`
}

function headerSubtitle(stats: Stats | null): string {
  const due = stats?.follow_ups?.length ?? 0
  const upcoming = stats?.upcoming_interviews ?? 0
  const parts: string[] = []
  if (due > 0) parts.push(count('dashboard.due_follow_ups', due))
  if (upcoming > 0) parts.push(count('dashboard.upcoming_interviews', upcoming))
  if (!parts.length) return currentWeekRange()
  return `${currentWeekRange()} · ${parts.join(' · ')}`
}

// Main page

export async function DashboardPage(): Promise<HTMLElement> {
  let period = loadPeriod()
  let stats = await api.stats(period).catch(() => null) as Stats | null

  const content = document.createElement('div')
  content.className = 'space-y-6 stagger dashboard-root'

  content.innerHTML = `
    <div class="dash-head relative z-10">
      <div>
        <h1 class="text-2xl font-bold text-primary tracking-tight">${t('dashboard.greeting')}</h1>
        <p class="text-sm text-muted mt-1" id="dash-subtitle">${headerSubtitle(stats)}</p>
      </div>
      <div class="dash-head-actions">
        <div class="dash-range" role="tablist" aria-label="${t('dashboard.period_label')}">
          ${PERIODS.map(p => `
            <button type="button" role="tab" data-period="${p}" aria-selected="${p === period ? 'true' : 'false'}">${periodLabel(p)}</button>
          `).join('')}
        </div>
        <div class="relative" id="data-menu">
          <button id="data-menu-btn" class="btn-ghost p-2.5 min-w-[44px] min-h-[44px] flex items-center justify-center text-muted" title="${t('dashboard.data_menu')}" aria-haspopup="true" aria-expanded="false">
            <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" class="w-5 h-5"><path stroke-linecap="round" stroke-linejoin="round" d="M6.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zm6 0a.75.75 0 11-1.5 0 .75.75 0 011.5 0zm6 0a.75.75 0 11-1.5 0 .75.75 0 011.5 0z"/></svg>
          </button>
          <div id="data-menu-dropdown" class="hidden absolute right-0 top-full mt-1 z-50 rounded border border-border shadow-elevated py-1" style="background: rgb(var(--color-surface-1));">
            <button id="reorder-btn" class="flex items-center gap-2.5 w-full text-left px-4 py-3 text-sm text-primary hover:bg-surface-2 transition-colors whitespace-nowrap">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" class="w-4 h-4 text-muted shrink-0" aria-hidden="true">
                <path d="M7 4a1 1 0 110 2 1 1 0 010-2zm6 0a1 1 0 110 2 1 1 0 010-2zM7 9a1 1 0 110 2 1 1 0 010-2zm6 0a1 1 0 110 2 1 1 0 010-2zM7 14a1 1 0 110 2 1 1 0 010-2zm6 0a1 1 0 110 2 1 1 0 010-2z"/>
              </svg>
              ${t('dashboard.reorder')}
            </button>
            <div class="border-t border-border/60 my-1"></div>
            <button id="export-btn" class="block w-full text-left px-4 py-3 text-sm text-primary hover:bg-surface-2 transition-colors whitespace-nowrap">${t('dashboard.export')}</button>
            <button id="export-csv-btn" class="block w-full text-left px-4 py-3 text-sm text-primary hover:bg-surface-2 transition-colors whitespace-nowrap">${t('dashboard.export_csv')}</button>
            <div class="border-t border-border/60 my-1"></div>
            <button id="import-btn" class="block w-full text-left px-4 py-3 text-sm text-primary hover:bg-surface-2 transition-colors whitespace-nowrap">${t('dashboard.import')}</button>
          </div>
          <input type="file" id="import-file" accept=".json" class="hidden" />
        </div>
      </div>
    </div>

    <div id="reorder-banner" class="hidden items-center justify-between gap-3 rounded border border-accent/30 bg-accent/[0.06] dark:bg-accent/[0.12] px-4 py-2.5 text-sm">
      <span class="text-primary">
        <span class="font-semibold">${t('dashboard.reorder_mode')}</span>
        <span class="text-muted ml-1.5">${t('dashboard.reorder_hint')}</span>
      </span>
      <div class="flex items-center gap-2">
        <button id="reorder-reset" class="text-xs px-3 py-1.5 rounded-full border border-border text-muted hover:text-primary hover:bg-surface-2 transition-colors">${t('dashboard.reorder_reset')}</button>
        <button id="reorder-done" class="text-xs px-3 py-1.5 rounded-full bg-primary text-[rgb(var(--color-surface))] font-medium hover:opacity-85 transition-opacity">${t('dashboard.reorder_done')}</button>
      </div>
    </div>

    <div id="dashboard-panels">${renderPanels(stats)}</div>
  `

  const panelsEl = content.querySelector('#dashboard-panels') as HTMLElement

  // ---- Panel wiring (re-run after every period change) ----------------------

  const dismissFollowUp = (appId: string) => {
    const card = content.querySelector(`[data-follow-up-id="${appId}"]`) as HTMLElement | null
    if (!card) return
    card.style.opacity = '0'
    card.style.transform = 'translateX(20px)'
    card.style.transition = 'opacity 0.2s ease, transform 0.2s ease'
    setTimeout(() => {
      card.remove()
      const list = content.querySelector('#follow-up-list')
      if (list && list.children.length === 0) list.closest('.dash-panel')?.remove()
    }, 200)
  }

  /** Hover / focus / tap a week slot to explain what its bars mean. */
  const wireTimelineTips = () => {
    const chart = panelsEl.querySelector<HTMLElement>('.tl-chart')
    const tip = chart?.querySelector<HTMLElement>('.tl-tip')
    if (!chart || !tip) return
    let active: HTMLElement | null = null

    const hide = () => {
      if (active) active.classList.remove('tl-hit-active')
      active = null
      tip.hidden = true
    }

    const show = (hit: HTMLElement) => {
      if (active && active !== hit) active.classList.remove('tl-hit-active')
      active = hit
      hit.classList.add('tl-hit-active')
      // Built with textContent, never innerHTML: the copy comes back out of a
      // data attribute and must not be parsed as markup.
      tip.textContent = ''
      const title = document.createElement('b')
      title.textContent = hit.dataset.tipTitle ?? ''
      tip.append(title)
      for (const key of ['tipSent', 'tipReplies'] as const) {
        const line = document.createElement('span')
        line.textContent = hit.dataset[key] ?? ''
        tip.append(line)
      }
      tip.hidden = false
      // Centre on the slot, then keep the whole tooltip inside the chart.
      const box = chart.getBoundingClientRect()
      const slot = hit.getBoundingClientRect()
      const centre = slot.left - box.left + slot.width / 2
      const half = tip.offsetWidth / 2
      const max = box.width - tip.offsetWidth
      tip.style.left = `${Math.min(Math.max(centre - half, 0), Math.max(max, 0))}px`
    }

    chart.querySelectorAll<HTMLElement>('.tl-hit').forEach(hit => {
      hit.addEventListener('mouseenter', () => show(hit))
      hit.addEventListener('focus', () => show(hit))
      hit.addEventListener('blur', hide)
      // Touch has no hover, so a tap toggles.
      hit.addEventListener('click', () => { if (active === hit) hide(); else show(hit) })
    })
    chart.addEventListener('mouseleave', hide)
  }

  /** GitHub-style: click a heatmap day to list that day's events in the card. */
  const wireHeatmapDays = () => {
    const wrap = panelsEl.querySelector<HTMLElement>('.heatmap-wrap')
    const dayPanel = wrap?.querySelector<HTMLElement>('.heatmap-day')
    if (!wrap || !dayPanel) return
    let selected: string | null = null

    const close = () => {
      selected = null
      dayPanel.hidden = true
      dayPanel.textContent = ''
      wrap.querySelectorAll('.heatmap-day-selected').forEach(c => c.classList.remove('heatmap-day-selected'))
    }

    const open = async (cell: HTMLElement, iso: string) => {
      if (selected === iso) { close(); return }
      dayPanel.setAttribute('aria-busy', 'true')
      let items: ActivityItem[]
      try {
        items = await api.activityByDay(iso)
      } catch {
        // Keep whatever was on screen rather than blanking the card.
        dayPanel.removeAttribute('aria-busy')
        toast(t('form.error'), 'error')
        return
      }
      dayPanel.removeAttribute('aria-busy')
      selected = iso
      wrap.querySelectorAll('.heatmap-day-selected').forEach(c => c.classList.remove('heatmap-day-selected'))
      cell.classList.add('heatmap-day-selected')

      // Midday avoids the parse landing on the previous day in a negative offset.
      const long = new Date(iso + 'T12:00:00').toLocaleDateString(getDateLocale(), {
        weekday: 'long', day: 'numeric', month: 'long',
      })
      const noun = items.length === 1 ? t('dashboard.heatmap_event_one') : t('dashboard.heatmap_event_other')
      dayPanel.innerHTML = `
        <div class="heatmap-day-head">
          <span>${esc(long)} · ${items.length} ${esc(noun)}</span>
          <button type="button" class="heatmap-day-close" title="${t('dashboard.heatmap_day_close')}" aria-label="${t('dashboard.heatmap_day_close')}">
            <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
          </button>
        </div>
        ${items.length ? activityFeed(items) : `<div class="heatmap-day-empty">${t('dashboard.heatmap_day_empty')}</div>`}`
      dayPanel.hidden = false
      dayPanel.querySelector('.heatmap-day-close')?.addEventListener('click', close)
    }

    wrap.querySelectorAll<HTMLElement>('[data-day]').forEach(cell => {
      const iso = cell.dataset.day!
      cell.addEventListener('click', () => { void open(cell, iso) })
      cell.addEventListener('keydown', (e) => {
        if (e.key !== 'Enter' && e.key !== ' ') return
        e.preventDefault()
        void open(cell, iso)
      })
    })
  }

  const wirePanels = () => {
    panelsEl.querySelectorAll<HTMLButtonElement>('[data-snooze-id]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const appId = btn.dataset.snoozeId!
        const days = Number(btn.dataset.snoozeDays)
        const until = new Date()
        until.setDate(until.getDate() + days)
        try {
          await api.applications.snooze(appId, { until: localDayKey(until) })
          dismissFollowUp(appId)
          toast(t('dashboard.follow_up_snoozed'), 'success')
        } catch { toast(t('form.error'), 'error') }
      })
    })
    panelsEl.querySelectorAll<HTMLButtonElement>('[data-skip-id]').forEach(btn => {
      btn.addEventListener('click', async () => {
        const appId = btn.dataset.skipId!
        try {
          await api.applications.snooze(appId, { skip: true })
          dismissFollowUp(appId)
          toast(t('dashboard.follow_up_skipped'), 'info')
        } catch { toast(t('form.error'), 'error') }
      })
    })

    wireTimelineTips()
    wireHeatmapDays()

    panelsEl.querySelectorAll<HTMLButtonElement>('[data-src-more]').forEach(btn => {
      btn.addEventListener('click', () => {
        const list = btn.parentElement?.querySelector('.src-list')
        if (!list) return
        const open = list.classList.toggle('expanded')
        btn.setAttribute('aria-expanded', open ? 'true' : 'false')
        btn.textContent = open
          ? t('dashboard.sources_less')
          : interpolate('dashboard.sources_more', Number(btn.dataset.srcMore))
      })
    })

    // Panels keep their reorder affordances across a re-render.
    if (content.classList.contains('dash-reorder-mode')) {
      panelsEl.querySelectorAll<HTMLElement>('.dash-panel').forEach(p => p.setAttribute('tabindex', '0'))
    }

    requestAnimationFrame(() => {
      animateCounters(panelsEl)
      panelsEl.querySelectorAll<HTMLElement>('[data-bar-width]').forEach(el => {
        el.style.width = el.dataset.barWidth!
      })
    })
  }

  wirePanels()

  // ---- Period selector ------------------------------------------------------

  const rangeButtons = Array.from(content.querySelectorAll<HTMLButtonElement>('.dash-range button'))
  const setPeriod = async (next: DashboardPeriod) => {
    if (next === period) return
    // Fetch first, commit after: a failed request must leave `period`, the
    // stored preference and the selector untouched, so clicking again retries
    // instead of being swallowed by the `next === period` guard.
    panelsEl.setAttribute('aria-busy', 'true')
    const fresh = await api.stats(next).catch(() => null) as Stats | null
    panelsEl.removeAttribute('aria-busy')
    if (!fresh) { toast(t('dashboard.period_error'), 'error'); return }
    period = next
    savePeriod(next)
    rangeButtons.forEach(b => b.setAttribute('aria-selected', b.dataset.period === next ? 'true' : 'false'))
    stats = fresh
    const subtitle = content.querySelector('#dash-subtitle')
    if (subtitle) subtitle.textContent = headerSubtitle(stats)
    panelsEl.innerHTML = renderPanels(stats)
    wirePanels()
  }
  rangeButtons.forEach(btn => {
    btn.addEventListener('click', () => { void setPeriod(btn.dataset.period as DashboardPeriod) })
  })

  // ---- Data menu dropdown ---------------------------------------------------

  const menuBtn = content.querySelector('#data-menu-btn') as HTMLElement
  const dropdown = content.querySelector('#data-menu-dropdown') as HTMLElement
  const closeMenu = () => {
    dropdown.classList.replace('dropdown-enter', 'dropdown-exit')
    dropdown.addEventListener('animationend', () => { dropdown.classList.add('hidden'); dropdown.classList.remove('dropdown-exit') }, { once: true })
    menuBtn.setAttribute('aria-expanded', 'false')
  }
  menuBtn?.addEventListener('click', () => {
    const open = !dropdown.classList.contains('hidden')
    if (open) { closeMenu(); return }
    dropdown.classList.remove('hidden')
    dropdown.classList.add('dropdown-enter')
    menuBtn.setAttribute('aria-expanded', 'true')
  })
  document.addEventListener('click', (e) => {
    if (!content.querySelector('#data-menu')?.contains(e.target as Node) && !dropdown.classList.contains('hidden')) {
      closeMenu()
    }
  })

  // ---- Reorder mode ---------------------------------------------------------

  const reorderBanner = content.querySelector('#reorder-banner') as HTMLElement
  const reorderDone = content.querySelector('#reorder-done') as HTMLButtonElement
  const reorderReset = content.querySelector('#reorder-reset') as HTMLButtonElement
  const setReorder = (on: boolean) => {
    content.classList.toggle('dash-reorder-mode', on)
    if (on) { reorderBanner.classList.remove('hidden'); reorderBanner.classList.add('flex') }
    else { reorderBanner.classList.add('hidden'); reorderBanner.classList.remove('flex') }
    if (on) {
      content.querySelectorAll<HTMLElement>('.dash-panel').forEach(p => p.setAttribute('tabindex', '0'))
      content.querySelector<HTMLElement>('.dash-panel')?.focus()
    } else {
      content.querySelectorAll<HTMLElement>('.dash-panel').forEach(p => p.removeAttribute('tabindex'))
    }
  }
  content.querySelector('#reorder-btn')?.addEventListener('click', () => {
    closeMenu()
    setReorder(true)
  })
  reorderDone.addEventListener('click', () => setReorder(false))
  reorderReset.addEventListener('click', () => {
    resetOrder()
    toast(t('dashboard.reorder_reset_done'), 'info')
    navigate(window.location.pathname + window.location.search)
  })

  setupDragAndDrop(content, saveOrder)

  // ---- Export / import ------------------------------------------------------

  content.querySelector('#export-btn')?.addEventListener('click', async () => {
    closeMenu()
    const data = await api.export()
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `jobctrl-export-${new Date().toISOString().slice(0, 10)}.json`
    a.click()
    URL.revokeObjectURL(url)
  })

  content.querySelector('#export-csv-btn')?.addEventListener('click', () => {
    closeMenu()
    window.open('/api/export/csv', '_blank')
  })

  const importFile = content.querySelector('#import-file') as HTMLInputElement
  content.querySelector('#import-btn')?.addEventListener('click', () => { closeMenu(); importFile.click() })
  importFile?.addEventListener('change', async () => {
    const file = importFile.files?.[0]
    if (!file) return
    try {
      const text = await file.text()
      const data = JSON.parse(text)
      if (!data.applications) {
        toast(t('dashboard.import_invalid'), 'error')
        return
      }
      const result = await api.import_(data)
      const parts = [`${result.imported} ${t('dashboard.import_success')}`]
      if (result.skipped > 0) parts.push(`${result.skipped} ${t('dashboard.import_skipped')}`)
      toast(parts.join(', '), result.imported > 0 ? 'success' : 'info')
      if (result.imported > 0) setTimeout(() => navigate('/'), 1500)
    } catch {
      toast(t('dashboard.import_error'), 'error')
    } finally {
      importFile.value = ''
    }
  })

  return createLayout(content)
}
