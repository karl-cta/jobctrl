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
	StatusWithdrawn    ApplicationStatus = "Withdrawn"
)

type InterviewType string

const (
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
)

type Application struct {
	ID              string            `json:"id"`
	CompanyName     string            `json:"company_name"`
	CompanyWebsite  *string           `json:"company_website"`
	CompanyIndustry *string           `json:"company_industry"`
	CompanySize     *string           `json:"company_size"`
	CompanyLocation *string           `json:"company_location"`
	JobTitle        string            `json:"job_title"`
	JobURL          *string           `json:"job_url"`
	JobDescription  *string           `json:"job_description"`
	ContractType    ContractType      `json:"contract_type"`
	WorkMode        WorkMode          `json:"work_mode"`
	Location        *string           `json:"location"`
	Salary          *int              `json:"salary"`
	SalaryCurrency  string            `json:"salary_currency"`
	Status          ApplicationStatus `json:"status"`
	AppliedAt       *time.Time        `json:"applied_at"`
	Source          *string           `json:"source"`
	Notes           *string           `json:"notes"`
	Speech          *string           `json:"speech"`
	Rating          *int              `json:"rating"`
	Confidence      *int              `json:"confidence"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`

	Interviews     []Interview     `json:"interviews,omitempty"`
	Contacts       []Contact       `json:"contacts,omitempty"`
	TimelineEvents []TimelineEvent `json:"timeline_events,omitempty"`
}

type Interview struct {
	ID                string           `json:"id"`
	ApplicationID     string           `json:"application_id"`
	Round             int              `json:"round"`
	Type              InterviewType    `json:"type"`
	ScheduledAt       *time.Time       `json:"scheduled_at"`
	DurationMinutes   *int             `json:"duration_minutes"`
	InterviewerName   *string          `json:"interviewer_name"`
	InterviewerRole   *string          `json:"interviewer_role"`
	Notes             *string          `json:"notes"`
	PrepNotes         *string          `json:"prep_notes"`
	Outcome           *InterviewOutcome `json:"outcome"`
	CreatedAt         time.Time        `json:"created_at"`
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
	Total              int                `json:"total"`
	ByStatus           map[string]int     `json:"by_status"`
	ResponseRate       float64            `json:"response_rate"`
	OfferRate          float64            `json:"offer_rate"`
	ActiveInterviews   int                `json:"active_interviews"`
	AvgSalary          *float64           `json:"avg_salary"`
	SalaryDistribution []SalaryBucket     `json:"salary_distribution"`
	TopSources         []SourceCount      `json:"top_sources"`
	OverTime           []TimeSeriesPoint  `json:"over_time"`
	AvgDaysInStatus    map[string]float64 `json:"avg_days_in_status"`
}

type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

type TimeSeriesPoint struct {
	Period string `json:"period"`
	Count  int    `json:"count"`
}

type SalaryBucket struct {
	Range string `json:"range"`
	Count int    `json:"count"`
}
