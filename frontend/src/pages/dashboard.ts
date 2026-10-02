import { api } from '../api'
import { createLayout } from '../components/layout'
import { rerender, setNavigationCleanup } from '../router'
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

/** "mercredi 30 septembre" for a `YYYY-MM-DD` day key. Midday avoids the parse
 *  landing on the previous day in a negative offset. */
function longDay(iso: string): string {
  return new Date(iso + 'T12:00:00').toLocaleDateString(getDateLocale(), {
    weekday: 'long', day: 'numeric', month: 'long',
  })
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
  // Decorative: the tile's own text carries the value and the delta.
  return `<svg class="kpi-spark" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}" fill="none" aria-hidden="true">
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
    const sentText = count('dashboard.timeline_tip_sent', w.sent)
    const repliesText = count('dashboard.timeline_tip_replies', w.replies)
    // The tooltip is aria-hidden, so the label carries the week's figures.
    return `<button type="button" class="tl-hit"
      data-tip-title="${esc(title)}"
      data-tip-sent="${esc(sentText)}"
      data-tip-replies="${esc(repliesText)}"
      aria-label="${esc(`${title}: ${sentText}, ${repliesText}`)}"></button>`
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

/** Linear-interpolated percentile of an ascending list, 0 when it is empty. */
function percentile(sorted: number[], p: number): number {
  if (sorted.length === 0) return 0
  const rank = p * (sorted.length - 1)
  const lo = Math.floor(rank)
  const hi = Math.ceil(rank)
  return sorted[lo] + (sorted[hi] - sorted[lo]) * (rank - lo)
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

  // Scale on the 90th percentile of active days rather than the busiest one,
  // so a single unusual day (a batch of automatic "no reply" moves, an import)
  // does not fade every other day to the lightest shade. Days above it get the
  // top level.
  const active = days.map(d => d.count).filter(c => c > 0).sort((a, b) => a - b)
  const max = Math.max(1, percentile(active, 0.9))

  const level = (c: number): number => {
    if (c === 0) return 0
    const r = c / max
    if (r > 0.75) return 4
    if (r > 0.5) return 3
    if (r > 0.25) return 2
    return 1
  }

  // Emitted week by week (the grid flows by column), so document and focus
  // order are chronological. Only today is a tab stop; arrow keys walk the
  // other days (see wireHeatmapDays).
  const todayKey = localDayKey(today)
  let cells = ''
  for (let col = 0; col < WEEKS; col++) {
    for (let row = 0; row < 7; row++) {
      const d = new Date(start)
      d.setDate(start.getDate() + col * 7 + row)
      if (d > today) { cells += `<i aria-hidden="true" class="opacity-0"></i>`; continue }
      const iso = localDayKey(d)
      const c = byDate.get(iso) ?? 0
      const lvl = level(c)
      const label = `${longDay(iso)} · ${c} ${tp('dashboard.heatmap_event', c)}`
      cells += `<i data-l="${lvl}" data-day="${iso}" role="button" tabindex="${iso === todayKey ? '0' : '-1'}" title="${esc(label)}" aria-label="${esc(label)}"></i>`
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
      <div class="heatmap" role="group" aria-label="${t('dashboard.heatmap_title')}" style="grid-template-columns: repeat(${WEEKS}, 1fr); grid-template-rows: repeat(7, 1fr); grid-auto-flow: column">${cells}</div>
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

/** `interview_held` descriptions arrive as "<Type> · round <n>" (English,
 *  built by the server): the type and the round word are localised. */
function heldDescription(description: string): string {
  const m = description.match(/^(.+) · round (\d+)$/)
  if (m) return `${interviewTypeLabel(m[1])} · ${t('detail.round').toLowerCase()} ${m[2]}`
  const sep = description.indexOf(' · ')
  if (sep === -1) return interviewTypeLabel(description)
  return interviewTypeLabel(description.slice(0, sep)) + description.slice(sep)
}

/** "auj. 10:24" · "hier" · "14 avr.": timeline events are real instants,
 *  shown in the reader's zone. `floating` values (an interview's scheduled_at)
 *  are wall-clock times labelled UTC, so they are read back in UTC. */
function feedTime(raw: string, floating = false): string {
  const d = parseUTC(raw)
  if (isNaN(d.getTime())) return ''
  const loc = getDateLocale()
  const zone: Intl.DateTimeFormatOptions = floating ? { timeZone: 'UTC' } : {}
  // Compare calendar days, so DST changes cannot skew the count.
  const now = new Date()
  const today = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate())
  const day = floating
    ? Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate())
    : Date.UTC(d.getFullYear(), d.getMonth(), d.getDate())
  const diffDays = Math.round((today - day) / 86400000)
  if (diffDays === 0) {
    return `${t('dashboard.time_today')} ${d.toLocaleTimeString(loc, { hour: '2-digit', minute: '2-digit', ...zone })}`
  }
  if (diffDays === 1) return t('dashboard.time_yesterday')
  return d.toLocaleDateString(loc, { day: 'numeric', month: 'short', ...zone })
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
  // `draggable` is only set in reorder mode (setReorder): a draggable panel
  // cannot have its text selected with the mouse.
  return `<section class="${cls}" data-panel-id="${id}"${span} aria-label="${label}">${panelHandle()}`
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
      delta = `<div class="kpi-delta ${tone}"><span aria-hidden="true">${arrow}</span> ${up ? '+' : '-'}${Math.abs(diff)} ${opts.deltaUnit}</div>`
    }
  }

  // No aria-label: the link is named by its content, so the label, the value
  // and the delta are all read out.
  return `
    <a href="${esc(opts.href)}" data-link class="kpi">
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

/** The list a tile leads to, filtered like the tile's own count: sent
 *  applications only, over the same window (`period` left out for all time). */
function kpiHref(query: string, days: number | undefined): string {
  const period = days === 30 || days === 90 || days === 365 ? `&period=${days}` : ''
  return `/applications?${query}${period}`
}

function kpisPanel(period: PeriodStats | undefined): string {
  const token = periodToken(period?.days)
  const days = period?.days
  const tiles = [
    kpiTile(t('dashboard.kpi_sent'), period?.sent, 'chart-sent', { href: kpiHref('sent=1', days), deltaUnit: token }),
    kpiTile(t('dashboard.kpi_responded'), period?.responded, 'chart-replies', { href: kpiHref('has_reply=1&sent=1', days), deltaUnit: token }),
    kpiTile(t('dashboard.kpi_no_reply'), period?.no_reply, 'chart-no-reply', { href: kpiHref('status=NoReply', days), deltaUnit: token, neutral: true }),
    kpiTile(t('dashboard.kpi_interviews'), period?.interviews, 'chart-interviews', { href: kpiHref('has_interviews=1&sent=1', days), deltaUnit: token }),
    kpiTile(t('dashboard.kpi_rejected'), period?.rejected, 'chart-neutral', { href: kpiHref('status=Rejected', days), deltaUnit: token, neutral: true }),
    kpiTile(t('dashboard.kpi_offers'), period?.offers, 'chart-offers', { href: kpiHref('status=Offer,Accepted', days), deltaUnit: token }),
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
  const sub = tp('dashboard.funnel_sub', sent).replace('{sent}', String(sent)).replace('{period}', periodText)

  // A refusal is a reply: it is counted in the first bar, not in the rest.
  const rejected = Math.round(period?.rejected?.value ?? 0)
  const respondedLabel = rejected > 0
    ? `${t('dashboard.funnel_responded')}, ${count('dashboard.funnel_responded_rejected', rejected)}`
    : t('dashboard.funnel_responded')

  const rows: Array<{ label: string; value: number; color: string }> = [
    { label: respondedLabel, value: responded, color: 'chart-replies' },
    { label: t('dashboard.funnel_interviewing'), value: f?.interviewing ?? 0, color: 'chart-interviews' },
    { label: t('dashboard.funnel_offer'), value: f?.offers ?? 0, color: 'chart-offers' },
    { label: t('dashboard.funnel_accepted'), value: f?.accepted ?? 0, color: 'chart-accepted' },
  ]

  // Same period as the funnel: what is not in the replies bar is either
  // unanswered or still pending, so replies + these two add up to `sent`.
  // Clamped because the counts are computed independently server-side.
  const noReply = Math.round(period?.no_reply?.value ?? 0)
  const pending = Math.max(0, sent - responded - noReply)

  // "50 %" in French, "50%" in English.
  const percent = new Intl.NumberFormat(getDateLocale(), { style: 'percent', maximumFractionDigits: 0 })
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
            <div class="fun-count">${r.value} · ${esc(percent.format(pct / 100))}</div>
          </div>`
      }).join('')}
      <div class="fun-rest">${fill('dashboard.funnel_others', { no_reply: noReply, pending })}</div>
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
      ${panelHead(t('dashboard.status_breakdown'), total > 0 ? count('dashboard.status_breakdown_sub', total) : undefined)}
      ${body}
    </section>`
}

// Top sources

function sourcesPanel(sources: Array<{ source: string; count: number }>): string {
  const rows = sources.map((src, i) => {
    const domain = getSourceDomain(src.source)
    const favicon = domain
      ? `<img src="${esc(faviconUrl(domain))}" width="16" height="16" alt="" loading="lazy" class="source-favicon" data-hide-on-error />`
      : ''
    return `
      <a href="/applications?source=${encodeURIComponent(src.source)}" data-link class="src-row${i >= LIST_LIMIT ? ' src-extra' : ''}">
        ${favicon}<span class="src-name">${esc(src.source)}</span>
        <span class="src-count">${src.count}</span>
      </a>`
  }).join('')

  const extra = sources.length - LIST_LIMIT
  const toggle = extra > 0
    ? `<button type="button" class="src-more" data-src-more="${extra}" aria-expanded="false">${count('dashboard.sources_more', extra)}</button>`
    : ''

  return `
    ${panelOpen('sources', t('dashboard.sources_title'))}
      ${panelHead(t('dashboard.sources_title'), t('dashboard.sources_sub'))}
      <div class="src-list">${rows}</div>
      ${toggle}
    </section>`
}

// Active processes

/** Server datetimes arrive as `2006-01-02 15:04:05` (UTC, no zone) or as
 *  RFC 3339 with a zone; `Z` is only appended when the zone is missing. */
function parseUTC(at: string): Date {
  const s = at.trim().replace(' ', 'T')
  return new Date(/(Z|[+-]\d{2}:?\d{2})$/i.test(s) ? s : s + 'Z')
}

/** "21 sept.": the interview day as typed. Interview times are floating
 *  wall-clock times labelled UTC, so they are formatted in UTC. */
function stepDate(at: string): string {
  const d = parseUTC(at)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleDateString(getDateLocale(), { day: 'numeric', month: 'short', timeZone: 'UTC' })
}

/** "21 sept. 14:30" — used for a scheduled interview, where the hour matters. */
function stepDateTime(at: string): string {
  const d = parseUTC(at)
  if (isNaN(d.getTime())) return ''
  const loc = getDateLocale()
  const day = d.toLocaleDateString(loc, { day: 'numeric', month: 'short', timeZone: 'UTC' })
  return `${day} ${d.toLocaleTimeString(loc, { hour: '2-digit', minute: '2-digit', timeZone: 'UTC' })}`
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
      <a href="/applications/${esc(p.id)}" data-link class="ap-row">
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
          <a href="/applications/${esc(it.application_id)}" data-link class="feed-item">
            <div class="feed-time">${esc(feedTime(it.time, it.event_type === 'interview_held'))}</div>
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
        <section class="dash-panel dash-panel-card border-amber-500/20 dark:border-amber-400/15" data-panel-id="follow-ups" data-span="full" aria-label="${t('dashboard.follow_ups_title')}">
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
              // Parsed as UTC, like the server's own silent_days count on the
              // same value; an unparseable date shows no age rather than NaN.
              const ivTime = f.last_interview_at ? parseUTC(f.last_interview_at).getTime() : NaN
              const daysAgo = isNaN(ivTime) ? null : Math.floor((Date.now() - ivTime) / 86400000)
              const id = esc(f.id)
              return `
              <div class="rounded border border-border/60 p-4" data-follow-up-id="${id}">
                <div class="flex items-start justify-between gap-3 mb-3">
                  <a href="/applications/${id}" data-link class="flex-1 min-w-0 no-underline group">
                    <span class="text-sm font-semibold text-primary group-hover:text-accent transition-colors block">${esc(f.company_name)}</span>
                    <span class="text-sm text-muted block mt-0.5">${esc(f.job_title)}</span>
                  </a>
                  ${daysAgo === null ? '' : `<span class="text-xs text-amber-600 dark:text-amber-400 font-medium whitespace-nowrap">${t('dashboard.follow_up_days_ago').replace('{days}', String(daysAgo))}</span>`}
                </div>
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="text-xs text-muted mr-auto">${t('dashboard.follow_up_remind_later')}</span>
                  <button data-snooze-id="${id}" data-snooze-days="7" class="text-xs px-3 py-2 rounded-full border border-border text-muted hover:text-primary hover:bg-surface-2 transition-colors">${t('dashboard.follow_up_snooze_1w')}</button>
                  <button data-snooze-id="${id}" data-snooze-days="14" class="text-xs px-3 py-2 rounded-full border border-border text-muted hover:text-primary hover:bg-surface-2 transition-colors hidden sm:block">${t('dashboard.follow_up_snooze_2w')}</button>
                  <button data-snooze-id="${id}" data-snooze-days="21" class="text-xs px-3 py-2 rounded-full border border-border text-muted hover:text-primary hover:bg-surface-2 transition-colors hidden sm:block">${t('dashboard.follow_up_snooze_3w')}</button>
                  <button data-skip-id="${id}" class="text-xs px-3 py-2 rounded-full border border-border text-muted/50 hover:text-rose-500 hover:border-rose-300 dark:hover:border-rose-800 hover:bg-rose-50 dark:hover:bg-rose-950/30 transition-colors">${t('dashboard.follow_up_skip')}</button>
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
          ${panelHead(t('dashboard.heatmap_title'), t('dashboard.heatmap_desc'), count('dashboard.heatmap_total', interactions))}
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

  /** Saves the on-screen order without losing the slot of the panels that are
   *  not rendered right now (no follow-ups due, no sources...): each one goes
   *  back right after the panel that preceded it in the previous order. */
  const persist = () => {
    const merged = panels().map(p => p.dataset.panelId as PanelId)
    const previous = loadOrder()
    previous.forEach((id, i) => {
      if (merged.includes(id)) return
      let at = 0
      for (let j = i - 1; j >= 0; j--) {
        const k = merged.indexOf(previous[j])
        if (k !== -1) { at = k + 1; break }
      }
      merged.splice(at, 0, id)
    })
    setOrder(merged)
  }

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
    // Dropped where it was picked up: the placeholder was never inserted.
    if (!dragged || !placeholder || !placeholder.isConnected) { onDragEnd(); return }
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

  // Screen readers hear where a keyboard move put the panel.
  const announce = (panel: HTMLElement) => {
    const live = root.querySelector('#reorder-live')
    if (!live) return
    const all = panels()
    live.textContent = fill('dashboard.reorder_moved', {
      panel: panel.getAttribute('aria-label') ?? '',
      n: all.indexOf(panel) + 1,
      total: all.length,
    })
  }

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
      persist(); panel.focus(); announce(panel)
    } else if (forward && panel.nextElementSibling) {
      e.preventDefault()
      container.insertBefore(panel.nextElementSibling, panel)
      persist(); panel.focus(); announce(panel)
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

/** Shown in place of the panels when the stats cannot be loaded, so a server
 *  hiccup never reads as an empty search. Not a `.dash-panel`: reordering
 *  and the saved order ignore it. */
function loadErrorBlock(): string {
  return `
    <div class="dash-panel-card text-center" data-span="full" role="alert">
      <p class="text-primary font-semibold mb-1">${t('dashboard.load_error')}</p>
      <p class="text-sm text-muted mb-5">${t('common.load_error_hint')}</p>
      <button type="button" id="dash-retry" class="btn-primary">${t('common.retry')}</button>
    </div>`
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
          <button id="data-menu-btn" class="btn-ghost p-2.5 min-w-[44px] min-h-[44px] flex items-center justify-center text-muted" title="${t('dashboard.data_menu')}" aria-expanded="false" aria-controls="data-menu-dropdown">
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
      <span id="reorder-live" class="sr-only" role="status"></span>
    </div>

    <div id="dashboard-panels">${stats ? renderPanels(stats) : loadErrorBlock()}</div>
  `

  const panelsEl = content.querySelector('#dashboard-panels') as HTMLElement

  // ---- Panel wiring (re-run after every period change) ----------------------

  const dismissFollowUp = (appId: string) => {
    const card = content.querySelector(`[data-follow-up-id="${CSS.escape(appId)}"]`) as HTMLElement | null
    if (!card) return
    // The header counts the reminders due: keep it in step with the list.
    if (stats?.follow_ups) stats.follow_ups = stats.follow_ups.filter(f => f.id !== appId)
    const subtitle = content.querySelector('#dash-subtitle')
    if (subtitle) subtitle.textContent = headerSubtitle(stats)
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
      // Touch has no hover: a tap shows the week (it follows the emulated
      // mouseenter, so toggling here would hide it again at once). Tapping
      // elsewhere blurs the slot and hides the tooltip.
      hit.addEventListener('click', () => show(hit))
    })
    chart.addEventListener('mouseleave', hide)
  }

  /** GitHub-style: click a heatmap day to list that day's events in the card. */
  const wireHeatmapDays = () => {
    const wrap = panelsEl.querySelector<HTMLElement>('.heatmap-wrap')
    const dayPanel = wrap?.querySelector<HTMLElement>('.heatmap-day')
    if (!wrap || !dayPanel) return
    let selected: string | null = null
    // The day the card currently lists (null when closed).
    let shown: string | null = null
    // Each request takes a ticket: only the latest one may fill the card, and
    // closing the card voids any request still in flight.
    let req = 0

    const highlight = (iso: string | null) => {
      wrap.querySelectorAll('.heatmap-day-selected').forEach(c => c.classList.remove('heatmap-day-selected'))
      if (iso) wrap.querySelector(`[data-day="${iso}"]`)?.classList.add('heatmap-day-selected')
    }

    const close = () => {
      req++
      selected = null
      shown = null
      dayPanel.hidden = true
      dayPanel.textContent = ''
      dayPanel.removeAttribute('aria-busy')
      highlight(null)
    }

    const open = async (iso: string) => {
      if (selected === iso) { close(); return }
      const my = ++req
      // Selected right away, so a second click on the same day closes it.
      selected = iso
      highlight(iso)
      dayPanel.setAttribute('aria-busy', 'true')
      let items: ActivityItem[]
      try {
        items = await api.activityByDay(iso)
      } catch {
        if (my !== req) return
        // Keep whatever was on screen rather than blanking the card, and
        // point the selection back at the day it shows.
        dayPanel.removeAttribute('aria-busy')
        selected = shown
        highlight(shown)
        toast(t('form.error'), 'error')
        return
      }
      if (my !== req) return
      dayPanel.removeAttribute('aria-busy')
      shown = iso

      const noun = tp('dashboard.heatmap_event', items.length)
      dayPanel.innerHTML = `
        <div class="heatmap-day-head">
          <span>${esc(longDay(iso))} · ${items.length} ${esc(noun)}</span>
          <button type="button" class="heatmap-day-close" title="${t('dashboard.heatmap_day_close')}" aria-label="${t('dashboard.heatmap_day_close')}">
            <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke-width="2" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
          </button>
        </div>
        ${items.length ? activityFeed(items) : `<div class="heatmap-day-empty">${t('dashboard.heatmap_day_empty')}</div>`}`
      dayPanel.hidden = false
      dayPanel.querySelector('.heatmap-day-close')?.addEventListener('click', close)
    }

    // Cells are in chronological order: one step is a day, seven a week.
    const cells = Array.from(wrap.querySelectorAll<HTMLElement>('[data-day]'))
    const steps: Record<string, number> = { ArrowUp: -1, ArrowDown: 1, ArrowLeft: -7, ArrowRight: 7 }
    const rove = (to: HTMLElement) => cells.forEach(c => { c.tabIndex = c === to ? 0 : -1 })
    cells.forEach((cell, i) => {
      const iso = cell.dataset.day!
      cell.addEventListener('click', () => { rove(cell); void open(iso) })
      cell.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          void open(iso)
          return
        }
        const next = cells[i + (steps[e.key] ?? NaN)]
        if (!next) return
        e.preventDefault()
        rove(next)
        next.focus()
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
          : count('dashboard.sources_more', Number(btn.dataset.srcMore))
      })
    })

    // Panels keep their reorder affordances across a re-render.
    if (content.classList.contains('dash-reorder-mode')) {
      panelsEl.querySelectorAll<HTMLElement>('.dash-panel').forEach(p => {
        p.setAttribute('tabindex', '0')
        p.setAttribute('draggable', 'true')
      })
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
  // `requested` is the last period asked for, `period` the one on screen.
  // Each fetch takes a ticket so that only the latest answer is applied: a
  // slow older response can no longer overwrite (and persist) a newer choice.
  let requested = period
  let statsSeq = 0
  const fetchStats = async (next: DashboardPeriod, errorKey: string) => {
    requested = next
    const my = ++statsSeq
    panelsEl.setAttribute('aria-busy', 'true')
    const fresh = await api.stats(next).catch(() => null) as Stats | null
    // A newer request owns the panels (and aria-busy) now.
    if (my !== statsSeq) return
    panelsEl.removeAttribute('aria-busy')
    // Fetch first, commit after: a failed request must leave `period`, the
    // stored preference and the selector untouched, and a click on the same
    // period must retry instead of being swallowed by the guard below.
    if (!fresh) { requested = period; toast(t(errorKey), 'error'); return }
    period = next
    savePeriod(next)
    rangeButtons.forEach(b => b.setAttribute('aria-selected', b.dataset.period === next ? 'true' : 'false'))
    stats = fresh
    const subtitle = content.querySelector('#dash-subtitle')
    if (subtitle) subtitle.textContent = headerSubtitle(stats)
    panelsEl.innerHTML = renderPanels(stats)
    wirePanels()
  }
  const setPeriod = (next: DashboardPeriod) => {
    if (next === requested) return
    void fetchStats(next, 'dashboard.period_error')
  }
  rangeButtons.forEach(btn => {
    btn.addEventListener('click', () => setPeriod(btn.dataset.period as DashboardPeriod))
  })
  // Only rendered when the first load failed; a successful fetch replaces it.
  panelsEl.querySelector('#dash-retry')?.addEventListener('click', () => {
    void fetchStats(period, 'dashboard.load_error')
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
  // Removed on the next navigation, so visits do not pile up listeners.
  const docListeners = new AbortController()
  setNavigationCleanup(() => docListeners.abort())
  document.addEventListener('click', (e) => {
    if (!content.querySelector('#data-menu')?.contains(e.target as Node) && !dropdown.classList.contains('hidden')) {
      closeMenu()
    }
  }, { signal: docListeners.signal })
  content.querySelector('#data-menu')?.addEventListener('keydown', (e) => {
    if ((e as KeyboardEvent).key !== 'Escape' || dropdown.classList.contains('hidden')) return
    closeMenu()
    menuBtn.focus()
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
      content.querySelectorAll<HTMLElement>('.dash-panel').forEach(p => {
        p.setAttribute('tabindex', '0')
        p.setAttribute('draggable', 'true')
      })
      content.querySelector<HTMLElement>('.dash-panel')?.focus()
    } else {
      content.querySelectorAll<HTMLElement>('.dash-panel').forEach(p => {
        p.removeAttribute('tabindex')
        p.removeAttribute('draggable')
      })
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
    rerender()
  })

  setupDragAndDrop(content, saveOrder)

  // ---- Export / import ------------------------------------------------------

  content.querySelector('#export-btn')?.addEventListener('click', async () => {
    closeMenu()
    try {
      const data = await api.export()
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      // Named after the reader's own date, not the UTC one.
      a.download = `jobctrl-export-${localDayKey(new Date())}.json`
      a.click()
      // Revoking right away can cancel the download in Safari and Firefox.
      setTimeout(() => URL.revokeObjectURL(url), 30_000)
    } catch {
      toast(t('dashboard.export_error'), 'error')
    }
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
      if (result.imported > 0) setTimeout(() => rerender(), 1500)
    } catch {
      toast(t('dashboard.import_error'), 'error')
    } finally {
      importFile.value = ''
    }
  })

  return createLayout(content)
}
