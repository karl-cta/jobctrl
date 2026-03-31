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
  | 'Withdrawn'

export type InterviewType = 'Phone' | 'Video' | 'On-site' | 'Technical' | 'HR' | 'Culture' | 'Final'
export type InterviewOutcome = 'Passed' | 'Failed' | 'Pending' | 'Cancelled'

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
  work_mode: WorkMode
  location?: string
  salary_min?: number
  salary_max?: number
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
  offer_rate: number
  active_interviews: number
  avg_salary_min?: number
  avg_salary_max?: number
  salary_distribution?: Array<{ range: string; count: number }>
  top_sources: Array<{ source: string; count: number }>
  over_time?: Array<{ period: string; count: number }>
  avg_days_in_status?: Record<string, number>
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

export const STATUS_COLORS: Record<ApplicationStatus, string> = {
  Wishlist: 'bg-stone-100 text-stone-600 dark:bg-stone-800/60 dark:text-stone-400',
  Applied: 'bg-sky-50 text-sky-700 dark:bg-sky-500/15 dark:text-sky-400',
  Screening: 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-400',
  Interviewing: 'bg-orange-50 text-orange-700 dark:bg-orange-500/15 dark:text-orange-400',
  Offer: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-400',
  Accepted: 'bg-teal-50 text-teal-700 dark:bg-teal-500/15 dark:text-teal-400',
  Rejected: 'bg-rose-50 text-rose-600 dark:bg-rose-500/15 dark:text-rose-400',
  Withdrawn: 'bg-stone-100 text-stone-500 dark:bg-stone-800/40 dark:text-stone-500',
}

export const ALL_STATUSES: ApplicationStatus[] = [
  'Wishlist', 'Applied', 'Screening', 'Interviewing', 'Offer', 'Accepted', 'Rejected', 'Withdrawn',
]
