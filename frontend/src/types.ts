import { t } from './i18n'

export type ContractType = 'CDI' | 'CDD' | 'Freelance' | 'Internship' | 'Other'
export type WorkMode = 'On-site' | 'Hybrid' | 'Remote'
export type ApplicationStatus =
  | 'Wishlist'
  | 'Applied'
  | 'Screening'
  | 'Interviewing'
  | 'Offer'
  | 'Accepted'
  | 'Rejected'
  | 'NoReply'

export type InterviewType = 'Screening' | 'Phone' | 'Video' | 'On-site' | 'Technical' | 'HR' | 'Culture' | 'Final'
export type InterviewOutcome = 'Passed' | 'Failed' | 'Pending' | 'Cancelled' | 'Rejected'

export interface Application {
  id: string
  company_name: string
  company_website?: string
  company_industry?: string
  company_size?: string
  company_location?: string
  job_title: string
  job_url?: string
  job_description?: string
  contract_type: ContractType
  contract_duration?: number
  work_mode: WorkMode
  location?: string
  salary?: number
  salary_currency: string
  status: ApplicationStatus
  applied_at?: string
  source?: string
  notes?: string
  speech?: string
  rating?: number
  confidence?: number
  created_at: string
  updated_at: string
  interviews?: Interview[]
  contacts?: Contact[]
  timeline_events?: TimelineEvent[]
}

export interface Interview {
  id: string
  application_id: string
  round: number
  type: InterviewType
  scheduled_at?: string
  duration_minutes?: number
  interviewer_name?: string
  interviewer_role?: string
  notes?: string
  prep_notes?: string
  outcome?: InterviewOutcome
  created_at: string
}

export interface Contact {
  id: string
  application_id: string
  name: string
  role?: string
  email?: string
  phone?: string
  linkedin?: string
  notes?: string
  created_at: string
}

export interface TimelineEvent {
  id: string
  application_id: string
  event_type: string
  description: string
  created_at: string
}

export interface Stats {
  total: number
  by_status: Record<ApplicationStatus, number>
  response_rate: number
  top_sources: Array<{ source: string; count: number }>
  follow_ups?: FollowUpItem[]
  activity_heatmap?: ActivityDay[]
  recent_activity?: ActivityItem[]
  upcoming_interviews?: number
  active_processes?: ActiveProcess[]
  period?: PeriodStats
  weekly?: WeeklyPoint[]
}

/** A metric over the selected period: current value, previous-period value
 *  (null when there is no previous period, e.g. "all time") and 8 bucketed
 *  values for the sparkline. */
export interface KPI {
  value: number
  prev: number | null
  series: number[]
}

export interface FunnelStats {
  sent: number
  responded: number
  interviewing: number
  offers: number
  accepted: number
}

/** Metrics that depend on the dashboard period selector. `days` is 0 for "all time". */
export interface PeriodStats {
  days: number
  sent: KPI
  responded: KPI
  response_rate: KPI
  interviews: KPI
  rejected: KPI
  no_reply: KPI
  offers: KPI
  funnel: FunnelStats
}

/** One interview as seen from the dashboard. `at` is a UTC datetime string
 *  (`2006-01-02 15:04:05`). */
export interface InterviewStep {
  round: number
  type: string
  at: string
  outcome: string
}

/** An application currently in Screening or Interviewing: where it stands and
 *  how long the company has been silent. */
export interface ActiveProcess {
  id: string
  company_name: string
  job_title: string
  status: 'Screening' | 'Interviewing'
  rounds: number
  last_interview: InterviewStep | null
  next_interview: InterviewStep | null
  silent_days: number
}

/** One Monday-based week of the 12-week timeline chart. */
export interface WeeklyPoint {
  week_start: string
  sent: number
  replies: number
}

export type DashboardPeriod = '30' | '90' | '365' | 'all'

export interface ActivityDay {
  date: string
  count: number
}

export interface ActivityItem {
  time: string
  event_type: string
  description: string
  application_id: string
  company_name: string
  job_title: string
  status: ApplicationStatus
}

export interface FollowUpItem {
  id: string
  company_name: string
  job_title: string
  status: string
  last_interview_at: string
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  per_page: number
  total_pages: number
}

export function statusLabel(status: ApplicationStatus): string {
  return t(`status.${status}`)
}

/** Localised contract type; unknown values pass through unchanged. */
export function contractLabel(value: string): string {
  const key = `contract.${value}`
  const label = t(key)
  return label === key ? value : label
}

/** Localised work mode; unknown values pass through unchanged. */
export function workModeLabel(value: string): string {
  const key = `work_mode.${value}`
  const label = t(key)
  return label === key ? value : label
}

/** Localised interview type; unknown values pass through unchanged. */
export function interviewTypeLabel(type: string): string {
  const key = `interview.type.${type}`
  const label = t(key)
  return label === key ? type : label
}

/** Localised interview outcome; unknown values pass through unchanged. */
export function interviewOutcomeLabel(outcome: string): string {
  const key = `interview.outcome.${outcome}`
  const label = t(key)
  return label === key ? outcome : label
}

export const STATUS_COLORS: Record<ApplicationStatus, string> = {
  Wishlist: 'bg-stone-100 text-stone-600 dark:bg-stone-800/60 dark:text-stone-400',
  Applied: 'bg-sky-50 text-sky-700 dark:bg-sky-500/15 dark:text-sky-400',
  Screening: 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-400',
  Interviewing: 'bg-orange-50 text-orange-700 dark:bg-orange-500/15 dark:text-orange-400',
  Offer: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-400',
  Accepted: 'bg-teal-50 text-teal-700 dark:bg-teal-500/15 dark:text-teal-400',
  Rejected: 'bg-rose-50 text-rose-600 dark:bg-rose-500/15 dark:text-rose-400',
  NoReply: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-400',
}

export const ALL_STATUSES: ApplicationStatus[] = [
  'Wishlist', 'Applied', 'Screening', 'Interviewing', 'Offer', 'Accepted', 'Rejected', 'NoReply',
]
