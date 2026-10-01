package handlers

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"job-ctrl/internal/extract"
	"job-ctrl/internal/models"
)

const defaultPageSize = 20
const maxPageSize = 500

// maxPage bounds ?page= so that (page-1)*limit can never overflow.
const maxPage = 1_000_000

// maxImportBytes caps an import upload. A backup of a few thousand
// applications with job descriptions is 20 MB or more, and the decoder holds
// the whole payload in memory, so the cap stays well below "unbounded".
var maxImportBytes int64 = 64 << 20

// extractSlots caps the outbound fetches /api/extract runs at once: each one
// can take 10 s and read 2 MB, and nothing else limits how many a page starts.
var extractSlots = make(chan struct{}, 4)

// validID is the shape an imported id must have to be kept. Every id the app
// generates (uuids, "migration-006-<uuid>", "auto-noreply-<uuid>") matches;
// anything else is replaced, since ids end up in URLs and HTML attributes.
var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type Handler struct {
	db *sql.DB
}

func New(db *sql.DB) *Handler {
	return &Handler{db: db}
}

// dbtx is what both *sql.DB and *sql.Tx offer. The pool has a single
// connection (SetMaxOpenConns(1)), so code running inside a transaction must
// send every statement through the tx: a call on h.db would wait forever for
// the connection the tx is holding.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON decodes the request body into dst. It answers 413 when the body
// is over the limit a MaxBytesReader put on it (the /api middleware, or
// Import's own), 400 for any other decoding error, and reports success.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// rowExists reports whether table holds a row with this id. table is always a
// constant from this package, never user input.
func rowExists(ctx context.Context, q dbtx, table, id string) (bool, error) {
	var one int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM `+table+` WHERE id = ?`, id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

var validContractTypes = map[models.ContractType]bool{
	models.ContractCDI: true, models.ContractCDD: true,
	models.ContractFreelance: true, models.ContractInternship: true, models.ContractOther: true,
}

var validWorkModes = map[models.WorkMode]bool{
	models.WorkModeOnsite: true, models.WorkModeHybrid: true, models.WorkModeRemote: true,
}

var validStatuses = map[models.ApplicationStatus]bool{
	models.StatusWishlist: true, models.StatusApplied: true,
	models.StatusScreening: true, models.StatusInterviewing: true,
	models.StatusOffer: true, models.StatusAccepted: true,
	models.StatusRejected: true, models.StatusNoReply: true,
}

// legacyStatuses maps statuses that no longer exist to their replacement,
// mirroring the SQL migrations so that old exports import cleanly.
var legacyStatuses = map[string]models.ApplicationStatus{
	"Withdrawn": models.StatusApplied, // migration 006
}

var validInterviewTypes = map[models.InterviewType]bool{
	models.InterviewScreening: true, models.InterviewPhone: true, models.InterviewVideo: true,
	models.InterviewOnsite: true, models.InterviewTechnical: true,
	models.InterviewHR: true, models.InterviewCulture: true,
	models.InterviewFinal: true,
}

var validInterviewOutcomes = map[models.InterviewOutcome]bool{
	models.OutcomePassed: true, models.OutcomeFailed: true,
	models.OutcomePending: true, models.OutcomeCancelled: true,
	models.OutcomeRejected: true,
}

// normalizeInterview fills what the schema would default to, so that an
// empty type or round 0 is not written as-is over the 'Phone' / 1 defaults.
func normalizeInterview(iv *models.Interview) {
	if iv.Type == "" {
		iv.Type = models.InterviewPhone
	}
	if iv.Round == 0 {
		iv.Round = 1
	}
}

func validateInterview(iv *models.Interview) error {
	if iv.Type != "" && !validInterviewTypes[iv.Type] {
		return fmt.Errorf("invalid type: %s", iv.Type)
	}
	if iv.Outcome != nil && !validInterviewOutcomes[*iv.Outcome] {
		return fmt.Errorf("invalid outcome: %s", *iv.Outcome)
	}
	if iv.Round < 0 {
		return fmt.Errorf("round must be non-negative")
	}
	if iv.DurationMinutes != nil && *iv.DurationMinutes < 0 {
		return fmt.Errorf("duration_minutes must be non-negative")
	}
	return nil
}

func validateContact(c *models.Contact) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

func validateApplication(a *models.Application) error {
	if strings.TrimSpace(a.CompanyName) == "" {
		return fmt.Errorf("company_name is required")
	}
	if strings.TrimSpace(a.JobTitle) == "" {
		return fmt.Errorf("job_title is required")
	}
	if a.ContractType != "" && !validContractTypes[a.ContractType] {
		return fmt.Errorf("invalid contract_type: %s", a.ContractType)
	}
	if a.WorkMode != "" && !validWorkModes[a.WorkMode] {
		return fmt.Errorf("invalid work_mode: %s", a.WorkMode)
	}
	if a.Status != "" && !validStatuses[a.Status] {
		return fmt.Errorf("invalid status: %s", a.Status)
	}
	if a.Rating != nil && (*a.Rating < 1 || *a.Rating > 5) {
		return fmt.Errorf("rating must be between 1 and 5")
	}
	return nil
}

// checkApplicationRanges validates the numeric fields the UI bounds. prev is
// the stored row on an update (nil on create): a value that did not change is
// accepted even when out of range, so a row written before these rules
// existed can still be edited. Only a new invalid value is refused.
func checkApplicationRanges(a, prev *models.Application) error {
	var oldConfidence, oldSalary, oldDuration *int
	if prev != nil {
		oldConfidence, oldSalary, oldDuration = prev.Confidence, prev.Salary, prev.ContractDuration
	}
	stored := func(v, old *int) bool { return old != nil && *old == *v }
	if c := a.Confidence; c != nil && (*c < 1 || *c > 4) && !stored(c, oldConfidence) {
		return fmt.Errorf("confidence must be between 1 and 4")
	}
	if s := a.Salary; s != nil && *s < 0 && !stored(s, oldSalary) {
		return fmt.Errorf("salary must be non-negative")
	}
	if d := a.ContractDuration; d != nil && *d < 1 && !stored(d, oldDuration) {
		return fmt.Errorf("contract_duration must be at least 1")
	}
	return nil
}

// normalizeImportedApplication clears out-of-range values instead of letting
// validation skip the whole application: losing an application on restore is
// never acceptable, a meaningless rating or salary is.
func normalizeImportedApplication(a *models.Application) {
	drop := func(field string, v **int, ok func(int) bool) {
		if *v != nil && !ok(**v) {
			log.Printf("Import: %q has out-of-range %s %d, importing it empty", a.CompanyName, field, **v)
			*v = nil
		}
	}
	drop("rating", &a.Rating, func(n int) bool { return n >= 1 && n <= 5 })
	drop("confidence", &a.Confidence, func(n int) bool { return n >= 1 && n <= 4 })
	drop("salary", &a.Salary, func(n int) bool { return n >= 0 })
	drop("contract_duration", &a.ContractDuration, func(n int) bool { return n >= 1 })
}

// trimApplication strips the spaces pasted around names from a job posting,
// which would otherwise defeat duplicate detection and split the sources.
func trimApplication(a *models.Application) {
	a.CompanyName = strings.TrimSpace(a.CompanyName)
	a.JobTitle = strings.TrimSpace(a.JobTitle)
	if a.Source != nil {
		s := strings.TrimSpace(*a.Source)
		a.Source = &s
	}
}

func paginationParams(r *http.Request) (limit, offset int) {
	limit = defaultPageSize
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if page > maxPage {
		page = maxPage
	}
	offset = (page - 1) * limit
	return limit, offset
}

// Applications

// sentStatusesSQL lists the statuses of an application that was actually
// sent (isSentStatus). The dashboard's "sent" tile links to ?sent=1.
const sentStatusesSQL = `('Applied', 'NoReply', 'Screening', 'Interviewing', 'Offer', 'Accepted', 'Rejected')`

// hasInterviewSQL: the application landed at least one real interview.
const hasInterviewSQL = `EXISTS (SELECT 1 FROM interviews i WHERE i.application_id = a.id AND COALESCE(i.outcome, '') != 'Cancelled')`

// hasReplySQL is the "replied" rule of computePeriodStats: the status says
// the company answered, or a sent application landed a real interview.
const hasReplySQL = `(a.status IN ('Screening', 'Interviewing', 'Offer', 'Accepted', 'Rejected')
	OR (a.status IN ('Applied', 'NoReply') AND ` + hasInterviewSQL + `))`

// sentAtSQL is when an application was sent (loadSentApps), normalised to
// the sqliteTime() form so that it compares as text against a sqliteTime().
const sentAtSQL = `replace(replace(COALESCE(a.applied_at, a.created_at), 'T', ' '), 'Z', '')`

const applicationColumns = `id, company_name, company_website, company_industry,
	company_size, company_location, job_title, job_url, job_description, contract_type, contract_duration, work_mode,
	location, salary, salary_currency, status, applied_at, source,
	notes, speech, rating, confidence, created_at, updated_at, follow_up_snoozed_until`

// listSorts maps each ?sort= key to a fixed SQL expression, so the key itself
// never reaches the query. Names sort ignoring case and accents ("École"
// among the E's), statuses in pipeline order rather than by English name.
var listSorts = map[string]string{
	"created_at":   "a.created_at",
	"updated_at":   "a.updated_at",
	"company_name": "jc_fold(a.company_name)",
	"job_title":    "jc_fold(a.job_title)",
	"status": `CASE a.status WHEN 'Wishlist' THEN 0 WHEN 'Applied' THEN 1 WHEN 'Screening' THEN 2
		WHEN 'Interviewing' THEN 3 WHEN 'Offer' THEN 4 WHEN 'Accepted' THEN 5 WHEN 'Rejected' THEN 6
		WHEN 'NoReply' THEN 7 ELSE 8 END`,
	"applied_at": "a.applied_at",
	"salary":     "a.salary",
	"rating":     "a.rating",
	"confidence": "a.confidence",
}

func queryFlag(v string) bool { return v == "1" || v == "true" }

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func (h *Handler) ListApplications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// The WHERE clause is built once and shared by the count and the page
	// query, so the total always describes the same rows.
	where := " WHERE 1=1"
	args := []any{}

	// status takes one status or a comma-separated list (the offers tile
	// links to status=Offer,Accepted).
	if raw := q.Get("status"); raw != "" {
		var statuses []any
		for _, s := range strings.Split(raw, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if !validStatuses[models.ApplicationStatus(s)] {
				writeError(w, http.StatusBadRequest, "invalid status")
				return
			}
			statuses = append(statuses, s)
		}
		if len(statuses) > 0 {
			where += " AND a.status IN (" + placeholders(len(statuses)) + ")"
			args = append(args, statuses...)
		}
	}
	if search := q.Get("search"); search != "" {
		// jc_fold (see fold.go) makes "école" find "École".
		where += ` AND (jc_fold(a.company_name) LIKE ? ESCAPE '\' OR jc_fold(a.job_title) LIKE ? ESCAPE '\'
			OR jc_fold(a.location) LIKE ? ESCAPE '\' OR jc_fold(a.source) LIKE ? ESCAPE '\')`
		like := "%" + escapeLike(foldText(search)) + "%"
		args = append(args, like, like, like, like)
	}
	if source := q.Get("source"); source != "" {
		where += " AND a.source = ?"
		args = append(args, source)
	}
	// sent=1: only applications actually sent.
	if queryFlag(q.Get("sent")) {
		where += " AND a.status IN " + sentStatusesSQL
	}
	// has_interviews=1: only applications that landed a non-cancelled
	// interview. The dashboard's "interviews" tile links here.
	if queryFlag(q.Get("has_interviews")) {
		where += " AND " + hasInterviewSQL
	}
	// has_reply=1: the company answered, whatever the answer. The dashboard's
	// "replies" tile links here.
	if queryFlag(q.Get("has_reply")) {
		where += " AND " + hasReplySQL
	}
	// period=30|90|365: sent within the same window as the dashboard KPIs
	// (computePeriodStats). "all", or no period at all, means no window.
	if raw := strings.TrimSpace(q.Get("period")); raw != "" {
		if days := parsePeriod(raw); days > 0 {
			now := statsNow()
			where += " AND " + sentAtSQL + " >= ? AND " + sentAtSQL + " < ?"
			args = append(args, sqliteTime(now.AddDate(0, 0, -days)), sqliteTime(now.Add(time.Second)))
		}
	}

	sortExpr, ok := listSorts[q.Get("sort")]
	if !ok {
		sortExpr = listSorts["created_at"]
	}
	dir := "DESC"
	if q.Get("dir") == "asc" {
		dir = "ASC"
	}

	var total int
	if err := h.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM applications a"+where, args...).Scan(&total); err != nil {
		log.Printf("ListApplications count: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load applications")
		return
	}

	limit, offset := paginationParams(r)
	// Empty values last in both directions, and the id as a tiebreaker so
	// that equal keys keep a stable order from one page to the next.
	query := `SELECT a.id, a.company_name, a.company_website, a.company_industry, a.company_size,
		a.company_location, a.job_title, a.job_url, a.job_description, a.contract_type, a.contract_duration, a.work_mode,
		a.location, a.salary, a.salary_currency, a.status, a.applied_at, a.source,
		a.notes, a.speech, a.rating, a.confidence, a.created_at, a.updated_at, a.follow_up_snoozed_until,
		(SELECT COUNT(*) FROM interviews WHERE application_id = a.id) as interview_count,
		(SELECT COUNT(*) FROM contacts WHERE application_id = a.id) as contact_count
		FROM applications a` + where +
		` ORDER BY ` + sortExpr + ` ` + dir + ` NULLS LAST, a.id LIMIT ? OFFSET ?`
	pageArgs := append(append([]any{}, args...), limit, offset)

	rows, err := h.db.QueryContext(r.Context(), query, pageArgs...)
	if err != nil {
		log.Printf("ListApplications query: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load applications")
		return
	}
	defer rows.Close()

	type appWithCounts struct {
		models.Application
		InterviewCount int `json:"interview_count"`
		ContactCount   int `json:"contact_count"`
	}

	apps := []appWithCounts{}
	for rows.Next() {
		var a appWithCounts
		if err := rows.Scan(
			&a.ID, &a.CompanyName, &a.CompanyWebsite, &a.CompanyIndustry, &a.CompanySize,
			&a.CompanyLocation, &a.JobTitle, &a.JobURL, &a.JobDescription,
			&a.ContractType, &a.ContractDuration, &a.WorkMode, &a.Location,
			&a.Salary, &a.SalaryCurrency, &a.Status,
			&a.AppliedAt, &a.Source, &a.Notes, &a.Speech, &a.Rating, &a.Confidence,
			&a.CreatedAt, &a.UpdatedAt, &a.FollowUpSnoozedUntil,
			&a.InterviewCount, &a.ContactCount,
		); err != nil {
			log.Printf("ListApplications scan: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load applications")
			return
		}
		apps = append(apps, a)
	}
	if err := rows.Err(); err != nil {
		log.Printf("ListApplications rows iteration: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load applications")
		return
	}

	page := offset/limit + 1
	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":        apps,
		"total":       total,
		"page":        page,
		"per_page":    limit,
		"total_pages": totalPages,
	})
}

// loadApplication reads one stored application, without its children.
func loadApplication(ctx context.Context, q dbtx, id string) (models.Application, error) {
	var a models.Application
	err := scanApplication(q.QueryRowContext(ctx, `SELECT `+applicationColumns+` FROM applications WHERE id = ?`, id), &a)
	return a, err
}

func (h *Handler) GetApplication(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := loadApplication(r.Context(), h.db, id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "application not found")
		return
	} else if err != nil {
		log.Printf("GetApplication: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load application")
		return
	}

	interviews, err := h.getInterviewsByApplication(r, id)
	if err != nil {
		log.Printf("GetApplication interviews: %v", err)
	}
	a.Interviews = interviews

	contacts, err := h.getContactsByApplication(r, id)
	if err != nil {
		log.Printf("GetApplication contacts: %v", err)
	}
	a.Contacts = contacts

	events, err := h.getTimelineByApplication(r, id)
	if err != nil {
		log.Printf("GetApplication timeline: %v", err)
	}
	a.TimelineEvents = events

	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) CreateApplication(w http.ResponseWriter, r *http.Request) {
	var a models.Application
	if !decodeJSON(w, r, &a) {
		return
	}
	trimApplication(&a)
	if a.SalaryCurrency == "" {
		a.SalaryCurrency = "EUR"
	}
	if a.Status == "" {
		a.Status = models.StatusWishlist
	}
	if a.ContractType == "" {
		a.ContractType = models.ContractCDI
	}
	if a.WorkMode == "" {
		a.WorkMode = models.WorkModeHybrid
	}
	if err := validateApplication(&a); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := checkApplicationRanges(&a, nil); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	a.ID = uuid.New().String()
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now
	// Created straight as Applied: it was sent now, as when moved to Applied.
	if a.Status == models.StatusApplied && a.AppliedAt == nil {
		a.AppliedAt = &now
	}

	var appliedAtStr *string
	if a.AppliedAt != nil {
		s := sqliteTime(*a.AppliedAt)
		appliedAtStr = &s
	}

	fail := func(what string, err error) {
		log.Printf("CreateApplication %s: %v", what, err)
		writeError(w, http.StatusInternalServerError, "could not create application")
	}
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		fail("begin", err)
		return
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `INSERT INTO applications
		(id, company_name, company_website, company_industry, company_size, company_location,
		job_title, job_url, job_description, contract_type, contract_duration, work_mode, location,
		salary, salary_currency, status, applied_at, source, notes, speech, rating, confidence,
		created_at, updated_at, follow_up_snoozed_until) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.CompanyName, a.CompanyWebsite, a.CompanyIndustry, a.CompanySize, a.CompanyLocation,
		a.JobTitle, a.JobURL, a.JobDescription, a.ContractType, a.ContractDuration, a.WorkMode, a.Location,
		a.Salary, a.SalaryCurrency, a.Status, appliedAtStr, a.Source, a.Notes, a.Speech, a.Rating, a.Confidence,
		sqliteTime(a.CreatedAt), sqliteTime(a.UpdatedAt), nil,
	)
	if err != nil {
		fail("insert", err)
		return
	}
	if err := insertTimelineEvent(ctx, tx, a.ID, "created", "Application created"); err != nil {
		fail("timeline", err)
		return
	}
	stored, err := loadApplication(ctx, tx, a.ID)
	if err != nil {
		fail("reload", err)
		return
	}
	if err := tx.Commit(); err != nil {
		fail("commit", err)
		return
	}
	writeJSON(w, http.StatusCreated, stored)
}

// UpdateApplication replaces the whole application (PUT): every field not
// sent is cleared. It answers with the row as stored.
func (h *Handler) UpdateApplication(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var a models.Application
	if !decodeJSON(w, r, &a) {
		return
	}
	trimApplication(&a)
	if a.SalaryCurrency == "" {
		a.SalaryCurrency = "EUR"
	}
	// Written as-is, an empty enum would drop the application out of every
	// status filter and statistic.
	if a.Status == "" || a.ContractType == "" || a.WorkMode == "" {
		writeError(w, http.StatusBadRequest, "status, contract_type and work_mode are required")
		return
	}
	if err := validateApplication(&a); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	fail := func(what string, err error) {
		log.Printf("UpdateApplication %s: %v", what, err)
		writeError(w, http.StatusInternalServerError, "could not update application")
	}
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		fail("begin", err)
		return
	}
	defer tx.Rollback()

	var old models.Application
	err = tx.QueryRowContext(ctx, `SELECT status, confidence, salary, contract_duration FROM applications WHERE id = ?`, id).
		Scan(&old.Status, &old.Confidence, &old.Salary, &old.ContractDuration)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "application not found")
		return
	} else if err != nil {
		fail("lookup", err)
		return
	}
	if err := checkApplicationRanges(&a, &old); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	a.UpdatedAt = now

	if a.Status == models.StatusApplied && old.Status != models.StatusApplied && a.AppliedAt == nil {
		a.AppliedAt = &now
	}

	var appliedAtStr *string
	if a.AppliedAt != nil {
		s := sqliteTime(*a.AppliedAt)
		appliedAtStr = &s
	}

	_, err = tx.ExecContext(ctx, `UPDATE applications SET
		company_name=?, company_website=?, company_industry=?, company_size=?, company_location=?,
		job_title=?, job_url=?, job_description=?, contract_type=?, contract_duration=?, work_mode=?, location=?,
		salary=?, salary_currency=?, status=?, applied_at=?, source=?,
		notes=?, speech=?, rating=?, confidence=?, updated_at=? WHERE id=?`,
		a.CompanyName, a.CompanyWebsite, a.CompanyIndustry, a.CompanySize, a.CompanyLocation,
		a.JobTitle, a.JobURL, a.JobDescription, a.ContractType, a.ContractDuration, a.WorkMode, a.Location,
		a.Salary, a.SalaryCurrency, a.Status, appliedAtStr, a.Source,
		a.Notes, a.Speech, a.Rating, a.Confidence, sqliteTime(a.UpdatedAt), id,
	)
	if err != nil {
		fail("update", err)
		return
	}

	if a.Status != old.Status {
		desc := fmt.Sprintf("Status changed from %s to %s", old.Status, a.Status)
		if err := insertTimelineEvent(ctx, tx, id, "status_change", desc); err != nil {
			fail("timeline", err)
			return
		}
	}

	stored, err := loadApplication(ctx, tx, id)
	if err != nil {
		fail("reload", err)
		return
	}
	if err := tx.Commit(); err != nil {
		fail("commit", err)
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) DeleteApplication(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	result, err := h.db.ExecContext(r.Context(), `DELETE FROM applications WHERE id=?`, id)
	if err != nil {
		log.Printf("DeleteApplication: %v", err)
		writeError(w, http.StatusInternalServerError, "could not delete application")
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		log.Printf("DeleteApplication RowsAffected: %v", err)
		writeError(w, http.StatusInternalServerError, "could not delete application")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// BulkUpdateStatus moves every listed application to one status, all or
// nothing: one transaction, the timeline events written inside it. Unknown
// ids and applications already in that status are skipped.
func (h *Handler) BulkUpdateStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs    []string                 `json:"ids"`
		Status models.ApplicationStatus `json:"status"`
		// AppliedAt is the sent date to stamp on applications moving to
		// Applied without one. The UI sends the user's local calendar day;
		// without it the server falls back to now.
		AppliedAt *time.Time `json:"applied_at"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no application IDs provided")
		return
	}
	if !validStatuses[body.Status] {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	fail := func(id string, err error) {
		log.Printf("BulkUpdateStatus %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not update applications")
	}
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		fail("begin", err)
		return
	}
	defer tx.Rollback()

	now := sqliteTime(time.Now().UTC())
	appliedAt := now
	if body.AppliedAt != nil {
		appliedAt = sqliteTime(*body.AppliedAt)
	}
	var updated int
	for _, id := range body.IDs {
		var oldStatus models.ApplicationStatus
		err := tx.QueryRowContext(ctx, `SELECT status FROM applications WHERE id = ?`, id).Scan(&oldStatus)
		if err == sql.ErrNoRows {
			continue
		} else if err != nil {
			fail(id, err)
			return
		}
		if oldStatus == body.Status {
			continue
		}
		// Moving to Applied stamps the sent date when there is none, as the
		// single update does.
		if _, err := tx.ExecContext(ctx, `UPDATE applications SET status = ?, updated_at = ?,
			applied_at = CASE WHEN ? = 'Applied' AND applied_at IS NULL THEN ? ELSE applied_at END
			WHERE id = ?`, body.Status, now, body.Status, appliedAt, id); err != nil {
			fail(id, err)
			return
		}
		desc := fmt.Sprintf("Status changed from %s to %s", oldStatus, body.Status)
		if err := insertTimelineEvent(ctx, tx, id, "status_change", desc); err != nil {
			fail(id, err)
			return
		}
		updated++
	}
	if err := tx.Commit(); err != nil {
		fail("commit", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"updated": updated})
}

// BulkDelete deletes every listed application in one transaction.
func (h *Handler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no application IDs provided")
		return
	}

	fail := func(id string, err error) {
		log.Printf("BulkDelete %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not delete applications")
	}
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		fail("begin", err)
		return
	}
	defer tx.Rollback()

	var deleted int
	for _, id := range body.IDs {
		result, err := tx.ExecContext(ctx, `DELETE FROM applications WHERE id = ?`, id)
		if err != nil {
			fail(id, err)
			return
		}
		n, err := result.RowsAffected()
		if err != nil {
			fail(id, err)
			return
		}
		deleted += int(n)
	}
	if err := tx.Commit(); err != nil {
		fail("commit", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted})
}

// Interviews

func (h *Handler) ListInterviews(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if exists, err := rowExists(r.Context(), h.db, "applications", id); err != nil {
		log.Printf("ListInterviews lookup: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load interviews")
		return
	} else if !exists {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}
	interviews, err := h.getInterviewsByApplication(r, id)
	if err != nil {
		log.Printf("ListInterviews: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load interviews")
		return
	}
	if interviews == nil {
		interviews = []models.Interview{}
	}
	writeJSON(w, http.StatusOK, interviews)
}

// loadInterview reads one stored interview.
func loadInterview(ctx context.Context, q dbtx, id string) (models.Interview, error) {
	var iv models.Interview
	err := q.QueryRowContext(ctx, `SELECT `+interviewColumns+` FROM interviews WHERE id = ?`, id).
		Scan(&iv.ID, &iv.ApplicationID, &iv.Round, &iv.Type, &iv.ScheduledAt,
			&iv.DurationMinutes, &iv.InterviewerName, &iv.InterviewerRole, &iv.Notes,
			&iv.PrepNotes, &iv.Outcome, &iv.CreatedAt)
	return iv, err
}

func (h *Handler) GetInterview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	iv, err := loadInterview(r.Context(), h.db, id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "interview not found")
		return
	} else if err != nil {
		log.Printf("GetInterview: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load interview")
		return
	}
	writeJSON(w, http.StatusOK, iv)
}

func (h *Handler) CreateInterview(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	var iv models.Interview
	if !decodeJSON(w, r, &iv) {
		return
	}
	normalizeInterview(&iv)
	if err := validateInterview(&iv); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	iv.ID = uuid.New().String()
	iv.ApplicationID = appID
	iv.CreatedAt = time.Now().UTC()

	var scheduledStr *string
	if iv.ScheduledAt != nil {
		s := sqliteTime(*iv.ScheduledAt)
		scheduledStr = &s
	}

	fail := func(what string, err error) {
		log.Printf("CreateInterview %s: %v", what, err)
		writeError(w, http.StatusInternalServerError, "could not create interview")
	}
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		fail("begin", err)
		return
	}
	defer tx.Rollback()

	if exists, err := rowExists(ctx, tx, "applications", appID); err != nil {
		fail("lookup", err)
		return
	} else if !exists {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO interviews
		(id, application_id, round, type, scheduled_at, duration_minutes, interviewer_name,
		interviewer_role, notes, prep_notes, outcome, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		iv.ID, iv.ApplicationID, iv.Round, iv.Type, scheduledStr, iv.DurationMinutes,
		iv.InterviewerName, iv.InterviewerRole, iv.Notes, iv.PrepNotes, iv.Outcome, sqliteTime(iv.CreatedAt),
	)
	if err != nil {
		fail("insert", err)
		return
	}
	desc := fmt.Sprintf("Interview round %d (%s) added", iv.Round, iv.Type)
	if err := insertTimelineEvent(ctx, tx, appID, "interview_added", desc); err != nil {
		fail("timeline", err)
		return
	}
	stored, err := loadInterview(ctx, tx, iv.ID)
	if err != nil {
		fail("reload", err)
		return
	}
	if err := tx.Commit(); err != nil {
		fail("commit", err)
		return
	}
	writeJSON(w, http.StatusCreated, stored)
}

// UpdateInterview replaces the interview's fields and answers with the row as
// stored (application_id and created_at included).
func (h *Handler) UpdateInterview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var iv models.Interview
	if !decodeJSON(w, r, &iv) {
		return
	}
	normalizeInterview(&iv)
	if err := validateInterview(&iv); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var scheduledStr *string
	if iv.ScheduledAt != nil {
		s := sqliteTime(*iv.ScheduledAt)
		scheduledStr = &s
	}

	result, err := h.db.ExecContext(r.Context(), `UPDATE interviews SET
		round=?, type=?, scheduled_at=?, duration_minutes=?, interviewer_name=?,
		interviewer_role=?, notes=?, prep_notes=?, outcome=? WHERE id=?`,
		iv.Round, iv.Type, scheduledStr, iv.DurationMinutes, iv.InterviewerName,
		iv.InterviewerRole, iv.Notes, iv.PrepNotes, iv.Outcome, id,
	)
	if err != nil {
		log.Printf("UpdateInterview: %v", err)
		writeError(w, http.StatusInternalServerError, "could not update interview")
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		log.Printf("UpdateInterview RowsAffected: %v", err)
		writeError(w, http.StatusInternalServerError, "could not update interview")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "interview not found")
		return
	}
	stored, err := loadInterview(r.Context(), h.db, id)
	if err != nil {
		log.Printf("UpdateInterview reload: %v", err)
		writeError(w, http.StatusInternalServerError, "could not update interview")
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) DeleteInterview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var appID string
	if err := h.db.QueryRowContext(r.Context(), `SELECT application_id FROM interviews WHERE id=?`, id).Scan(&appID); err != nil && err != sql.ErrNoRows {
		log.Printf("DeleteInterview lookup: %v", err)
	}
	result, err := h.db.ExecContext(r.Context(), `DELETE FROM interviews WHERE id=?`, id)
	if err != nil {
		log.Printf("DeleteInterview: %v", err)
		writeError(w, http.StatusInternalServerError, "could not delete interview")
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		log.Printf("DeleteInterview RowsAffected: %v", err)
		writeError(w, http.StatusInternalServerError, "could not delete interview")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "interview not found")
		return
	}
	if appID != "" {
		h.addTimelineEvent(r, appID, "interview_deleted", "Interview removed")
	}
	w.WriteHeader(http.StatusNoContent)
}

// Contacts

func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if exists, err := rowExists(r.Context(), h.db, "applications", id); err != nil {
		log.Printf("ListContacts lookup: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load contacts")
		return
	} else if !exists {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}
	contacts, err := h.getContactsByApplication(r, id)
	if err != nil {
		log.Printf("ListContacts: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load contacts")
		return
	}
	if contacts == nil {
		contacts = []models.Contact{}
	}
	writeJSON(w, http.StatusOK, contacts)
}

// loadContact reads one stored contact.
func loadContact(ctx context.Context, q dbtx, id string) (models.Contact, error) {
	var c models.Contact
	err := q.QueryRowContext(ctx, `SELECT `+contactColumns+` FROM contacts WHERE id = ?`, id).
		Scan(&c.ID, &c.ApplicationID, &c.Name, &c.Role, &c.Email, &c.Phone, &c.LinkedIn, &c.Notes, &c.CreatedAt)
	return c, err
}

func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := loadContact(r.Context(), h.db, id)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "contact not found")
		return
	} else if err != nil {
		log.Printf("GetContact: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load contact")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) CreateContact(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")
	var c models.Contact
	if !decodeJSON(w, r, &c) {
		return
	}
	if err := validateContact(&c); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c.ID = uuid.New().String()
	c.ApplicationID = appID
	c.CreatedAt = time.Now().UTC()

	fail := func(what string, err error) {
		log.Printf("CreateContact %s: %v", what, err)
		writeError(w, http.StatusInternalServerError, "could not create contact")
	}
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		fail("begin", err)
		return
	}
	defer tx.Rollback()

	if exists, err := rowExists(ctx, tx, "applications", appID); err != nil {
		fail("lookup", err)
		return
	} else if !exists {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO contacts
		(id, application_id, name, role, email, phone, linkedin, notes, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		c.ID, c.ApplicationID, c.Name, c.Role, c.Email, c.Phone, c.LinkedIn, c.Notes, sqliteTime(c.CreatedAt),
	)
	if err != nil {
		fail("insert", err)
		return
	}
	if err := insertTimelineEvent(ctx, tx, appID, "contact_added", fmt.Sprintf("Contact %s added", c.Name)); err != nil {
		fail("timeline", err)
		return
	}
	stored, err := loadContact(ctx, tx, c.ID)
	if err != nil {
		fail("reload", err)
		return
	}
	if err := tx.Commit(); err != nil {
		fail("commit", err)
		return
	}
	writeJSON(w, http.StatusCreated, stored)
}

// UpdateContact replaces the contact's fields and answers with the row as
// stored (application_id and created_at included).
func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var c models.Contact
	if !decodeJSON(w, r, &c) {
		return
	}
	if err := validateContact(&c); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.db.ExecContext(r.Context(), `UPDATE contacts SET name=?, role=?, email=?, phone=?, linkedin=?, notes=? WHERE id=?`,
		c.Name, c.Role, c.Email, c.Phone, c.LinkedIn, c.Notes, id)
	if err != nil {
		log.Printf("UpdateContact: %v", err)
		writeError(w, http.StatusInternalServerError, "could not update contact")
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		log.Printf("UpdateContact RowsAffected: %v", err)
		writeError(w, http.StatusInternalServerError, "could not update contact")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "contact not found")
		return
	}
	stored, err := loadContact(r.Context(), h.db, id)
	if err != nil {
		log.Printf("UpdateContact reload: %v", err)
		writeError(w, http.StatusInternalServerError, "could not update contact")
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var appID string
	if err := h.db.QueryRowContext(r.Context(), `SELECT application_id FROM contacts WHERE id=?`, id).Scan(&appID); err != nil && err != sql.ErrNoRows {
		log.Printf("DeleteContact lookup: %v", err)
	}
	result, err := h.db.ExecContext(r.Context(), `DELETE FROM contacts WHERE id=?`, id)
	if err != nil {
		log.Printf("DeleteContact: %v", err)
		writeError(w, http.StatusInternalServerError, "could not delete contact")
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		log.Printf("DeleteContact RowsAffected: %v", err)
		writeError(w, http.StatusInternalServerError, "could not delete contact")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "contact not found")
		return
	}
	if appID != "" {
		h.addTimelineEvent(r, appID, "contact_deleted", "Contact removed")
	}
	w.WriteHeader(http.StatusNoContent)
}

// Stats & Export

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats := models.Stats{ByStatus: map[string]int{}}

	// The dashboard stands on the totals: rather than show an empty tracker
	// on a database error, fail. The other panels degrade to empty and log.
	failTotals := func(what string, err error) {
		log.Printf("GetStats %s: %v", what, err)
		writeError(w, http.StatusInternalServerError, "could not load statistics")
	}
	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM applications`).Scan(&stats.Total); err != nil {
		failTotals("total", err)
		return
	}
	statusRows, err := h.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM applications GROUP BY status`)
	if err != nil {
		failTotals("byStatus", err)
		return
	}
	for statusRows.Next() {
		var status string
		var count int
		if err := statusRows.Scan(&status, &count); err != nil {
			statusRows.Close()
			failTotals("byStatus scan", err)
			return
		}
		stats.ByStatus[status] = count
	}
	statusRows.Close()
	if err := statusRows.Err(); err != nil {
		failTotals("byStatus rows", err)
		return
	}

	responded := stats.ByStatus[string(models.StatusScreening)] +
		stats.ByStatus[string(models.StatusInterviewing)] +
		stats.ByStatus[string(models.StatusOffer)] +
		stats.ByStatus[string(models.StatusAccepted)] +
		stats.ByStatus[string(models.StatusRejected)]
	// NoReply applications were sent like any other, they just never got an
	// answer: they belong to the "applied" pool but never to "responded".
	applied := stats.ByStatus[string(models.StatusApplied)] +
		stats.ByStatus[string(models.StatusNoReply)] + responded
	if applied > 0 {
		stats.ResponseRate = float64(responded) / float64(applied) * 100
	}

	if sourceRows, err := h.db.QueryContext(ctx, `SELECT source, COUNT(*) as c FROM applications WHERE source IS NOT NULL AND source != '' GROUP BY source ORDER BY c DESC, source ASC`); err != nil {
		log.Printf("GetStats topSources: %v", err)
	} else {
		defer sourceRows.Close()
		for sourceRows.Next() {
			var sc models.SourceCount
			if err := sourceRows.Scan(&sc.Source, &sc.Count); err != nil {
				log.Printf("GetStats topSources scan: %v", err)
				continue
			}
			stats.TopSources = append(stats.TopSources, sc)
		}
	}

	// Activity heatmap, bucketed by the viewer's own days (?tz=).
	if days, err := h.loadActivityHeatmap(ctx, time.Now(), requestLocation(r)); err != nil {
		log.Printf("GetStats heatmap: %v", err)
	} else {
		stats.ActivityHeatmap = days
	}

	if actRows, err := h.db.QueryContext(ctx, `SELECT te.created_at, te.event_type, te.description,
		a.id, a.company_name, a.job_title, a.status
		FROM timeline_events te
		JOIN applications a ON a.id = te.application_id
		ORDER BY replace(replace(te.created_at, 'T', ' '), 'Z', '') DESC
		LIMIT 10`); err != nil {
		log.Printf("GetStats recentActivity: %v", err)
	} else {
		defer actRows.Close()
		for actRows.Next() {
			var item models.ActivityItem
			if err := actRows.Scan(&item.Time, &item.EventType, &item.Description,
				&item.ApplicationID, &item.CompanyName, &item.JobTitle, &item.Status); err != nil {
				log.Printf("GetStats recentActivity scan: %v", err)
				continue
			}
			stats.RecentActivity = append(stats.RecentActivity, item)
		}
	}

	// Follow-ups: active processes whose last real interview is more than ten
	// days old. A cancelled interview never took place, so it is neither the
	// "last interview" nor a reason to follow up.
	nowStr := sqliteTime(time.Now().UTC())
	tenDaysAgo := sqliteTime(time.Now().UTC().AddDate(0, 0, -10))
	if fuRows, err := h.db.QueryContext(ctx, `SELECT a.id, a.company_name, a.job_title, a.status, MAX(i.scheduled_at) as last_iv
		FROM applications a JOIN interviews i ON i.application_id = a.id
		WHERE a.status IN ('Screening', 'Interviewing')
		  AND (a.follow_up_snoozed_until IS NULL OR a.follow_up_snoozed_until < ?)
		  AND i.scheduled_at IS NOT NULL
		  AND COALESCE(i.outcome, '') != 'Cancelled'
		GROUP BY a.id
		HAVING MAX(replace(i.scheduled_at, 'T', ' ')) < ?`, nowStr, tenDaysAgo); err != nil {
		log.Printf("GetStats followUps: %v", err)
	} else {
		defer fuRows.Close()
		for fuRows.Next() {
			var item models.FollowUpItem
			if err := fuRows.Scan(&item.ID, &item.CompanyName, &item.JobTitle, &item.Status, &item.LastInterviewAt); err != nil {
				log.Printf("GetStats followUps scan: %v", err)
				continue
			}
			stats.FollowUps = append(stats.FollowUps, item)
		}
	}

	now := statsNow()
	sentApps := h.loadSentApps(ctx)
	replyEvents := h.loadReplyEvents(ctx)
	interviewTimes := h.loadInterviewTimes(ctx)
	stats.Period = computePeriodStats(now, parsePeriod(r.URL.Query().Get("period")), sentApps, interviewTimes)
	stats.Weekly = computeWeekly(now, sentApps, replyEvents)
	stats.UpcomingInterviews = countUpcomingInterviews(now, interviewTimes)
	stats.ActiveProcesses = h.loadActiveProcesses(ctx, now, replyEvents)

	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) SnoozeFollowUp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Until *string `json:"until"`
		Skip  bool    `json:"skip"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	var snoozedUntil string
	if body.Skip {
		snoozedUntil = "9999-12-31 23:59:59"
	} else if body.Until != nil {
		t, err := time.Parse("2006-01-02", *body.Until)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid date format, expected YYYY-MM-DD")
			return
		}
		if t.Before(time.Now().UTC()) {
			writeError(w, http.StatusBadRequest, "snooze date must be in the future")
			return
		}
		snoozedUntil = sqliteTime(t)
	} else {
		writeError(w, http.StatusBadRequest, "must provide 'until' date or 'skip: true'")
		return
	}

	result, err := h.db.ExecContext(r.Context(),
		`UPDATE applications SET follow_up_snoozed_until = ?, updated_at = ? WHERE id = ?`,
		snoozedUntil, sqliteTime(time.Now().UTC()), id)
	if err != nil {
		log.Printf("SnoozeFollowUp: %v", err)
		writeError(w, http.StatusInternalServerError, "could not update application")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) CheckDuplicates(w http.ResponseWriter, r *http.Request) {
	companyName := foldText(r.URL.Query().Get("company_name"))
	if companyName == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Both sides are folded (see fold.go), so "Société Générale " saved from
	// a job posting is found again from "societe generale".
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, company_name, job_title, status, created_at FROM applications WHERE jc_fold(company_name) = ?`,
		companyName)
	if err != nil {
		log.Printf("CheckDuplicates: %v", err)
		writeError(w, http.StatusInternalServerError, "could not check duplicates")
		return
	}
	defer rows.Close()

	type dupResult struct {
		ID          string `json:"id"`
		CompanyName string `json:"company_name"`
		JobTitle    string `json:"job_title"`
		Status      string `json:"status"`
		CreatedAt   string `json:"created_at"`
	}
	results := []dupResult{}
	for rows.Next() {
		var d dupResult
		if err := rows.Scan(&d.ID, &d.CompanyName, &d.JobTitle, &d.Status, &d.CreatedAt); err != nil {
			log.Printf("CheckDuplicates scan: %v", err)
			writeError(w, http.StatusInternalServerError, "could not check duplicates")
			return
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		log.Printf("CheckDuplicates rows iteration: %v", err)
		writeError(w, http.StatusInternalServerError, "could not check duplicates")
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handler) ListSources(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT DISTINCT source FROM applications WHERE source IS NOT NULL AND source != '' ORDER BY source`)
	if err != nil {
		log.Printf("ListSources: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load sources")
		return
	}
	defer rows.Close()

	sources := make([]string, 0)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			log.Printf("ListSources scan: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load sources")
			return
		}
		sources = append(sources, s)
	}
	if err := rows.Err(); err != nil {
		log.Printf("ListSources rows iteration: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load sources")
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.db.QueryContext(ctx, `SELECT `+applicationColumns+` FROM applications ORDER BY created_at`)
	if err != nil {
		log.Printf("Export query: %v", err)
		writeError(w, http.StatusInternalServerError, "could not export data")
		return
	}
	var apps []models.Application
	for rows.Next() {
		var a models.Application
		if err := scanApplication(rows, &a); err != nil {
			rows.Close()
			log.Printf("Export scan: %v", err)
			writeError(w, http.StatusInternalServerError, "could not export data")
			return
		}
		apps = append(apps, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Printf("Export rows iteration: %v", err)
		writeError(w, http.StatusInternalServerError, "could not export data")
		return
	}

	// The children come in three queries rather than three per application,
	// and any failure fails the export: the export is the user's backup, and
	// one silently missing its interviews only shows up on restore.
	interviews, err := collectInterviews(h.db.QueryContext(ctx, `SELECT `+interviewColumns+`
		FROM interviews ORDER BY application_id, round, created_at`))
	if err != nil {
		log.Printf("Export interviews: %v", err)
		writeError(w, http.StatusInternalServerError, "could not export data")
		return
	}
	contacts, err := collectContacts(h.db.QueryContext(ctx, `SELECT `+contactColumns+`
		FROM contacts ORDER BY application_id, created_at`))
	if err != nil {
		log.Printf("Export contacts: %v", err)
		writeError(w, http.StatusInternalServerError, "could not export data")
		return
	}
	events, err := collectTimeline(h.db.QueryContext(ctx, `SELECT `+timelineColumns+`
		FROM timeline_events ORDER BY application_id, created_at`))
	if err != nil {
		log.Printf("Export timeline: %v", err)
		writeError(w, http.StatusInternalServerError, "could not export data")
		return
	}

	ivByApp := map[string][]models.Interview{}
	for _, iv := range interviews {
		ivByApp[iv.ApplicationID] = append(ivByApp[iv.ApplicationID], iv)
	}
	ctByApp := map[string][]models.Contact{}
	for _, c := range contacts {
		ctByApp[c.ApplicationID] = append(ctByApp[c.ApplicationID], c)
	}
	evByApp := map[string][]models.TimelineEvent{}
	for _, e := range events {
		evByApp[e.ApplicationID] = append(evByApp[e.ApplicationID], e)
	}
	for i := range apps {
		apps[i].Interviews = ivByApp[apps[i].ID]
		apps[i].Contacts = ctByApp[apps[i].ID]
		apps[i].TimelineEvents = evByApp[apps[i].ID]
	}

	writeJSON(w, http.StatusOK, map[string]any{"applications": apps, "exported_at": time.Now().UTC()})
}

// Import restores an export in one transaction: a database error or a
// dropped connection leaves the database exactly as it was, never half
// restored. Rows that fail validation are skipped and logged; everything
// else is kept, if need be under a new id.
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)

	var payload struct {
		Applications []models.Application `json:"applications"`
	}
	// Decoded before the transaction starts, so the only connection is not
	// held while the upload comes in.
	if !decodeJSON(w, r, &payload) {
		return
	}
	if len(payload.Applications) == 0 {
		writeError(w, http.StatusBadRequest, "no applications to import")
		return
	}

	fail := func(what string, err error) {
		log.Printf("Import %s: %v (nothing imported)", what, err)
		writeError(w, http.StatusInternalServerError, "import failed, nothing was imported")
	}
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		fail("begin", err)
		return
	}
	defer tx.Rollback()

	// childID keeps an imported child id when it is well-formed and free, and
	// replaces it otherwise: a malformed id could end up in the page's HTML,
	// and a taken one would make the insert fail. The row is kept either way.
	childID := func(table, id string) (string, error) {
		if validID.MatchString(id) {
			taken, err := rowExists(ctx, tx, table, id)
			if err != nil || !taken {
				return id, err
			}
		}
		return uuid.New().String(), nil
	}

	var imported, skipped, present int
	now := time.Now().UTC()

	for _, a := range payload.Applications {
		trimApplication(&a)
		if a.SalaryCurrency == "" {
			a.SalaryCurrency = "EUR"
		}
		if a.Status == "" {
			a.Status = models.StatusWishlist
		}
		if a.ContractType == "" {
			a.ContractType = models.ContractCDI
		}
		if a.WorkMode == "" {
			a.WorkMode = models.WorkModeHybrid
		}
		// Legacy statuses from older exports are mapped instead of dropped:
		// losing a whole application on restore is never acceptable.
		if mapped, ok := legacyStatuses[string(a.Status)]; ok {
			log.Printf("Import: %q has legacy status %q, importing as %q", a.CompanyName, a.Status, mapped)
			a.Status = mapped
		}
		normalizeImportedApplication(&a)
		if err := validateApplication(&a); err != nil {
			log.Printf("Import: skipping %q: %v", a.CompanyName, err)
			skipped++
			continue
		}

		// Checked on the id as exported, so that re-importing a backup stays a
		// no-op, even for ids an older version let through.
		if a.ID != "" {
			exists, err := rowExists(ctx, tx, "applications", a.ID)
			if err != nil {
				fail("duplicate check", err)
				return
			}
			if exists {
				present++
				skipped++
				continue
			}
		}
		if !validID.MatchString(a.ID) {
			if a.ID != "" {
				log.Printf("Import: %q has a malformed id, importing it under a new one", a.CompanyName)
			}
			a.ID = uuid.New().String()
		}

		if a.CreatedAt.IsZero() {
			a.CreatedAt = now
		}
		if a.UpdatedAt.IsZero() {
			a.UpdatedAt = now
		}

		var appliedAtStr *string
		if a.AppliedAt != nil {
			s := sqliteTime(*a.AppliedAt)
			appliedAtStr = &s
		}

		snoozedStr := a.FollowUpSnoozedUntil
		if _, err := tx.ExecContext(ctx, `INSERT INTO applications
			(id, company_name, company_website, company_industry, company_size, company_location,
			job_title, job_url, job_description, contract_type, contract_duration, work_mode, location,
			salary, salary_currency, status, applied_at, source, notes, speech, rating, confidence,
			created_at, updated_at, follow_up_snoozed_until) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.CompanyName, a.CompanyWebsite, a.CompanyIndustry, a.CompanySize, a.CompanyLocation,
			a.JobTitle, a.JobURL, a.JobDescription, a.ContractType, a.ContractDuration, a.WorkMode, a.Location,
			a.Salary, a.SalaryCurrency, a.Status, appliedAtStr, a.Source, a.Notes, a.Speech, a.Rating, a.Confidence,
			sqliteTime(a.CreatedAt), sqliteTime(a.UpdatedAt), snoozedStr,
		); err != nil {
			fail(fmt.Sprintf("application %q", a.CompanyName), err)
			return
		}

		// The id was free, so any child still pointing at it is an orphan left
		// by a delete made while foreign keys were not enforced. The backup's
		// version replaces it rather than being mixed with it.
		for _, table := range []string{"interviews", "contacts", "timeline_events"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE application_id = ?`, a.ID); err != nil {
				fail("orphan cleanup", err)
				return
			}
		}

		for _, iv := range a.Interviews {
			iv.ApplicationID = a.ID
			if iv.CreatedAt.IsZero() {
				iv.CreatedAt = now
			}
			// Keep interviews an older version stored with values the API
			// now refuses.
			normalizeInterview(&iv)
			if iv.Round < 1 {
				iv.Round = 1
			}
			if iv.DurationMinutes != nil && *iv.DurationMinutes < 0 {
				iv.DurationMinutes = nil
			}
			if err := validateInterview(&iv); err != nil {
				log.Printf("Import: skipping an interview of %q: %v", a.CompanyName, err)
				continue
			}
			if iv.ID, err = childID("interviews", iv.ID); err != nil {
				fail("interview id check", err)
				return
			}
			var scheduledStr *string
			if iv.ScheduledAt != nil {
				s := sqliteTime(*iv.ScheduledAt)
				scheduledStr = &s
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO interviews
				(id, application_id, round, type, scheduled_at, duration_minutes, interviewer_name,
				interviewer_role, notes, prep_notes, outcome, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				iv.ID, iv.ApplicationID, iv.Round, iv.Type, scheduledStr, iv.DurationMinutes,
				iv.InterviewerName, iv.InterviewerRole, iv.Notes, iv.PrepNotes, iv.Outcome, sqliteTime(iv.CreatedAt),
			); err != nil {
				fail(fmt.Sprintf("interview of %q", a.CompanyName), err)
				return
			}
		}

		for _, c := range a.Contacts {
			c.ApplicationID = a.ID
			if c.CreatedAt.IsZero() {
				c.CreatedAt = now
			}
			if err := validateContact(&c); err != nil {
				log.Printf("Import: skipping a contact of %q: %v", a.CompanyName, err)
				continue
			}
			if c.ID, err = childID("contacts", c.ID); err != nil {
				fail("contact id check", err)
				return
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO contacts
				(id, application_id, name, role, email, phone, linkedin, notes, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
				c.ID, c.ApplicationID, c.Name, c.Role, c.Email, c.Phone, c.LinkedIn, c.Notes, sqliteTime(c.CreatedAt),
			); err != nil {
				fail(fmt.Sprintf("contact of %q", a.CompanyName), err)
				return
			}
		}

		for _, e := range a.TimelineEvents {
			e.ApplicationID = a.ID
			if e.CreatedAt.IsZero() {
				e.CreatedAt = now
			}
			if e.ID, err = childID("timeline_events", e.ID); err != nil {
				fail("timeline id check", err)
				return
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO timeline_events
				(id, application_id, event_type, description, created_at) VALUES (?,?,?,?,?)`,
				e.ID, e.ApplicationID, e.EventType, e.Description, sqliteTime(e.CreatedAt),
			); err != nil {
				fail(fmt.Sprintf("timeline event of %q", a.CompanyName), err)
				return
			}
		}

		imported++
	}

	if err := tx.Commit(); err != nil {
		fail("commit", err)
		return
	}
	if present > 0 {
		log.Printf("Import: %d application(s) already present, left as they are", present)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"total":    len(payload.Applications),
	})
}

func (h *Handler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, company_name, company_website, company_industry,
		company_size, company_location, job_title, job_url, job_description, contract_type, contract_duration, work_mode,
		location, salary, salary_currency, status, applied_at, source,
		notes, speech, rating, confidence, created_at, updated_at, follow_up_snoozed_until FROM applications ORDER BY created_at`)
	if err != nil {
		log.Printf("ExportCSV query: %v", err)
		writeError(w, http.StatusInternalServerError, "could not export data")
		return
	}
	var apps []models.Application
	for rows.Next() {
		var a models.Application
		if err := scanApplication(rows, &a); err != nil {
			rows.Close()
			log.Printf("ExportCSV scan: %v", err)
			writeError(w, http.StatusInternalServerError, "could not export data")
			return
		}
		apps = append(apps, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Printf("ExportCSV rows iteration: %v", err)
		writeError(w, http.StatusInternalServerError, "could not export data")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="jobctrl-export.csv"`)
	w.WriteHeader(http.StatusOK)

	// UTF-8 BOM for Excel compatibility
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	defer cw.Flush()

	cw.Write([]string{
		"Company", "Website", "Industry", "Company Size", "Company Location",
		"Job Title", "Job URL", "Contract", "Duration (months)", "Work Mode",
		"Location", "Salary", "Status", "Applied At", "Source",
		"Rating", "Confidence", "Created At",
	})

	for _, a := range apps {
		cw.Write([]string{
			csvSafe(a.CompanyName), csvSafe(ptrStr(a.CompanyWebsite)), csvSafe(ptrStr(a.CompanyIndustry)),
			csvSafe(ptrStr(a.CompanySize)), csvSafe(ptrStr(a.CompanyLocation)),
			csvSafe(a.JobTitle), csvSafe(ptrStr(a.JobURL)), string(a.ContractType), ptrIntStr(a.ContractDuration),
			string(a.WorkMode), csvSafe(ptrStr(a.Location)),
			ptrIntStr(a.Salary), string(a.Status),
			ptrTimeStr(a.AppliedAt), csvSafe(ptrStr(a.Source)),
			ptrIntStr(a.Rating), ptrIntStr(a.Confidence),
			a.CreatedAt.Format("2006-01-02"),
		})
	}
}

// csvSafe prefixes strings starting with formula-trigger characters to prevent
// CSV injection when opened in spreadsheet applications.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrIntStr(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

func ptrTimeStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// Helpers

type scanner interface {
	Scan(dest ...any) error
}

func scanApplication(s scanner, a *models.Application) error {
	return s.Scan(
		&a.ID, &a.CompanyName, &a.CompanyWebsite, &a.CompanyIndustry, &a.CompanySize,
		&a.CompanyLocation, &a.JobTitle, &a.JobURL, &a.JobDescription,
		&a.ContractType, &a.ContractDuration, &a.WorkMode, &a.Location,
		&a.Salary, &a.SalaryCurrency, &a.Status,
		&a.AppliedAt, &a.Source, &a.Notes, &a.Speech, &a.Rating, &a.Confidence,
		&a.CreatedAt, &a.UpdatedAt, &a.FollowUpSnoozedUntil,
	)
}

const interviewColumns = `id, application_id, round, type, scheduled_at,
	duration_minutes, interviewer_name, interviewer_role, notes, prep_notes, outcome, created_at`

const contactColumns = `id, application_id, name, role, email, phone, linkedin, notes, created_at`

const timelineColumns = `id, application_id, event_type, description, created_at`

// collectInterviews reads every row of a query on interviewColumns. It takes
// QueryContext's two results as they come, and closes the rows before
// returning, which the single connection requires before the next query.
func collectInterviews(rows *sql.Rows, err error) ([]models.Interview, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Interview
	for rows.Next() {
		var iv models.Interview
		if err := rows.Scan(&iv.ID, &iv.ApplicationID, &iv.Round, &iv.Type, &iv.ScheduledAt,
			&iv.DurationMinutes, &iv.InterviewerName, &iv.InterviewerRole, &iv.Notes,
			&iv.PrepNotes, &iv.Outcome, &iv.CreatedAt); err != nil {
			return list, fmt.Errorf("scan interview: %w", err)
		}
		list = append(list, iv)
	}
	return list, rows.Err()
}

// collectContacts is collectInterviews for contactColumns.
func collectContacts(rows *sql.Rows, err error) ([]models.Contact, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Contact
	for rows.Next() {
		var c models.Contact
		if err := rows.Scan(&c.ID, &c.ApplicationID, &c.Name, &c.Role, &c.Email, &c.Phone, &c.LinkedIn, &c.Notes, &c.CreatedAt); err != nil {
			return list, fmt.Errorf("scan contact: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// collectTimeline is collectInterviews for timelineColumns.
func collectTimeline(rows *sql.Rows, err error) ([]models.TimelineEvent, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.TimelineEvent
	for rows.Next() {
		var e models.TimelineEvent
		if err := rows.Scan(&e.ID, &e.ApplicationID, &e.EventType, &e.Description, &e.CreatedAt); err != nil {
			return list, fmt.Errorf("scan timeline event: %w", err)
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func (h *Handler) getInterviewsByApplication(r *http.Request, appID string) ([]models.Interview, error) {
	return collectInterviews(h.db.QueryContext(r.Context(), `SELECT `+interviewColumns+`
		FROM interviews WHERE application_id=? ORDER BY round, created_at`, appID))
}

func (h *Handler) getContactsByApplication(r *http.Request, appID string) ([]models.Contact, error) {
	return collectContacts(h.db.QueryContext(r.Context(), `SELECT `+contactColumns+`
		FROM contacts WHERE application_id=? ORDER BY created_at`, appID))
}

func (h *Handler) getTimelineByApplication(r *http.Request, appID string) ([]models.TimelineEvent, error) {
	return collectTimeline(h.db.QueryContext(r.Context(), `SELECT `+timelineColumns+`
		FROM timeline_events WHERE application_id=? ORDER BY created_at`, appID))
}

// insertTimelineEvent writes one timeline event through q: the transaction,
// when the event records a change made inside one, so both land or neither.
func insertTimelineEvent(ctx context.Context, q dbtx, appID, eventType, desc string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO timeline_events (id, application_id, event_type, description, created_at) VALUES (?,?,?,?,?)`,
		uuid.New().String(), appID, eventType, desc, sqliteTime(time.Now().UTC()))
	return err
}

func (h *Handler) addTimelineEvent(r *http.Request, appID, eventType, desc string) {
	if err := insertTimelineEvent(r.Context(), h.db, appID, eventType, desc); err != nil {
		log.Printf("addTimelineEvent: %v", err)
	}
}

func (h *Handler) ExtractFromURL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	// Wait for a free slot, but never longer than the client does.
	select {
	case extractSlots <- struct{}{}:
		defer func() { <-extractSlots }()
	case <-r.Context().Done():
		writeError(w, http.StatusServiceUnavailable, "extraction cancelled")
		return
	}

	result, err := extract.FromURLContext(r.Context(), body.URL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// sqliteTime formats t the way every datetime is stored: in UTC, so that a
// time sent with an offset lands on the right instant (and day).
func sqliteTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}
