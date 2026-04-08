package handlers

import (
	"database/sql"
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
	models.StatusRejected: true, models.StatusWithdrawn: true,
}

var validInterviewTypes = map[models.InterviewType]bool{
	models.InterviewPhone: true, models.InterviewVideo: true,
	models.InterviewOnsite: true, models.InterviewTechnical: true,
	models.InterviewHR: true, models.InterviewCulture: true,
	models.InterviewFinal: true,
}

var validInterviewOutcomes = map[models.InterviewOutcome]bool{
	models.OutcomePassed: true, models.OutcomeFailed: true,
	models.OutcomePending: true, models.OutcomeCancelled: true,
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
		a.notes, a.speech, a.rating, a.confidence, a.created_at, a.updated_at,
		(SELECT COUNT(*) FROM interviews WHERE application_id = a.id) as interview_count,
		(SELECT COUNT(*) FROM contacts WHERE application_id = a.id) as contact_count
		FROM applications a WHERE 1=1`
	args := []any{}

	if statusFilter != "" {
		query += " AND a.status = ?"
		args = append(args, statusFilter)
	}
	if search != "" {
		query += " AND (a.company_name LIKE ? OR a.job_title LIKE ? OR a.location LIKE ? OR a.source LIKE ?)"
		like := "%" + search + "%"
		args = append(args, like, like, like, like)
	}
	sourceFilter := q.Get("source")
	if sourceFilter != "" {
		query += " AND a.source = ?"
		args = append(args, sourceFilter)
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
		countQuery += " AND (a.company_name LIKE ? OR a.job_title LIKE ? OR a.location LIKE ? OR a.source LIKE ?)"
		like := "%" + search + "%"
		countArgs = append(countArgs, like, like, like, like)
	}
	if sourceFilter != "" {
		countQuery += " AND a.source = ?"
		countArgs = append(countArgs, sourceFilter)
	}
	var total int
	h.db.QueryRowContext(r.Context(), countQuery, countArgs...).Scan(&total)

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
			&a.CreatedAt, &a.UpdatedAt,
			&a.InterviewCount, &a.ContactCount,
		); err != nil {
			log.Printf("ListApplications scan: %v", err)
			writeError(w, http.StatusInternalServerError, "could not load applications")
			return
		}
		apps = append(apps, a)
	}

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
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
		notes, speech, rating, confidence, created_at, updated_at FROM applications WHERE id = ?`, id)
	if err := scanApplication(row, &a); err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "application not found")
		return
	} else if err != nil {
		log.Printf("GetApplication: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load application")
		return
	}

	interviews, _ := h.getInterviewsByApplication(r, id)
	a.Interviews = interviews

	contacts, _ := h.getContactsByApplication(r, id)
	a.Contacts = contacts

	events, _ := h.getTimelineByApplication(r, id)
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
		created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.CompanyName, a.CompanyWebsite, a.CompanyIndustry, a.CompanySize, a.CompanyLocation,
		a.JobTitle, a.JobURL, a.JobDescription, a.ContractType, a.ContractDuration, a.WorkMode, a.Location,
		a.Salary, a.SalaryCurrency, a.Status, appliedAtStr, a.Source, a.Notes, a.Speech, a.Rating, a.Confidence,
		sqliteTime(a.CreatedAt), sqliteTime(a.UpdatedAt),
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
	n, _ := result.RowsAffected()
	if n == 0 {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	n, _ := result.RowsAffected()
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
	h.db.QueryRowContext(r.Context(), `SELECT application_id FROM interviews WHERE id=?`, id).Scan(&appID)
	result, err := h.db.ExecContext(r.Context(), `DELETE FROM interviews WHERE id=?`, id)
	if err != nil {
		log.Printf("DeleteInterview: %v", err)
		writeError(w, http.StatusInternalServerError, "could not delete interview")
		return
	}
	n, _ := result.RowsAffected()
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
	n, _ := result.RowsAffected()
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
	h.db.QueryRowContext(r.Context(), `SELECT application_id FROM contacts WHERE id=?`, id).Scan(&appID)
	result, err := h.db.ExecContext(r.Context(), `DELETE FROM contacts WHERE id=?`, id)
	if err != nil {
		log.Printf("DeleteContact: %v", err)
		writeError(w, http.StatusInternalServerError, "could not delete contact")
		return
	}
	n, _ := result.RowsAffected()
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
	stats := models.Stats{
		ByStatus:        map[string]int{},
		AvgDaysInStatus: map[string]float64{},
	}

	h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM applications`).Scan(&stats.Total)

	// Count by status
	rows, _ := h.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM applications GROUP BY status`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			rows.Scan(&status, &count)
			stats.ByStatus[status] = count
		}
	}

	responded := stats.ByStatus[string(models.StatusScreening)] +
		stats.ByStatus[string(models.StatusInterviewing)] +
		stats.ByStatus[string(models.StatusOffer)] +
		stats.ByStatus[string(models.StatusAccepted)] +
		stats.ByStatus[string(models.StatusRejected)]
	applied := stats.ByStatus[string(models.StatusApplied)] + responded
	if applied > 0 {
		stats.ResponseRate = float64(responded) / float64(applied) * 100
	}

	offers := stats.ByStatus[string(models.StatusOffer)] + stats.ByStatus[string(models.StatusAccepted)]
	if applied > 0 {
		stats.OfferRate = float64(offers) / float64(applied) * 100
	}

	h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM applications WHERE status IN ('Screening', 'Interviewing')`).Scan(&stats.ActiveInterviews)

	h.db.QueryRowContext(ctx, `SELECT AVG(salary) FROM applications WHERE salary IS NOT NULL`).Scan(&stats.AvgSalary)

	salaryRows, _ := h.db.QueryContext(ctx, `SELECT
		CASE
			WHEN salary < 30000 THEN '< 30k'
			WHEN salary < 40000 THEN '30-40k'
			WHEN salary < 50000 THEN '40-50k'
			WHEN salary < 60000 THEN '50-60k'
			WHEN salary < 70000 THEN '60-70k'
			WHEN salary < 80000 THEN '70-80k'
			WHEN salary < 90000 THEN '80-90k'
			WHEN salary < 100000 THEN '90-100k'
			ELSE '100k+'
		END as range_bucket,
		COUNT(*) as c
		FROM applications WHERE salary IS NOT NULL
		GROUP BY range_bucket ORDER BY salary`)
	if salaryRows != nil {
		defer salaryRows.Close()
		for salaryRows.Next() {
			var b models.SalaryBucket
			salaryRows.Scan(&b.Range, &b.Count)
			stats.SalaryDistribution = append(stats.SalaryDistribution, b)
		}
	}

	sourceRows, _ := h.db.QueryContext(ctx, `SELECT source, COUNT(*) as c FROM applications WHERE source IS NOT NULL AND source != '' GROUP BY source ORDER BY c DESC LIMIT 5`)
	if sourceRows != nil {
		defer sourceRows.Close()
		for sourceRows.Next() {
			var sc models.SourceCount
			sourceRows.Scan(&sc.Source, &sc.Count)
			stats.TopSources = append(stats.TopSources, sc)
		}
	}

	// strftime needs plain datetime; strip RFC3339 T and Z from stored values.
	timeRows, _ := h.db.QueryContext(ctx, `SELECT
		strftime('%Y-W%W', replace(replace(created_at, 'T', ' '), 'Z', '')) as period,
		COUNT(*) as c
		FROM applications
		WHERE created_at >= datetime('now', '-84 days')
		GROUP BY period ORDER BY period`)
	if timeRows != nil {
		defer timeRows.Close()
		for timeRows.Next() {
			var p models.TimeSeriesPoint
			timeRows.Scan(&p.Period, &p.Count)
			stats.OverTime = append(stats.OverTime, p)
		}
	}

	daysRows, _ := h.db.QueryContext(ctx, `SELECT
		te1.description,
		AVG(julianday(replace(replace(te2.created_at, 'T', ' '), 'Z', ''))
		  - julianday(replace(replace(te1.created_at, 'T', ' '), 'Z', '')))
		FROM timeline_events te1
		INNER JOIN timeline_events te2 ON te1.application_id = te2.application_id
			AND te2.event_type = 'status_change'
			AND te2.created_at > te1.created_at
		WHERE te1.event_type = 'status_change'
		GROUP BY te1.description`)
	if daysRows != nil {
		defer daysRows.Close()
		for daysRows.Next() {
			var desc string
			var avgDays float64
			daysRows.Scan(&desc, &avgDays)
			// Extract the target status from "Status changed from X to Y"
			if idx := strings.LastIndex(desc, " to "); idx >= 0 {
				status := desc[idx+4:]
				stats.AvgDaysInStatus[status] = avgDays
			}
		}
	}

	writeJSON(w, http.StatusOK, stats)
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
		notes, speech, rating, confidence, created_at, updated_at FROM applications ORDER BY created_at`)
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

	for i := range apps {
		apps[i].Interviews, _ = h.getInterviewsByApplication(r, apps[i].ID)
		apps[i].Contacts, _ = h.getContactsByApplication(r, apps[i].ID)
		apps[i].TimelineEvents, _ = h.getTimelineByApplication(r, apps[i].ID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"applications": apps, "exported_at": time.Now().UTC()})
}

func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20) // 50MB limit

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
		// Defaults
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
			skipped++
			continue
		}

		// Duplicate check by ID
		if a.ID != "" {
			var exists int
			h.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM applications WHERE id = ?", a.ID).Scan(&exists)
			if exists > 0 {
				skipped++
				continue
			}
		} else {
			a.ID = uuid.New().String()
		}

		// Preserve or set timestamps
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

		_, err := h.db.ExecContext(r.Context(), `INSERT INTO applications
			(id, company_name, company_website, company_industry, company_size, company_location,
			job_title, job_url, job_description, contract_type, contract_duration, work_mode, location,
			salary, salary_currency, status, applied_at, source, notes, speech, rating, confidence,
			created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			a.ID, a.CompanyName, a.CompanyWebsite, a.CompanyIndustry, a.CompanySize, a.CompanyLocation,
			a.JobTitle, a.JobURL, a.JobDescription, a.ContractType, a.ContractDuration, a.WorkMode, a.Location,
			a.Salary, a.SalaryCurrency, a.Status, appliedAtStr, a.Source, a.Notes, a.Speech, a.Rating, a.Confidence,
			sqliteTime(a.CreatedAt), sqliteTime(a.UpdatedAt),
		)
		if err != nil {
			skipped++
			continue
		}

		// Import interviews
		for _, iv := range a.Interviews {
			if iv.ID == "" {
				iv.ID = uuid.New().String()
			}
			iv.ApplicationID = a.ID
			if iv.CreatedAt.IsZero() {
				iv.CreatedAt = now
			}
			if validateInterview(&iv) != nil {
				continue
			}
			var scheduledStr *string
			if iv.ScheduledAt != nil {
				s := sqliteTime(*iv.ScheduledAt)
				scheduledStr = &s
			}
			h.db.ExecContext(r.Context(), `INSERT INTO interviews
				(id, application_id, round, type, scheduled_at, duration_minutes, interviewer_name,
				interviewer_role, notes, prep_notes, outcome, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
				iv.ID, iv.ApplicationID, iv.Round, iv.Type, scheduledStr, iv.DurationMinutes,
				iv.InterviewerName, iv.InterviewerRole, iv.Notes, iv.PrepNotes, iv.Outcome, sqliteTime(iv.CreatedAt),
			)
		}

		// Import contacts
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
			h.db.ExecContext(r.Context(), `INSERT INTO contacts
				(id, application_id, name, role, email, phone, linkedin, notes, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
				c.ID, c.ApplicationID, c.Name, c.Role, c.Email, c.Phone, c.LinkedIn, c.Notes, sqliteTime(c.CreatedAt),
			)
		}

		// Import timeline events
		for _, e := range a.TimelineEvents {
			if e.ID == "" {
				e.ID = uuid.New().String()
			}
			e.ApplicationID = a.ID
			if e.CreatedAt.IsZero() {
				e.CreatedAt = now
			}
			h.db.ExecContext(r.Context(), `INSERT INTO timeline_events
				(id, application_id, event_type, description, created_at) VALUES (?,?,?,?,?)`,
				e.ID, e.ApplicationID, e.EventType, e.Description, sqliteTime(e.CreatedAt),
			)
		}

		imported++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"total":    len(payload.Applications),
	})
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
		&a.CreatedAt, &a.UpdatedAt,
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
		rows.Scan(&iv.ID, &iv.ApplicationID, &iv.Round, &iv.Type, &iv.ScheduledAt,
			&iv.DurationMinutes, &iv.InterviewerName, &iv.InterviewerRole, &iv.Notes,
			&iv.PrepNotes, &iv.Outcome, &iv.CreatedAt)
		list = append(list, iv)
	}
	return list, nil
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
		rows.Scan(&c.ID, &c.ApplicationID, &c.Name, &c.Role, &c.Email, &c.Phone, &c.LinkedIn, &c.Notes, &c.CreatedAt)
		list = append(list, c)
	}
	return list, nil
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
		rows.Scan(&e.ID, &e.ApplicationID, &e.EventType, &e.Description, &e.CreatedAt)
		list = append(list, e)
	}
	return list, nil
}

func (h *Handler) addTimelineEvent(r *http.Request, appID, eventType, desc string) {
	h.db.ExecContext(r.Context(), `INSERT INTO timeline_events (id, application_id, event_type, description, created_at) VALUES (?,?,?,?,?)`,
		uuid.New().String(), appID, eventType, desc, sqliteTime(time.Now().UTC()))
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

func sqliteTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

