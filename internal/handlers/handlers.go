package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

type Handler struct {
	db *sql.DB
}

func New(db *sql.DB) *Handler {
	return &Handler{db: db}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
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
	offset = (page - 1) * limit
	return limit, offset
}

// Applications

func (h *Handler) ListApplications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	statusFilter := q.Get("status")
	search := q.Get("search")
	sortBy := q.Get("sort")
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortDir := q.Get("dir")

	query := `SELECT a.id, a.company_name, a.company_website, a.company_industry, a.company_size,
		a.company_location, a.job_title, a.job_url, a.job_description, a.contract_type, a.contract_duration, a.work_mode,
		a.location, a.salary, a.salary_currency, a.status, a.applied_at, a.source,
		a.notes, a.speech, a.rating, a.confidence, a.created_at, a.updated_at, a.follow_up_snoozed_until,
		(SELECT COUNT(*) FROM interviews WHERE application_id = a.id) as interview_count,
		(SELECT COUNT(*) FROM contacts WHERE application_id = a.id) as contact_count
		FROM applications a WHERE 1=1`
	args := []any{}

	if statusFilter != "" {
		query += " AND a.status = ?"
		args = append(args, statusFilter)
	}
	if search != "" {
		query += " AND (a.company_name LIKE ? ESCAPE '\\' OR a.job_title LIKE ? ESCAPE '\\' OR a.location LIKE ? ESCAPE '\\' OR a.source LIKE ? ESCAPE '\\')"
		like := "%" + escapeLike(search) + "%"
		args = append(args, like, like, like, like)
	}
	sourceFilter := q.Get("source")
	if sourceFilter != "" {
		query += " AND a.source = ?"
		args = append(args, sourceFilter)
	}
	// has_interviews=1: only applications that landed a non-cancelled
	// interview. The dashboard's "interviews" tile links here.
	hasInterviews := q.Get("has_interviews") == "1" || q.Get("has_interviews") == "true"
	if hasInterviews {
		query += " AND EXISTS (SELECT 1 FROM interviews i WHERE i.application_id = a.id AND COALESCE(i.outcome, '') != 'Cancelled')"
	}
	// has_reply=1: the company answered, whatever the answer. The dashboard's
	// "replies" tile links here.
	hasReply := q.Get("has_reply") == "1" || q.Get("has_reply") == "true"
	if hasReply {
		query += " AND a.status IN ('Screening', 'Interviewing', 'Offer', 'Accepted', 'Rejected')"
	}

	allowed := map[string]bool{
		"created_at": true, "updated_at": true, "company_name": true,
		"job_title": true, "status": true, "applied_at": true,
		"salary": true, "rating": true, "confidence": true,
	}
	if !allowed[sortBy] {
		sortBy = "created_at"
	}
	dir := "DESC"
	if sortDir == "asc" {
		dir = "ASC"
	}
	query += " ORDER BY a." + sortBy + " " + dir

	countQuery := "SELECT COUNT(*) FROM applications a WHERE 1=1"
	countArgs := []any{}
	if statusFilter != "" {
		countQuery += " AND a.status = ?"
		countArgs = append(countArgs, statusFilter)
	}
	if search != "" {
		countQuery += " AND (a.company_name LIKE ? ESCAPE '\\' OR a.job_title LIKE ? ESCAPE '\\' OR a.location LIKE ? ESCAPE '\\' OR a.source LIKE ? ESCAPE '\\')"
		like := "%" + escapeLike(search) + "%"
		countArgs = append(countArgs, like, like, like, like)
	}
	if sourceFilter != "" {
		countQuery += " AND a.source = ?"
		countArgs = append(countArgs, sourceFilter)
	}
	if hasInterviews {
		countQuery += " AND EXISTS (SELECT 1 FROM interviews i WHERE i.application_id = a.id AND COALESCE(i.outcome, '') != 'Cancelled')"
	}
	if hasReply {
		countQuery += " AND a.status IN ('Screening', 'Interviewing', 'Offer', 'Accepted', 'Rejected')"
	}
	var total int
	if err := h.db.QueryRowContext(r.Context(), countQuery, countArgs...).Scan(&total); err != nil {
		log.Printf("ListApplications count: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load applications")
		return
	}

	limit, offset := paginationParams(r)
	query += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := h.db.QueryContext(r.Context(), query, args...)
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

func (h *Handler) GetApplication(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var a models.Application
	row := h.db.QueryRowContext(r.Context(), `SELECT id, company_name, company_website, company_industry,
		company_size, company_location, job_title, job_url, job_description, contract_type, contract_duration, work_mode,
		location, salary, salary_currency, status, applied_at, source,
		notes, speech, rating, confidence, created_at, updated_at, follow_up_snoozed_until FROM applications WHERE id = ?`, id)
	if err := scanApplication(row, &a); err == sql.ErrNoRows {
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
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
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

	a.ID = uuid.New().String()
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	var appliedAtStr *string
	if a.AppliedAt != nil {
		s := sqliteTime(*a.AppliedAt)
		appliedAtStr = &s
	}

	_, err := h.db.ExecContext(r.Context(), `INSERT INTO applications
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
		log.Printf("CreateApplication: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create application")
		return
	}

	h.addTimelineEvent(r, a.ID, "created", "Application created")
	writeJSON(w, http.StatusCreated, a)
}

func (h *Handler) UpdateApplication(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var oldStatus models.ApplicationStatus
	err := h.db.QueryRowContext(r.Context(), `SELECT status FROM applications WHERE id = ?`, id).Scan(&oldStatus)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "application not found")
		return
	} else if err != nil {
		log.Printf("UpdateApplication lookup: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load application")
		return
	}

	var a models.Application
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateApplication(&a); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	a.UpdatedAt = now

	if a.Status == models.StatusApplied && oldStatus != models.StatusApplied && a.AppliedAt == nil {
		a.AppliedAt = &now
	}

	var appliedAtStr *string
	if a.AppliedAt != nil {
		s := sqliteTime(*a.AppliedAt)
		appliedAtStr = &s
	}

	_, err = h.db.ExecContext(r.Context(), `UPDATE applications SET
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
		log.Printf("UpdateApplication: %v", err)
		writeError(w, http.StatusInternalServerError, "could not update application")
		return
	}

	if a.Status != oldStatus {
		h.addTimelineEvent(r, id, "status_change", fmt.Sprintf("Status changed from %s to %s", oldStatus, a.Status))
	}

	a.ID = id
	writeJSON(w, http.StatusOK, a)
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

func (h *Handler) BulkUpdateStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs    []string                 `json:"ids"`
		Status models.ApplicationStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
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

	now := sqliteTime(time.Now().UTC())
	var updated int
	for _, id := range body.IDs {
		var oldStatus models.ApplicationStatus
		if err := h.db.QueryRowContext(r.Context(), `SELECT status FROM applications WHERE id = ?`, id).Scan(&oldStatus); err != nil {
			continue
		}
		if oldStatus == body.Status {
			continue
		}
		result, err := h.db.ExecContext(r.Context(), `UPDATE applications SET status = ?, updated_at = ? WHERE id = ?`, body.Status, now, id)
		if err != nil {
			continue
		}
		if n, _ := result.RowsAffected(); n > 0 {
			h.addTimelineEvent(r, id, "status_change", fmt.Sprintf("Status changed from %s to %s", oldStatus, body.Status))
			updated++
		}
	}

	writeJSON(w, http.StatusOK, map[string]int{"updated": updated})
}

func (h *Handler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no application IDs provided")
		return
	}

	var deleted int
	for _, id := range body.IDs {
		result, err := h.db.ExecContext(r.Context(), `DELETE FROM applications WHERE id = ?`, id)
		if err != nil {
			continue
		}
		if n, _ := result.RowsAffected(); n > 0 {
			deleted++
		}
	}

	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted})
}

// Interviews

func (h *Handler) ListInterviews(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	interviews, err := h.getInterviewsByApplication(r, id)
	if err != nil {
		log.Printf("ListInterviews: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load interviews")
		return
	}
	writeJSON(w, http.StatusOK, interviews)
}

func (h *Handler) GetInterview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var iv models.Interview
	row := h.db.QueryRowContext(r.Context(), `SELECT id, application_id, round, type, scheduled_at,
		duration_minutes, interviewer_name, interviewer_role, notes, prep_notes, outcome, created_at
		FROM interviews WHERE id = ?`, id)
	err := row.Scan(&iv.ID, &iv.ApplicationID, &iv.Round, &iv.Type, &iv.ScheduledAt,
		&iv.DurationMinutes, &iv.InterviewerName, &iv.InterviewerRole, &iv.Notes,
		&iv.PrepNotes, &iv.Outcome, &iv.CreatedAt)
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
	if err := json.NewDecoder(r.Body).Decode(&iv); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
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

	_, err := h.db.ExecContext(r.Context(), `INSERT INTO interviews
		(id, application_id, round, type, scheduled_at, duration_minutes, interviewer_name,
		interviewer_role, notes, prep_notes, outcome, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		iv.ID, iv.ApplicationID, iv.Round, iv.Type, scheduledStr, iv.DurationMinutes,
		iv.InterviewerName, iv.InterviewerRole, iv.Notes, iv.PrepNotes, iv.Outcome, sqliteTime(iv.CreatedAt),
	)
	if err != nil {
		log.Printf("CreateInterview: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create interview")
		return
	}
	h.addTimelineEvent(r, appID, "interview_added", fmt.Sprintf("Interview round %d (%s) added", iv.Round, iv.Type))
	writeJSON(w, http.StatusCreated, iv)
}

func (h *Handler) UpdateInterview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var iv models.Interview
	if err := json.NewDecoder(r.Body).Decode(&iv); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
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
	iv.ID = id
	writeJSON(w, http.StatusOK, iv)
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
	contacts, err := h.getContactsByApplication(r, id)
	if err != nil {
		log.Printf("ListContacts: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load contacts")
		return
	}
	writeJSON(w, http.StatusOK, contacts)
}

func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var c models.Contact
	row := h.db.QueryRowContext(r.Context(), `SELECT id, application_id, name, role, email, phone, linkedin, notes, created_at
		FROM contacts WHERE id = ?`, id)
	err := row.Scan(&c.ID, &c.ApplicationID, &c.Name, &c.Role, &c.Email, &c.Phone, &c.LinkedIn, &c.Notes, &c.CreatedAt)
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
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateContact(&c); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c.ID = uuid.New().String()
	c.ApplicationID = appID
	c.CreatedAt = time.Now().UTC()

	_, err := h.db.ExecContext(r.Context(), `INSERT INTO contacts
		(id, application_id, name, role, email, phone, linkedin, notes, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		c.ID, c.ApplicationID, c.Name, c.Role, c.Email, c.Phone, c.LinkedIn, c.Notes, sqliteTime(c.CreatedAt),
	)
	if err != nil {
		log.Printf("CreateContact: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create contact")
		return
	}
	h.addTimelineEvent(r, appID, "contact_added", fmt.Sprintf("Contact %s added", c.Name))
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var c models.Contact
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
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
	c.ID = id
	writeJSON(w, http.StatusOK, c)
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

	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM applications`).Scan(&stats.Total); err != nil {
		log.Printf("GetStats total: %v", err)
	}

	if rows, err := h.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM applications GROUP BY status`); err != nil {
		log.Printf("GetStats byStatus: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err != nil {
				log.Printf("GetStats byStatus scan: %v", err)
				continue
			}
			stats.ByStatus[status] = count
		}
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

	// Activity heatmap: daily count of events over the last 180 days:
	// applications created, timeline events, and interviews actually held.
	if heatRows, err := h.db.QueryContext(ctx, `SELECT day, SUM(c) as total FROM (
		SELECT date(replace(replace(created_at, 'T', ' '), 'Z', '')) as day, COUNT(*) as c
		FROM applications
		WHERE created_at >= datetime('now', '-180 days')
		GROUP BY day
		UNION ALL
		SELECT date(replace(replace(created_at, 'T', ' '), 'Z', '')) as day, COUNT(*) as c
		FROM timeline_events
		WHERE created_at >= datetime('now', '-180 days')
		GROUP BY day
		UNION ALL
		SELECT date(replace(replace(scheduled_at, 'T', ' '), 'Z', '')) as day, COUNT(*) as c
		FROM interviews
		WHERE scheduled_at IS NOT NULL
		  AND COALESCE(outcome, '') != 'Cancelled'
		  AND scheduled_at >= datetime('now', '-180 days')
		  AND scheduled_at <= datetime('now')
		GROUP BY day
	) GROUP BY day ORDER BY day`); err != nil {
		log.Printf("GetStats heatmap: %v", err)
	} else {
		defer heatRows.Close()
		for heatRows.Next() {
			var d models.ActivityDay
			if err := heatRows.Scan(&d.Date, &d.Count); err != nil {
				log.Printf("GetStats heatmap scan: %v", err)
				continue
			}
			stats.ActivityHeatmap = append(stats.ActivityHeatmap, d)
		}
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

	nowStr := sqliteTime(time.Now().UTC())
	tenDaysAgo := sqliteTime(time.Now().UTC().AddDate(0, 0, -10))
	if fuRows, err := h.db.QueryContext(r.Context(), `SELECT a.id, a.company_name, a.job_title, a.status, MAX(i.scheduled_at) as last_iv
		FROM applications a JOIN interviews i ON i.application_id = a.id
		WHERE a.status IN ('Screening', 'Interviewing')
		  AND (a.follow_up_snoozed_until IS NULL OR a.follow_up_snoozed_until < ?)
		  AND i.scheduled_at IS NOT NULL
		GROUP BY a.id
		HAVING MAX(replace(i.scheduled_at, 'T', ' ')) < ?`, nowStr, tenDaysAgo); err == nil {
		defer fuRows.Close()
		for fuRows.Next() {
			var item models.FollowUpItem
			if err := fuRows.Scan(&item.ID, &item.CompanyName, &item.JobTitle, &item.Status, &item.LastInterviewAt); err == nil {
				stats.FollowUps = append(stats.FollowUps, item)
			}
		}
	}

	now := time.Now().UTC()
	sentApps := h.loadSentApps(ctx)
	replyEvents := h.loadReplyEvents(ctx)
	interviewTimes := h.loadInterviewTimes(ctx)
	stats.Period = computePeriodStats(now, parsePeriod(r.URL.Query().Get("period")), sentApps, interviewTimes)
	stats.Weekly = computeWeekly(now, sentApps, replyEvents)
	stats.UpcomingInterviews = countUpcomingInterviews(now, interviewTimes)
	stats.ActiveProcesses = h.loadActiveProcesses(ctx, now)

	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) SnoozeFollowUp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Until *string `json:"until"`
		Skip  bool    `json:"skip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
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
	companyName := strings.TrimSpace(r.URL.Query().Get("company_name"))
	if companyName == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, company_name, job_title, status, created_at FROM applications WHERE LOWER(company_name) = LOWER(?)`,
		companyName)
	if err != nil {
		log.Printf("CheckDuplicates: %v", err)
		writeJSON(w, http.StatusOK, []any{})
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
	var results []dupResult
	for rows.Next() {
		var d dupResult
		if err := rows.Scan(&d.ID, &d.CompanyName, &d.JobTitle, &d.Status, &d.CreatedAt); err == nil {
			results = append(results, d)
		}
	}
	if results == nil {
		results = []dupResult{}
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handler) ListSources(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT DISTINCT source FROM applications WHERE source IS NOT NULL AND source != '' ORDER BY source`)
	if err != nil {
		writeJSON(w, http.StatusOK, []string{})
		return
	}
	defer rows.Close()

	sources := make([]string, 0)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil {
			sources = append(sources, s)
		}
	}
	writeJSON(w, http.StatusOK, sources)
}

func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, company_name, company_website, company_industry,
		company_size, company_location, job_title, job_url, job_description, contract_type, contract_duration, work_mode,
		location, salary, salary_currency, status, applied_at, source,
		notes, speech, rating, confidence, created_at, updated_at, follow_up_snoozed_until FROM applications ORDER BY created_at`)
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

	for i := range apps {
		if iv, err := h.getInterviewsByApplication(r, apps[i].ID); err != nil {
			log.Printf("Export interviews for %s: %v", apps[i].ID, err)
		} else {
			apps[i].Interviews = iv
		}
		if ct, err := h.getContactsByApplication(r, apps[i].ID); err != nil {
			log.Printf("Export contacts for %s: %v", apps[i].ID, err)
		} else {
			apps[i].Contacts = ct
		}
		if ev, err := h.getTimelineByApplication(r, apps[i].ID); err != nil {
			log.Printf("Export timeline for %s: %v", apps[i].ID, err)
		} else {
			apps[i].TimelineEvents = ev
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"applications": apps, "exported_at": time.Now().UTC()})
}

func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB limit

	var payload struct {
		Applications []models.Application `json:"applications"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(payload.Applications) == 0 {
		writeError(w, http.StatusBadRequest, "no applications to import")
		return
	}

	var imported, skipped int
	now := time.Now().UTC()

	for _, a := range payload.Applications {
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
		if err := validateApplication(&a); err != nil {
			log.Printf("Import: skipping %q: %v", a.CompanyName, err)
			skipped++
			continue
		}

		if a.ID != "" {
			var exists int
			if err := h.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM applications WHERE id = ?", a.ID).Scan(&exists); err != nil {
				log.Printf("Import duplicate check: %v", err)
			}
			if exists > 0 {
				skipped++
				continue
			}
		} else {
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
		_, err := h.db.ExecContext(r.Context(), `INSERT INTO applications
			(id, company_name, company_website, company_industry, company_size, company_location,
			job_title, job_url, job_description, contract_type, contract_duration, work_mode, location,
			salary, salary_currency, status, applied_at, source, notes, speech, rating, confidence,
			created_at, updated_at, follow_up_snoozed_until) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.CompanyName, a.CompanyWebsite, a.CompanyIndustry, a.CompanySize, a.CompanyLocation,
			a.JobTitle, a.JobURL, a.JobDescription, a.ContractType, a.ContractDuration, a.WorkMode, a.Location,
			a.Salary, a.SalaryCurrency, a.Status, appliedAtStr, a.Source, a.Notes, a.Speech, a.Rating, a.Confidence,
			sqliteTime(a.CreatedAt), sqliteTime(a.UpdatedAt), snoozedStr,
		)
		if err != nil {
			skipped++
			continue
		}

		for _, iv := range a.Interviews {
			if iv.ID == "" {
				iv.ID = uuid.New().String()
			}
			iv.ApplicationID = a.ID
			if iv.CreatedAt.IsZero() {
				iv.CreatedAt = now
			}
			if err := validateInterview(&iv); err != nil {
				log.Printf("Import: skipping interview %s of %q: %v", iv.ID, a.CompanyName, err)
				continue
			}
			var scheduledStr *string
			if iv.ScheduledAt != nil {
				s := sqliteTime(*iv.ScheduledAt)
				scheduledStr = &s
			}
			if _, err := h.db.ExecContext(r.Context(), `INSERT INTO interviews
				(id, application_id, round, type, scheduled_at, duration_minutes, interviewer_name,
				interviewer_role, notes, prep_notes, outcome, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				iv.ID, iv.ApplicationID, iv.Round, iv.Type, scheduledStr, iv.DurationMinutes,
				iv.InterviewerName, iv.InterviewerRole, iv.Notes, iv.PrepNotes, iv.Outcome, sqliteTime(iv.CreatedAt),
			); err != nil {
				log.Printf("Import interview: %v", err)
			}
		}

		for _, c := range a.Contacts {
			if c.ID == "" {
				c.ID = uuid.New().String()
			}
			c.ApplicationID = a.ID
			if c.CreatedAt.IsZero() {
				c.CreatedAt = now
			}
			if validateContact(&c) != nil {
				continue
			}
			if _, err := h.db.ExecContext(r.Context(), `INSERT INTO contacts
				(id, application_id, name, role, email, phone, linkedin, notes, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
				c.ID, c.ApplicationID, c.Name, c.Role, c.Email, c.Phone, c.LinkedIn, c.Notes, sqliteTime(c.CreatedAt),
			); err != nil {
				log.Printf("Import contact: %v", err)
			}
		}

		for _, e := range a.TimelineEvents {
			if e.ID == "" {
				e.ID = uuid.New().String()
			}
			e.ApplicationID = a.ID
			if e.CreatedAt.IsZero() {
				e.CreatedAt = now
			}
			if _, err := h.db.ExecContext(r.Context(), `INSERT INTO timeline_events
				(id, application_id, event_type, description, created_at) VALUES (?,?,?,?,?)`,
				e.ID, e.ApplicationID, e.EventType, e.Description, sqliteTime(e.CreatedAt),
			); err != nil {
				log.Printf("Import timeline event: %v", err)
			}
		}

		imported++
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

func (h *Handler) getInterviewsByApplication(r *http.Request, appID string) ([]models.Interview, error) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, application_id, round, type, scheduled_at,
		duration_minutes, interviewer_name, interviewer_role, notes, prep_notes, outcome, created_at
		FROM interviews WHERE application_id=? ORDER BY round, created_at`, appID)
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

func (h *Handler) getContactsByApplication(r *http.Request, appID string) ([]models.Contact, error) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, application_id, name, role, email, phone, linkedin, notes, created_at
		FROM contacts WHERE application_id=? ORDER BY created_at`, appID)
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

func (h *Handler) getTimelineByApplication(r *http.Request, appID string) ([]models.TimelineEvent, error) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, application_id, event_type, description, created_at
		FROM timeline_events WHERE application_id=? ORDER BY created_at`, appID)
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

func (h *Handler) addTimelineEvent(r *http.Request, appID, eventType, desc string) {
	if _, err := h.db.ExecContext(r.Context(), `INSERT INTO timeline_events (id, application_id, event_type, description, created_at) VALUES (?,?,?,?,?)`,
		uuid.New().String(), appID, eventType, desc, sqliteTime(time.Now().UTC())); err != nil {
		log.Printf("addTimelineEvent: %v", err)
	}
}

func (h *Handler) ExtractFromURL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	result, err := extract.FromURL(body.URL)
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

func sqliteTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}
