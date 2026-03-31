package extract

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Result contains the fields we could confidently extract from a job URL.
// nil means "not found / not confident enough to fill".
type Result struct {
	CompanyName     *string `json:"company_name,omitempty"`
	CompanyWebsite  *string `json:"company_website,omitempty"`
	CompanyLocation *string `json:"company_location,omitempty"`
	JobTitle        *string `json:"job_title,omitempty"`
	JobDescription  *string `json:"job_description,omitempty"`
	Location        *string `json:"location,omitempty"`
	ContractType    *string `json:"contract_type,omitempty"`
	WorkMode        *string `json:"work_mode,omitempty"`
	Salary          *int    `json:"salary,omitempty"`
	SalaryCurrency  *string `json:"salary_currency,omitempty"`
	Source          *string `json:"source,omitempty"`
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// FromURL fetches the given URL and extracts job posting data.
func FromURL(rawURL string) (*Result, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid URL")
	}

	result := &Result{}

	// Source = domain name
	source := cleanDomain(parsed.Host)
	if source != "" {
		result.Source = &source
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return result, nil // return what we have (source)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,fr;q=0.8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return result, nil // return what we have (source)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Still return partial result (source) — don't fail entirely
		return result, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB max
	if err != nil {
		return result, nil
	}

	page := string(body)

	// Try JSON-LD first (most reliable)
	if ld := extractJobPostingLD(page); ld != nil {
		applyJobPosting(result, ld)
	}

	// Fill gaps from Open Graph / meta tags
	applyMetaTags(result, page)

	return result, nil
}

// --- JSON-LD extraction ---

type jobPostingLD struct {
	Type               string      `json:"@type"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	EmploymentType     interface{} `json:"employmentType"`     // string or []string
	JobLocationType    string      `json:"jobLocationType"`    // "TELECOMMUTE"
	HiringOrganization interface{} `json:"hiringOrganization"` // object
	JobLocation        interface{} `json:"jobLocation"`        // object or []object
	BaseSalary         interface{} `json:"baseSalary"`         // object
	Industry           string      `json:"industry"`
}

type orgLD struct {
	Name   string `json:"name"`
	SameAs string `json:"sameAs"`
	URL    string `json:"url"`
}

type locationLD struct {
	Type    string    `json:"@type"`
	Address interface{} `json:"address"` // string or object
}

type addressLD struct {
	Locality string `json:"addressLocality"`
	Region   string `json:"addressRegion"`
	Country  string `json:"addressCountry"`
}

type salaryLD struct {
	Currency string      `json:"currency"`
	Value    interface{} `json:"value"` // object with minValue/maxValue or a number
}

type salaryValueLD struct {
	MinValue float64 `json:"minValue"`
	MaxValue float64 `json:"maxValue"`
	UnitText string  `json:"unitText"` // "YEAR", "MONTH", "HOUR"
}

var jsonLDRegex = regexp.MustCompile(`<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>([\s\S]*?)</script>`)

func extractJobPostingLD(page string) *jobPostingLD {
	matches := jsonLDRegex.FindAllStringSubmatch(page, -1)
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		if raw == "" {
			continue
		}

		// Try as single object
		var single jobPostingLD
		if err := json.Unmarshal([]byte(raw), &single); err == nil {
			if single.Type == "JobPosting" {
				return &single
			}
		}

		// Try as array (some sites wrap in an array)
		var arr []jobPostingLD
		if err := json.Unmarshal([]byte(raw), &arr); err == nil {
			for i := range arr {
				if arr[i].Type == "JobPosting" {
					return &arr[i]
				}
			}
		}

		// Try as @graph wrapper
		var graph struct {
			Graph []jobPostingLD `json:"@graph"`
		}
		if err := json.Unmarshal([]byte(raw), &graph); err == nil {
			for i := range graph.Graph {
				if graph.Graph[i].Type == "JobPosting" {
					return &graph.Graph[i]
				}
			}
		}
	}
	return nil
}

func applyJobPosting(r *Result, ld *jobPostingLD) {
	if s := clean(ld.Title); s != "" {
		r.JobTitle = &s
	}

	if s := cleanHTML(ld.Description); s != "" {
		r.JobDescription = &s
	}

	// Hiring organization
	if ld.HiringOrganization != nil {
		var org orgLD
		if b, err := json.Marshal(ld.HiringOrganization); err == nil {
			if json.Unmarshal(b, &org) == nil {
				if s := clean(org.Name); s != "" {
					r.CompanyName = &s
				}
				site := org.SameAs
				if site == "" {
					site = org.URL
				}
				if s := clean(site); s != "" && isCompanyWebsite(s) {
					r.CompanyWebsite = &s
				}
			}
		}
	}

	// Job location
	if ld.JobLocation != nil {
		if loc := extractLocation(ld.JobLocation); loc != "" {
			r.Location = &loc
		}
	}

	// Employment type -> contract type
	if ct := mapEmploymentType(ld.EmploymentType); ct != "" {
		r.ContractType = &ct
	}

	// Remote detection
	if strings.EqualFold(ld.JobLocationType, "TELECOMMUTE") {
		wm := "Remote"
		r.WorkMode = &wm
	}

	// Salary (only if yearly and sensible)
	if ld.BaseSalary != nil {
		applySalary(r, ld.BaseSalary)
	}
}

func extractLocation(raw interface{}) string {
	// Try as single location object
	b, err := json.Marshal(raw)
	if err != nil {
		return ""
	}

	var loc locationLD
	if json.Unmarshal(b, &loc) == nil && loc.Address != nil {
		return parseAddress(loc.Address)
	}

	// Try as array
	var locs []locationLD
	if json.Unmarshal(b, &locs) == nil && len(locs) > 0 {
		if locs[0].Address != nil {
			return parseAddress(locs[0].Address)
		}
	}

	return ""
}

func parseAddress(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return clean(v)
	default:
		b, err := json.Marshal(raw)
		if err != nil {
			return ""
		}
		var addr addressLD
		if json.Unmarshal(b, &addr) == nil {
			parts := []string{}
			if s := clean(addr.Locality); s != "" {
				parts = append(parts, s)
			}
			if s := clean(addr.Region); s != "" {
				parts = append(parts, s)
			}
			if s := clean(addr.Country); s != "" {
				parts = append(parts, s)
			}
			return strings.Join(parts, ", ")
		}
		return ""
	}
}

func mapEmploymentType(raw interface{}) string {
	var types []string
	switch v := raw.(type) {
	case string:
		types = []string{v}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				types = append(types, s)
			}
		}
	default:
		return ""
	}

	for _, t := range types {
		t = strings.ToUpper(strings.TrimSpace(t))
		switch t {
		case "FULL_TIME", "FULL-TIME":
			return "CDI"
		case "CONTRACT", "TEMPORARY", "TEMP":
			return "CDD"
		case "INTERN", "INTERNSHIP":
			return "Internship"
		case "FREELANCE", "CONTRACTOR":
			return "Freelance"
		case "PART_TIME", "PART-TIME":
			// Part-time exists but isn't a contract type in our model
			return ""
		}
	}
	return ""
}

func applySalary(r *Result, raw interface{}) {
	b, err := json.Marshal(raw)
	if err != nil {
		return
	}

	var sal salaryLD
	if json.Unmarshal(b, &sal) != nil {
		return
	}

	currency := strings.ToUpper(clean(sal.Currency))
	if currency == "" {
		return
	}

	vb, err := json.Marshal(sal.Value)
	if err != nil {
		return
	}

	var sv salaryValueLD
	if json.Unmarshal(vb, &sv) != nil {
		return
	}

	// Only accept yearly salaries (sensible range)
	unit := strings.ToUpper(sv.UnitText)
	min := sv.MinValue
	max := sv.MaxValue

	switch unit {
	case "YEAR", "YEARLY", "":
		// already yearly, or no unit (assume yearly for large values)
	case "MONTH", "MONTHLY":
		min *= 12
		max *= 12
	default:
		return // hourly, weekly, etc — too unreliable to convert
	}

	// Sanity check: salary between 10k and 1M
	// Pick the best single value: max if available, otherwise min
	val := max
	if val <= 0 || val < 10000 || val > 1000000 {
		val = min
	}
	if val > 0 && val >= 10000 && val <= 1000000 {
		salaryInt := int(val)
		r.Salary = &salaryInt
		r.SalaryCurrency = &currency
	}
}

// --- Meta tags fallback ---

var metaRegex = regexp.MustCompile(`<meta\s+([^>]+?)\/?>`)
var metaAttrRegex = regexp.MustCompile(`(property|name|content)\s*=\s*["']([^"']*?)["']`)
var titleRegex = regexp.MustCompile(`<title[^>]*>(.*?)</title>`)

func applyMetaTags(r *Result, page string) {
	meta := parseMetaTags(page)

	// Only fill what's still missing
	if r.JobTitle == nil {
		if s := clean(meta["og:title"]); s != "" {
			r.JobTitle = &s
		} else if s := extractTitle(page); s != "" {
			r.JobTitle = &s
		}
	}

	if r.CompanyName == nil {
		if s := clean(meta["og:site_name"]); s != "" {
			r.CompanyName = &s
		}
	}

	if r.JobDescription == nil {
		if s := clean(meta["og:description"]); s != "" {
			r.JobDescription = &s
		} else if s := clean(meta["description"]); s != "" {
			r.JobDescription = &s
		}
	}
}

func parseMetaTags(page string) map[string]string {
	result := map[string]string{}
	matches := metaRegex.FindAllStringSubmatch(page, -1)
	for _, m := range matches {
		attrs := map[string]string{}
		for _, a := range metaAttrRegex.FindAllStringSubmatch(m[1], -1) {
			attrs[a[1]] = a[2]
		}
		key := attrs["property"]
		if key == "" {
			key = attrs["name"]
		}
		if key != "" && attrs["content"] != "" {
			result[key] = attrs["content"]
		}
	}
	return result
}

func extractTitle(page string) string {
	m := titleRegex.FindStringSubmatch(page)
	if len(m) < 2 {
		return ""
	}
	return clean(m[1])
}

// --- Helpers ---

var htmlTagRegex = regexp.MustCompile(`<[^>]+>`)
var multiSpaceRegex = regexp.MustCompile(`\s+`)

func clean(s string) string {
	s = html.UnescapeString(s)
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

func cleanHTML(s string) string {
	s = html.UnescapeString(s)
	s = htmlTagRegex.ReplaceAllString(s, " ")
	s = multiSpaceRegex.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if len(s) > 5000 {
		s = s[:5000]
	}
	return s
}

// isCompanyWebsite returns true if the URL looks like an actual company website,
// not a profile page on a job board (e.g. indeed.com/cmp/... or linkedin.com/company/...).
func isCompanyWebsite(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Host)
	jobBoards := []string{"indeed", "linkedin", "glassdoor", "monster", "seek", "wttj", "welcometothejungle", "jobs.ie", "irishjobs"}
	for _, board := range jobBoards {
		if strings.Contains(host, board) {
			return false
		}
	}
	return true
}

func cleanDomain(host string) string {
	host = strings.ToLower(host)
	host = strings.TrimPrefix(host, "www.")
	// Match known job boards by any domain part (handles ie.indeed.com, fr.linkedin.com, etc.)
	known := map[string]string{
		"indeed":      "Indeed",
		"linkedin":    "LinkedIn",
		"glassdoor":   "Glassdoor",
		"monster":     "Monster",
		"welcometothejungle": "Welcome to the Jungle",
		"wttj":        "Welcome to the Jungle",
		"jobs":        host, // jobs.ie etc — keep full domain
	}
	for _, part := range strings.Split(host, ".") {
		if name, ok := known[part]; ok {
			return name
		}
	}
	return host
}
