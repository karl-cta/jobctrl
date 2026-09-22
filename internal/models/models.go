package models

import "time"

type ContractType string

const (
	ContractCDI        ContractType = "CDI"
	ContractCDD        ContractType = "CDD"
	ContractFreelance  ContractType = "Freelance"
	ContractInternship ContractType = "Internship"
	ContractOther      ContractType = "Other"
)

type WorkMode string

const (
	WorkModeOnsite WorkMode = "On-site"
	WorkModeHybrid WorkMode = "Hybrid"
	WorkModeRemote WorkMode = "Remote"
)

type ApplicationStatus string

const (
	StatusWishlist     ApplicationStatus = "Wishlist"
	StatusApplied      ApplicationStatus = "Applied"
	StatusScreening    ApplicationStatus = "Screening"
	StatusInterviewing ApplicationStatus = "Interviewing"
	StatusOffer        ApplicationStatus = "Offer"
	StatusAccepted     ApplicationStatus = "Accepted"
	StatusRejected     ApplicationStatus = "Rejected"
	// StatusNoReply marks an application that was sent but never answered.
	// It is set automatically once an "Applied" application has been waiting
	// for longer than JOB_CTRL_NO_REPLY_DAYS.
	StatusNoReply ApplicationStatus = "NoReply"
)

type InterviewType string

const (
	InterviewScreening InterviewType = "Screening"
	InterviewPhone     InterviewType = "Phone"
	InterviewVideo     InterviewType = "Video"
	InterviewOnsite    InterviewType = "On-site"
	InterviewTechnical InterviewType = "Technical"
	InterviewHR        InterviewType = "HR"
	InterviewCulture   InterviewType = "Culture"
	InterviewFinal     InterviewType = "Final"
)

type InterviewOutcome string

const (
	OutcomePassed    InterviewOutcome = "Passed"
	OutcomeFailed    InterviewOutcome = "Failed"
	OutcomePending   InterviewOutcome = "Pending"
	OutcomeCancelled InterviewOutcome = "Cancelled"
	OutcomeRejected  InterviewOutcome = "Rejected"
)

type Application struct {
	ID                   string            `json:"id"`
	CompanyName          string            `json:"company_name"`
	CompanyWebsite       *string           `json:"company_website"`
	CompanyIndustry      *string           `json:"company_industry"`
	CompanySize          *string           `json:"company_size"`
	CompanyLocation      *string           `json:"company_location"`
	JobTitle             string            `json:"job_title"`
	JobURL               *string           `json:"job_url"`
	JobDescription       *string           `json:"job_description"`
	ContractType         ContractType      `json:"contract_type"`
	ContractDuration     *int              `json:"contract_duration"`
	WorkMode             WorkMode          `json:"work_mode"`
	Location             *string           `json:"location"`
	Salary               *int              `json:"salary"`
	SalaryCurrency       string            `json:"salary_currency"`
	Status               ApplicationStatus `json:"status"`
	AppliedAt            *time.Time        `json:"applied_at"`
	Source               *string           `json:"source"`
	Notes                *string           `json:"notes"`
	Speech               *string           `json:"speech"`
	Rating               *int              `json:"rating"`
	Confidence           *int              `json:"confidence"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	FollowUpSnoozedUntil *string           `json:"follow_up_snoozed_until,omitempty"`

	Interviews     []Interview     `json:"interviews,omitempty"`
	Contacts       []Contact       `json:"contacts,omitempty"`
	TimelineEvents []TimelineEvent `json:"timeline_events,omitempty"`
}

type Interview struct {
	ID              string            `json:"id"`
	ApplicationID   string            `json:"application_id"`
	Round           int               `json:"round"`
	Type            InterviewType     `json:"type"`
	ScheduledAt     *time.Time        `json:"scheduled_at"`
	DurationMinutes *int              `json:"duration_minutes"`
	InterviewerName *string           `json:"interviewer_name"`
	InterviewerRole *string           `json:"interviewer_role"`
	Notes           *string           `json:"notes"`
	PrepNotes       *string           `json:"prep_notes"`
	Outcome         *InterviewOutcome `json:"outcome"`
	CreatedAt       time.Time         `json:"created_at"`
}

type Contact struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"application_id"`
	Name          string    `json:"name"`
	Role          *string   `json:"role"`
	Email         *string   `json:"email"`
	Phone         *string   `json:"phone"`
	LinkedIn      *string   `json:"linkedin"`
	Notes         *string   `json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
}

type TimelineEvent struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"application_id"`
	EventType     string    `json:"event_type"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
}

type Stats struct {
	Total              int             `json:"total"`
	ByStatus           map[string]int  `json:"by_status"`
	ResponseRate       float64         `json:"response_rate"`
	TopSources         []SourceCount   `json:"top_sources"`
	FollowUps          []FollowUpItem  `json:"follow_ups"`
	ActivityHeatmap    []ActivityDay   `json:"activity_heatmap"`
	RecentActivity     []ActivityItem  `json:"recent_activity"`
	UpcomingInterviews int             `json:"upcoming_interviews"`
	ActiveProcesses    []ActiveProcess `json:"active_processes"`
	Period             PeriodStats     `json:"period"`
	Weekly             []WeeklyPoint   `json:"weekly"`
}

// ActiveProcess summarises an application currently in Screening or
// Interviewing for the dashboard: where it stands and how long the company
// has been silent.
type ActiveProcess struct {
	ID            string         `json:"id"`
	CompanyName   string         `json:"company_name"`
	JobTitle      string         `json:"job_title"`
	Status        string         `json:"status"`
	Rounds        int            `json:"rounds"`
	LastInterview *InterviewStep `json:"last_interview"`
	NextInterview *InterviewStep `json:"next_interview"`
	SilentDays    int            `json:"silent_days"`
}

// InterviewStep is one interview as seen from the dashboard.
type InterviewStep struct {
	Round   int    `json:"round"`
	Type    string `json:"type"`
	At      string `json:"at"`
	Outcome string `json:"outcome"`
}

// KPI is a metric over the selected period: current value, previous-period
// value (nil when there is no previous period, e.g. "all time") and a short
// series of bucketed values for a sparkline.
type KPI struct {
	Value  float64   `json:"value"`
	Prev   *float64  `json:"prev"`
	Series []float64 `json:"series"`
}

// PeriodStats groups the metrics that depend on the dashboard period selector.
// Days is 0 for "all time".
type PeriodStats struct {
	Days         int         `json:"days"`
	Sent         KPI         `json:"sent"`
	Responded    KPI         `json:"responded"`
	ResponseRate KPI         `json:"response_rate"`
	Interviews   KPI         `json:"interviews"`
	Rejected     KPI         `json:"rejected"`
	NoReply      KPI         `json:"no_reply"`
	Offers       KPI         `json:"offers"`
	Funnel       FunnelStats `json:"funnel"`
}

// FunnelStats counts applications sent in the period by how far they got.
type FunnelStats struct {
	Sent         int `json:"sent"`
	Responded    int `json:"responded"`
	Interviewing int `json:"interviewing"`
	Offers       int `json:"offers"`
	Accepted     int `json:"accepted"`
}

// WeeklyPoint is one week (Monday-based) of the 12-week timeline chart.
type WeeklyPoint struct {
	WeekStart string `json:"week_start"`
	Sent      int    `json:"sent"`
	Replies   int    `json:"replies"`
}

type ActivityDay struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type ActivityItem struct {
	Time          string `json:"time"`
	EventType     string `json:"event_type"`
	Description   string `json:"description"`
	ApplicationID string `json:"application_id"`
	CompanyName   string `json:"company_name"`
	JobTitle      string `json:"job_title"`
	Status        string `json:"status"`
}

type FollowUpItem struct {
	ID              string `json:"id"`
	CompanyName     string `json:"company_name"`
	JobTitle        string `json:"job_title"`
	Status          string `json:"status"`
	LastInterviewAt string `json:"last_interview_at"`
}

type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}
