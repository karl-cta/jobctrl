package extract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
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

// errBlocked is returned when the URL, a redirect or a DNS answer leads to a
// private or reserved address.
var errBlocked = errors.New("URL points to a private network")

// addrAllowed is the address policy applied to the resolved URL host and to
// every connection. Tests replace it to reach httptest servers on loopback.
var addrAllowed = func(ap netip.AddrPort) bool { return !isBlockedAddr(ap.Addr()) }

// dialControl runs once the address is resolved and before connecting, so it
// sees the address actually dialed: the first request, every redirect and a
// DNS answer that changed since the pre-check all go through it.
func dialControl(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil || !addrAllowed(netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())) {
		return errBlocked
	}
	return nil
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		// No proxy from the environment: the dial check would then see the
		// proxy's address instead of the target's.
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   dialControl,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: time.Second,
	},
	CheckRedirect: checkRedirect,
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("too many redirects")
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("unsupported redirect scheme")
	}
	return nil
}

// blockedPrefixes lists the private and reserved ranges extraction must never
// connect to. IPv4-mapped IPv6 addresses are unmapped before the check.
var blockedPrefixes = func() []netip.Prefix {
	var prefixes []netip.Prefix
	for _, s := range []string{
		// IPv4
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
		"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		// IPv6
		"::/128", "::1/128",
		"::/96",          // deprecated IPv4-compatible (::127.0.0.1)
		"64:ff9b:1::/48", // local-use NAT64
		"100::/64",       // discard-only
		"2001::/32",      // Teredo
		"2001:db8::/32",  // documentation
		"2002::/16",      // 6to4
		"fc00::/7", "fe80::/10",
		"fec0::/10", // deprecated site-local
		"ff00::/8",
	} {
		prefixes = append(prefixes, netip.MustParsePrefix(s))
	}
	return prefixes
}()

// nat64Prefix is the well-known NAT64 prefix. It carries an IPv4 address in its
// last 4 bytes and, on IPv6-only networks, every IPv4-only site resolves into
// it, so the embedded IPv4 address is what gets checked.
var nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")

// isBlockedAddr reports whether a is a private, loopback, link-local, multicast
// or otherwise reserved address.
func isBlockedAddr(a netip.Addr) bool {
	// Prefix.Contains never matches a zoned or IPv4-mapped address
	a = a.Unmap().WithZone("")
	if nat64Prefix.Contains(a) {
		b := a.As16()
		return isBlockedAddr(netip.AddrFrom4([4]byte(b[12:])))
	}
	if !a.IsValid() || !a.IsGlobalUnicast() || a.IsPrivate() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// FromURL fetches the given URL and extracts job posting data.
func FromURL(rawURL string) (*Result, error) {
	return FromURLContext(context.Background(), rawURL)
}

// FromURLContext is FromURL bound to ctx: resolving and fetching stop when ctx is done.
func FromURLContext(ctx context.Context, rawURL string) (*Result, error) {
	if len(rawURL) > 2048 {
		return nil, fmt.Errorf("URL too long")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, fmt.Errorf("invalid URL")
	}
	port := parsed.Port()
	if port == "" {
		port = "80"
		if parsed.Scheme == "https" {
			port = "443"
		}
	}
	portNum, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}

	// SSRF protection, fast path: resolve hostname and block private/reserved IPs.
	// dialControl checks again on every connection (redirects, DNS rebinding).
	hostname := parsed.Hostname()
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", hostname)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("cannot resolve hostname")
	}
	for _, a := range addrs {
		if !addrAllowed(netip.AddrPortFrom(a.Unmap(), uint16(portNum))) {
			return nil, errBlocked
		}
	}

	result := &Result{}

	// Source = domain name
	source := cleanDomain(hostname)
	if source != "" {
		result.Source = &source
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return result, nil // return what we have (source)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,fr;q=0.8")

	resp, err := httpClient.Do(req)
	if err != nil {
		if errors.Is(err, errBlocked) {
			return nil, errBlocked
		}
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
	applyMetaTags(result, page, isBoardHost(hostname))

	return result, nil
}

// --- JSON-LD extraction ---

// jobPostingLD holds the JobPosting properties we use, read leniently from the
// decoded node: one property of an unexpected shape must not hide the others.
type jobPostingLD struct {
	Title              string
	Description        string
	EmploymentType     interface{} // string or []string
	JobLocationType    string      // "TELECOMMUTE"
	HiringOrganization interface{} // object, []object or a plain name
	JobLocation        interface{} // object or []object
	BaseSalary         interface{} // object
}

var jsonLDRegex = regexp.MustCompile(`(?i)<script[^>]*type\s*=\s*["']?application/ld\+json["']?[^>]*>([\s\S]*?)</script>`)

func extractJobPostingLD(page string) *jobPostingLD {
	matches := jsonLDRegex.FindAllStringSubmatch(page, -1)
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		if raw == "" {
			continue
		}

		var doc interface{}
		if json.Unmarshal([]byte(raw), &doc) != nil {
			continue
		}

		// A block holds a single node, an array of nodes or an @graph wrapper
		if node := findJobPosting(doc, 0); node != nil {
			return &jobPostingLD{
				Title:              firstString(node["title"]),
				Description:        firstString(node["description"]),
				EmploymentType:     node["employmentType"],
				JobLocationType:    firstString(node["jobLocationType"]),
				HiringOrganization: node["hiringOrganization"],
				JobLocation:        node["jobLocation"],
				BaseSalary:         node["baseSalary"],
			}
		}
	}
	return nil
}

// findJobPosting returns the first JobPosting node of v, looking into arrays
// and @graph wrappers. Nodes of any other shape are skipped.
func findJobPosting(v interface{}, depth int) map[string]interface{} {
	if depth > 3 {
		return nil
	}
	switch x := v.(type) {
	case map[string]interface{}:
		if isJobPostingType(x["@type"]) {
			return x
		}
		return findJobPosting(x["@graph"], depth+1)
	case []interface{}:
		for _, item := range x {
			if node := findJobPosting(item, depth+1); node != nil {
				return node
			}
		}
	}
	return nil
}

// isJobPostingType accepts "JobPosting", ["JobPosting", ...] and prefixed
// forms such as "schema:JobPosting" or "https://schema.org/JobPosting".
func isJobPostingType(v interface{}) bool {
	for _, t := range stringsOf(v) {
		t = strings.TrimSpace(t)
		if i := strings.LastIndexAny(t, "/:#"); i >= 0 {
			t = t[i+1:]
		}
		if t == "JobPosting" {
			return true
		}
	}
	return false
}

// stringsOf returns v as a list of strings: a string alone, or the string
// items of an array.
func stringsOf(v interface{}) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []interface{}:
		var out []string
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// firstString returns v when it is a string, or the first string of an array.
func firstString(v interface{}) string {
	if list := stringsOf(v); len(list) > 0 {
		return list[0]
	}
	return ""
}

// nameOf reads a text value that may also be an object with a name,
// e.g. "addressCountry": {"@type": "Country", "name": "FR"}.
func nameOf(v interface{}) string {
	if m, ok := v.(map[string]interface{}); ok {
		return firstString(m["name"])
	}
	return firstString(v)
}

func applyJobPosting(r *Result, ld *jobPostingLD) {
	if s := clean(ld.Title); s != "" {
		r.JobTitle = &s
	}

	if s := cleanHTML(ld.Description); s != "" {
		r.JobDescription = &s
	}

	// Hiring organization: an object, a list of objects (first one) or a plain name
	org := ld.HiringOrganization
	if list, ok := org.([]interface{}); ok && len(list) > 0 {
		org = list[0]
	}
	switch o := org.(type) {
	case string:
		if s := clean(o); s != "" {
			r.CompanyName = &s
		}
	case map[string]interface{}:
		if s := clean(firstString(o["name"])); s != "" {
			r.CompanyName = &s
		}
		// A single sameAs is usually the company website and wins over url.
		// A list of sameAs mostly holds social profiles, so url comes first then.
		sameAs := stringsOf(o["sameAs"])
		var sites []string
		if len(sameAs) == 1 {
			sites = append(sites, sameAs[0])
		}
		sites = append(sites, stringsOf(o["url"])...)
		if len(sameAs) > 1 {
			sites = append(sites, sameAs...)
		}
		for _, site := range sites {
			if s := companyWebsite(clean(site)); s != "" {
				r.CompanyWebsite = &s
				break
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
	switch v := raw.(type) {
	case map[string]interface{}:
		return parseAddress(v["address"])
	case []interface{}:
		// Several locations: keep the first one with a usable address
		for _, item := range v {
			if loc, ok := item.(map[string]interface{}); ok {
				if s := parseAddress(loc["address"]); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func parseAddress(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return clean(v)
	case map[string]interface{}:
		parts := []string{}
		for _, key := range []string{"addressLocality", "addressRegion", "addressCountry"} {
			if s := clean(nameOf(v[key])); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
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
	sal, ok := raw.(map[string]interface{})
	if !ok {
		return
	}

	currency := strings.ToUpper(clean(firstString(sal["currency"])))
	if currency == "" {
		return
	}

	// value is a QuantitativeValue (minValue/maxValue or value, plus unitText),
	// or directly a number or a numeric string
	var min, max, single float64
	unit := ""
	switch v := sal["value"].(type) {
	case map[string]interface{}:
		min = toNumber(v["minValue"])
		max = toNumber(v["maxValue"])
		single = toNumber(v["value"])
		unit = strings.ToUpper(strings.TrimSpace(firstString(v["unitText"])))
	default:
		single = toNumber(v)
	}

	// Only accept yearly salaries (sensible range)
	switch unit {
	case "YEAR", "YEARLY", "ANNUAL", "ANNUALLY", "":
		// already yearly, or no unit (assume yearly for large values)
	case "MONTH", "MONTHLY":
		min *= 12
		max *= 12
		single *= 12
	default:
		return // hourly, weekly, etc — too unreliable to convert
	}

	// Sanity check: salary between 10k and 1M.
	// Keep one value: the top of the range when sensible, else the single
	// value, else the bottom of the range.
	for _, val := range []float64{max, single, min} {
		if val >= 10000 && val <= 1000000 {
			salaryInt := int(val)
			r.Salary = &salaryInt
			r.SalaryCurrency = &currency
			return
		}
	}
}

// toNumber reads a JSON number or a numeric string such as "45000",
// "45,000.00" or "45 000". Anything else gives 0, which the range check rejects.
func toNumber(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		s := strings.Map(func(r rune) rune {
			if r == ',' || unicode.IsSpace(r) {
				return -1
			}
			return r
		}, x)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return 0
}

// --- Meta tags fallback ---

// A meta tag ends at the first '>' outside a quoted value
var metaRegex = regexp.MustCompile(`(?i)<meta\s+((?:"[^"]*"|'[^']*'|[^>"'])+)>`)

// An attribute value is double-quoted, single-quoted or bare; each attribute is
// matched whole so a quote of one kind can sit inside a value of the other.
var metaAttrRegex = regexp.MustCompile(`(?:^|\s)([^\s"'=<>/]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
var titleRegex = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func applyMetaTags(r *Result, page string, onBoard bool) {
	meta := parseMetaTags(page)

	// Only fill what's still missing
	if r.JobTitle == nil {
		if s := clean(meta["og:title"]); s != "" {
			r.JobTitle = &s
		} else if s := extractTitle(page); s != "" {
			r.JobTitle = &s
		}
	}

	// og:site_name names the site: the company on its own career pages, but
	// the job board itself on a board
	if r.CompanyName == nil && !onBoard {
		if s := clean(meta["og:site_name"]); s != "" && (r.Source == nil || !strings.EqualFold(s, *r.Source)) {
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
			// Only one of the three value groups matched
			attrs[strings.ToLower(a[1])] = a[2] + a[3] + a[4]
		}
		key := attrs["property"]
		if key == "" {
			key = attrs["name"]
		}
		if key != "" && attrs["content"] != "" {
			result[strings.ToLower(key)] = attrs["content"]
		}
	}
	return result
}

func extractTitle(page string) string {
	m := titleRegex.FindStringSubmatch(page)
	if len(m) < 2 {
		return ""
	}
	return clean(multiSpaceRegex.ReplaceAllString(m[1], " "))
}

// --- Helpers ---

var htmlTagRegex = regexp.MustCompile(`<[^>]+>`)
var multiSpaceRegex = regexp.MustCompile(`\s+`)

func clean(s string) string {
	s = html.UnescapeString(s)
	s = strings.TrimSpace(s)
	return truncate(s, 500)
}

func cleanHTML(s string) string {
	s = html.UnescapeString(s)
	s = htmlTagRegex.ReplaceAllString(s, " ")
	s = multiSpaceRegex.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return truncate(s, 5000)
}

// truncate cuts s to at most n bytes without splitting a UTF-8 character.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// companyWebsite returns the http(s) origin of rawURL when it looks like an
// actual company website, or "" for other schemes, malformed hosts and profile
// pages on a job board (e.g. indeed.com/cmp/... or linkedin.com/company/...).
func companyWebsite(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if !isHostname(host) || isBoardHost(host) {
		return ""
	}
	if port := parsed.Port(); port != "" {
		host += ":" + port
	}
	return parsed.Scheme + "://" + host
}

// isHostname reports whether host is made only of dot-separated labels of
// letters, digits and hyphens (IDN labels included).
func isHostname(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return false
		}
		for _, c := range label {
			if c != '-' && !unicode.IsLetter(c) && !unicode.IsDigit(c) {
				return false
			}
		}
	}
	return true
}

// jobBoards maps a host label to the job board name, spelled as in the
// frontend catalog (frontend/src/job-boards.ts). Labels are compared whole:
// fr.indeed.com and indeed.fr match, monsterenergy.com does not.
var jobBoards = map[string]string{
	"indeed":             "Indeed",
	"linkedin":           "LinkedIn",
	"glassdoor":          "Glassdoor",
	"monster":            "Monster",
	"welcometothejungle": "Welcome to the Jungle",
	"wttj":               "Welcome to the Jungle",
	"seek":               "Seek",
	"irishjobs":          "IrishJobs.ie",
	"ziprecruiter":       "ZipRecruiter",
	"hellowork":          "HelloWork",
	"cadremploi":         "Cadremploi",
	"apec":               "APEC",
	"francetravail":      "France Travail",
	"pole-emploi":        "France Travail",
	"jobteaser":          "Jobteaser",
	"meteojob":           "Meteojob",
}

// boardName returns the job board name for host, or "" when host is not a known board.
func boardName(host string) string {
	for _, label := range strings.Split(strings.ToLower(host), ".") {
		if name, ok := jobBoards[label]; ok {
			return name
		}
	}
	return ""
}

// isBoardHost reports whether host belongs to a known job board.
func isBoardHost(host string) bool {
	host = strings.ToLower(host)
	// jobs.ie is matched on the whole domain: a "jobs" label alone also names
	// company career sites such as jobs.acme.com
	return boardName(host) != "" || host == "jobs.ie" || strings.HasSuffix(host, ".jobs.ie")
}

func cleanDomain(host string) string {
	host = strings.ToLower(host)
	host = strings.TrimPrefix(host, "www.")
	// Known job boards get their catalog name (handles ie.indeed.com, fr.linkedin.com, etc.)
	if name := boardName(host); name != "" {
		return name
	}
	// Any other site (jobs.ie included) keeps its domain
	return host
}
