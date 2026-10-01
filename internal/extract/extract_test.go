package extract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// --- IP policy ---

func TestIsBlockedAddr(t *testing.T) {
	blocked := []string{
		"0.0.0.0", "0.1.2.3", "10.0.0.1", "100.64.0.1", "100.100.100.200", "127.0.0.1",
		"169.254.169.254", "172.16.0.1", "172.17.0.1", "192.0.0.1", "192.0.2.1", "192.168.1.1",
		"198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "239.255.255.250",
		"240.0.0.1", "255.255.255.255",
		"::", "::1", "::127.0.0.1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:169.254.169.254",
		"::ffff:100.100.100.200", "::ffff:198.18.0.1", "::ffff:0.1.2.3",
		"64:ff9b::7f00:1", "64:ff9b::6464:64c8", "64:ff9b::a9fe:a9fe", "64:ff9b::a00:1", "64:ff9b:1::808:808",
		"100::1", "2001::1", "2001:db8::1", "2002:7f00:1::", "2002:808:808::",
		"fc00::1", "fd00::1", "fd00:ec2::254", "fe80::1", "fe80::1%en0", "fec0::1",
		"ff02::1", "ff05::1",
	}
	allowed := []string{
		"8.8.8.8", "1.1.1.1", "93.184.216.34", "::ffff:8.8.8.8",
		"2606:4700:4700::1111", "2a00:1450:4007:80b::200e",
		"64:ff9b::808:808", // NAT64 to a public IPv4 address
	}
	for _, s := range blocked {
		if !isBlockedAddr(netip.MustParseAddr(s)) {
			t.Errorf("isBlockedAddr(%s) = false, want true", s)
		}
	}
	for _, s := range allowed {
		if isBlockedAddr(netip.MustParseAddr(s)) {
			t.Errorf("isBlockedAddr(%s) = true, want false", s)
		}
	}
	if !isBlockedAddr(netip.Addr{}) {
		t.Error("isBlockedAddr(zero Addr) = false, want true")
	}
}

func TestDialControl(t *testing.T) {
	for _, address := range []string{"10.0.0.1:80", "127.0.0.1:8080", "[::ffff:127.0.0.1]:443", "[fe80::1%en0]:80", "[::1]:80", "not-an-address"} {
		if err := dialControl("tcp", address, nil); !errors.Is(err, errBlocked) {
			t.Errorf("dialControl(%q) = %v, want errBlocked", address, err)
		}
	}
	for _, address := range []string{"8.8.8.8:443", "[2606:4700:4700::1111]:80"} {
		if err := dialControl("tcp", address, nil); err != nil {
			t.Errorf("dialControl(%q) = %v, want nil", address, err)
		}
	}
}

func TestCheckRedirect(t *testing.T) {
	req := func(raw string) *http.Request {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Request{URL: u}
	}
	if err := checkRedirect(req("https://example.com/next"), make([]*http.Request, 1)); err != nil {
		t.Errorf("https redirect refused: %v", err)
	}
	if err := checkRedirect(req("ftp://example.com/file"), make([]*http.Request, 1)); err == nil {
		t.Error("ftp redirect accepted")
	}
	if err := checkRedirect(req("https://example.com/next"), make([]*http.Request, 5)); err == nil {
		t.Error("sixth redirect accepted")
	}
}

func TestFromURLRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"http://example.com/" + strings.Repeat("a", 2030), "URL too long"},
		{"ftp://example.com/job", "invalid URL"},
		{"javascript:alert(1)", "invalid URL"},
		{"http://", "invalid URL"},
		{"http://example.com:99999/", "invalid URL"},
		{"http://127.0.0.1/", "URL points to a private network"},
		{"http://10.0.0.1:8080/", "URL points to a private network"},
		{"http://169.254.169.254/latest/meta-data/", "URL points to a private network"},
		{"http://[::ffff:192.168.1.1]/", "URL points to a private network"},
		{"http://[::1]:8080/", "URL points to a private network"},
	}
	for _, c := range cases {
		res, err := FromURL(c.url)
		if err == nil || err.Error() != c.want {
			t.Errorf("FromURL(%.40q) = %v, %v; want error %q", c.url, res, err, c.want)
		}
	}
}

// --- Fetching through httptest servers ---

// allowOnly makes srv's address:port the only one extraction may connect to.
func allowOnly(t *testing.T, srv *httptest.Server) {
	t.Helper()
	allowed := netip.MustParseAddrPort(srv.Listener.Addr().String())
	prev := addrAllowed
	addrAllowed = func(ap netip.AddrPort) bool { return ap == allowed }
	t.Cleanup(func() {
		addrAllowed = prev
		httpClient.CloseIdleConnections()
	})
}

// countingServer returns a server that counts the requests it receives.
func countingServer(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

const jobPage = `<!doctype html><html><head>
<title>Fallback title</title>
<meta property="og:title" content="OG title">
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
 "title":"Développeuse Go",
 "description":"&lt;p&gt;Rejoignez l&#39;équipe&lt;/p&gt;",
 "employmentType":"FULL_TIME",
 "hiringOrganization":{"@type":"Organization","name":"Acme","sameAs":"https://www.acme.com/about"},
 "jobLocation":{"@type":"Place","address":{"addressLocality":"Lyon","addressCountry":{"@type":"Country","name":"FR"}}},
 "baseSalary":{"@type":"MonetaryAmount","currency":"EUR","value":{"@type":"QuantitativeValue","minValue":45000,"maxValue":55000,"unitText":"YEAR"}}}
</script>
</head><body></body></html>`

func TestFromURLContextExtractsPage(t *testing.T) {
	srv, _ := countingServer(t, jobPage)
	allowOnly(t, srv)

	res, err := FromURLContext(context.Background(), srv.URL+"/jobs/42")
	if err != nil {
		t.Fatalf("FromURLContext: %v", err)
	}
	checks := map[string][2]string{
		"source":          {str(res.Source), "127.0.0.1"},
		"job_title":       {str(res.JobTitle), "Développeuse Go"},
		"job_description": {str(res.JobDescription), "Rejoignez l'équipe"},
		"company_name":    {str(res.CompanyName), "Acme"},
		"company_website": {str(res.CompanyWebsite), "https://www.acme.com"},
		"location":        {str(res.Location), "Lyon, FR"},
		"contract_type":   {str(res.ContractType), "CDI"},
		"salary_currency": {str(res.SalaryCurrency), "EUR"},
	}
	for field, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", field, c[0], c[1])
		}
	}
	if res.Salary == nil || *res.Salary != 55000 {
		t.Errorf("salary = %v, want 55000", res.Salary)
	}
}

func TestFromURLContextRefusesRedirectToOtherAddress(t *testing.T) {
	internal, internalHits := countingServer(t, "<title>internal admin</title>")
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/secret", http.StatusFound)
	}))
	t.Cleanup(front.Close)
	allowOnly(t, front)

	res, err := FromURLContext(context.Background(), front.URL+"/job")
	if !errors.Is(err, errBlocked) {
		t.Fatalf("FromURLContext = %+v, %v; want errBlocked", res, err)
	}
	if n := internalHits.Load(); n != 0 {
		t.Errorf("internal server received %d requests, want 0", n)
	}
}

func TestFromURLContextFollowsAllowedRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/job" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		fmt.Fprint(w, jobPage)
	}))
	t.Cleanup(srv.Close)
	allowOnly(t, srv)

	res, err := FromURLContext(context.Background(), srv.URL+"/job")
	if err != nil {
		t.Fatalf("FromURLContext: %v", err)
	}
	if str(res.JobTitle) != "Développeuse Go" {
		t.Errorf("job_title = %q, want the page behind the redirect", str(res.JobTitle))
	}
}

func TestFromURLRefusesLoopbackByDefault(t *testing.T) {
	srv, hits := countingServer(t, jobPage)
	t.Cleanup(httpClient.CloseIdleConnections)

	if _, err := FromURL(srv.URL); !errors.Is(err, errBlocked) {
		t.Fatalf("FromURL(loopback) error = %v, want errBlocked", err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("loopback server received %d requests, want 0", n)
	}
}

func TestFromURLContextNon200KeepsSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, jobPage)
	}))
	t.Cleanup(srv.Close)
	allowOnly(t, srv)

	res, err := FromURLContext(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FromURLContext: %v", err)
	}
	if str(res.Source) != "127.0.0.1" || res.JobTitle != nil || res.CompanyName != nil {
		t.Errorf("got %+v, want only Source 127.0.0.1", res)
	}
}

// --- JSON-LD ---

func ldPage(blocks ...string) string {
	var b strings.Builder
	b.WriteString("<html><head>")
	for _, block := range blocks {
		b.WriteString(`<script type="application/ld+json">` + block + `</script>`)
	}
	b.WriteString("</head></html>")
	return b.String()
}

func TestExtractJobPostingLD(t *testing.T) {
	cases := []struct {
		name string
		page string
		want string // title, "" when no JobPosting must be found
	}{
		{"single object", ldPage(`{"@type":"JobPosting","title":"Dev"}`), "Dev"},
		{"array", ldPage(`[{"@type":"WebSite"},{"@type":"JobPosting","title":"Dev"}]`), "Dev"},
		{"graph", ldPage(`{"@context":"https://schema.org","@graph":[{"@type":"Organization"},{"@type":"JobPosting","title":"Dev"}]}`), "Dev"},
		{"graph with odd nodes", ldPage(`{"@graph":[{"@type":["Organization","X"]},42,"text",null,{"@type":"JobPosting","title":"Dev"}]}`), "Dev"},
		{"type array", ldPage(`{"@type":["JobPosting"],"title":"Dev"}`), "Dev"},
		{"prefixed type", ldPage(`{"@type":"schema:JobPosting","title":"Dev"}`), "Dev"},
		{"type URL", ldPage(`{"@type":"http://schema.org/JobPosting","title":"Dev"}`), "Dev"},
		{"title array", ldPage(`{"@type":"JobPosting","title":["Dev","Other"]}`), "Dev"},
		{"industry array", ldPage(`{"@type":"JobPosting","title":"Dev","industry":["IT","Web"]}`), "Dev"},
		{"odd field types", ldPage(`{"@type":"JobPosting","title":"Dev","description":{"x":1},"jobLocationType":5,"baseSalary":"50k"}`), "Dev"},
		{"second block", ldPage(`{"@type":"BreadcrumbList"}`, `{"@type":"JobPosting","title":"Dev"}`), "Dev"},
		{"invalid first block", ldPage(`{"@type":"JobPosting",`, `{"@type":"JobPosting","title":"Dev"}`), "Dev"},
		{"uppercase tag", `<SCRIPT TYPE="application/ld+json">{"@type":"JobPosting","title":"Dev"}</SCRIPT>`, "Dev"},
		{"unquoted type", `<script type=application/ld+json>{"@type":"JobPosting","title":"Dev"}</script>`, "Dev"},
		{"no job posting", ldPage(`{"@type":"Organization","name":"Acme"}`), ""},
		{"no script", `<html><title>Dev</title></html>`, ""},
	}
	for _, c := range cases {
		ld := extractJobPostingLD(c.page)
		switch {
		case c.want == "" && ld != nil:
			t.Errorf("%s: got %+v, want nil", c.name, ld)
		case c.want != "" && ld == nil:
			t.Errorf("%s: got nil, want title %q", c.name, c.want)
		case c.want != "" && ld.Title != c.want:
			t.Errorf("%s: title = %q, want %q", c.name, ld.Title, c.want)
		}
	}
}

// fromLD runs the JSON-LD path on a single JobPosting object.
func fromLD(t *testing.T, node string) *Result {
	t.Helper()
	ld := extractJobPostingLD(ldPage(node))
	if ld == nil {
		t.Fatalf("no JobPosting found in %s", node)
	}
	r := &Result{}
	applyJobPosting(r, ld)
	return r
}

func TestApplyJobPostingOrganization(t *testing.T) {
	cases := []struct {
		org         string
		wantName    string
		wantWebsite string
	}{
		{`"Acme"`, "Acme", ""},
		{`[{"name":"Acme","url":"https://acme.com"},{"name":"Other"}]`, "Acme", "https://acme.com"},
		{`{"name":"Acme","sameAs":"https://www.acme.com/en/about?utm=x"}`, "Acme", "https://www.acme.com"},
		{`{"name":"Acme","sameAs":"https://www.linkedin.com/company/acme","url":"https://acme.com"}`, "Acme", "https://acme.com"},
		{`{"name":"Acme","sameAs":["https://twitter.com/acme","https://www.linkedin.com/company/acme"],"url":"https://acme.io"}`, "Acme", "https://acme.io"},
		{`{"name":"Acme","sameAs":["https://www.linkedin.com/company/acme","https://acme.fr"]}`, "Acme", "https://acme.fr"},
		{`{"name":"Acme","sameAs":"javascript://acme.com/%0Aalert(1)"}`, "Acme", ""},
		{`{"name":["Acme"],"url":5}`, "Acme", ""},
	}
	for _, c := range cases {
		r := fromLD(t, `{"@type":"JobPosting","title":"Dev","hiringOrganization":`+c.org+`}`)
		if str(r.CompanyName) != c.wantName || str(r.CompanyWebsite) != c.wantWebsite {
			t.Errorf("org %s: got name %q website %q, want %q %q", c.org, str(r.CompanyName), str(r.CompanyWebsite), c.wantName, c.wantWebsite)
		}
	}
}

func TestApplyJobPostingLocationAndMode(t *testing.T) {
	cases := []struct {
		fields       string
		wantLocation string
		wantMode     string
	}{
		{`"jobLocation":{"address":{"addressLocality":"Paris","addressRegion":"IDF","addressCountry":"FR"}}`, "Paris, IDF, FR", ""},
		{`"jobLocation":{"address":{"addressLocality":"Dublin","addressCountry":{"@type":"Country","name":"IE"}}}`, "Dublin, IE", ""},
		{`"jobLocation":[{"@type":"Place"},{"address":"Nantes"}]`, "Nantes", ""},
		{`"jobLocation":{"address":{"addressLocality":["Lille"],"postalCode":59000}}`, "Lille", ""},
		{`"jobLocationType":"TELECOMMUTE"`, "", "Remote"},
		{`"jobLocationType":["TELECOMMUTE"]`, "", "Remote"},
	}
	for _, c := range cases {
		r := fromLD(t, `{"@type":"JobPosting","title":"Dev",`+c.fields+`}`)
		if str(r.Location) != c.wantLocation || str(r.WorkMode) != c.wantMode {
			t.Errorf("%s: got location %q mode %q, want %q %q", c.fields, str(r.Location), str(r.WorkMode), c.wantLocation, c.wantMode)
		}
	}
}

func TestMapEmploymentType(t *testing.T) {
	cases := []struct {
		raw  interface{}
		want string
	}{
		{"FULL_TIME", "CDI"},
		{"full-time", "CDI"},
		{"CONTRACT", "CDD"},
		{"TEMPORARY", "CDD"},
		{"INTERN", "Internship"},
		{"CONTRACTOR", "Freelance"},
		{"PART_TIME", ""},
		{[]interface{}{"OTHER", "FREELANCE"}, "Freelance"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := mapEmploymentType(c.raw); got != c.want {
			t.Errorf("mapEmploymentType(%v) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestApplySalary(t *testing.T) {
	cases := []struct {
		name     string
		json     string
		want     int // 0 when no salary must be set
		currency string
	}{
		{"range yearly", `{"currency":"EUR","value":{"minValue":45000,"maxValue":55000,"unitText":"YEAR"}}`, 55000, "EUR"},
		{"range without unit", `{"currency":"eur","value":{"minValue":45000,"maxValue":55000}}`, 55000, "EUR"},
		{"value.value", `{"currency":"EUR","value":{"value":50000,"unitText":"YEAR"}}`, 50000, "EUR"},
		{"value.value string", `{"currency":"EUR","value":{"value":"52 000","unitText":"ANNUAL"}}`, 52000, "EUR"},
		{"bare number", `{"currency":"USD","value":50000}`, 50000, "USD"},
		{"numeric string", `{"currency":"USD","value":"50000"}`, 50000, "USD"},
		{"string bounds", `{"currency":"GBP","value":{"minValue":"45,000.00","maxValue":"55 000"}}`, 55000, "GBP"},
		{"unparseable max", `{"currency":"EUR","value":{"minValue":40000,"maxValue":"n/a"}}`, 40000, "EUR"},
		{"max out of range", `{"currency":"EUR","value":{"minValue":30000,"maxValue":5000000}}`, 30000, "EUR"},
		{"monthly", `{"currency":"EUR","value":{"minValue":3000,"maxValue":4000,"unitText":"MONTH"}}`, 48000, "EUR"},
		{"monthly single", `{"currency":"EUR","value":{"value":3500,"unitText":"MONTHLY"}}`, 42000, "EUR"},
		{"hourly", `{"currency":"EUR","value":{"minValue":25,"maxValue":30,"unitText":"HOUR"}}`, 0, ""},
		{"too low", `{"currency":"EUR","value":9999}`, 0, ""},
		{"too high", `{"currency":"EUR","value":1000001}`, 0, ""},
		{"no currency", `{"value":{"minValue":45000,"maxValue":55000,"unitText":"YEAR"}}`, 0, ""},
		{"not an object", `50000`, 0, ""},
	}
	for _, c := range cases {
		var raw interface{}
		if err := json.Unmarshal([]byte(c.json), &raw); err != nil {
			t.Fatalf("%s: bad fixture: %v", c.name, err)
		}
		r := &Result{}
		applySalary(r, raw)
		got := 0
		if r.Salary != nil {
			got = *r.Salary
		}
		if got != c.want || str(r.SalaryCurrency) != c.currency {
			t.Errorf("%s: got %d %q, want %d %q", c.name, got, str(r.SalaryCurrency), c.want, c.currency)
		}
	}
}

// --- Meta tags and title ---

func TestParseMetaTags(t *testing.T) {
	page := `<head>
<meta property="og:title" content="Développeur d'applications chez L'Oréal">
<meta name="description" content="Rejoignez l'équipe">
<meta property='og:description' content='Say "hello" to the team'>
<META PROPERTY="og:site_name" CONTENT="Acme">
<meta name="twitter:title" content="a > b" />
<meta
  name="keywords"
  content="go, sqlite">
<meta name="author" content="A" data-name="x">
<meta name="Robots" content="noindex">
<meta name="empty" content="">
</head>`
	meta := parseMetaTags(page)
	want := map[string]string{
		"og:title":       "Développeur d'applications chez L'Oréal",
		"description":    "Rejoignez l'équipe",
		"og:description": `Say "hello" to the team`,
		"og:site_name":   "Acme",
		"twitter:title":  "a > b",
		"keywords":       "go, sqlite",
		"author":         "A",
		"robots":         "noindex",
	}
	for k, v := range want {
		if meta[k] != v {
			t.Errorf("meta[%q] = %q, want %q", k, meta[k], v)
		}
	}
	if len(meta) != len(want) {
		t.Errorf("got keys %v, want exactly %d keys", meta, len(want))
	}
}

func TestApplyMetaTags(t *testing.T) {
	page := `<title>Ignored</title>
<meta property="og:title" content="Chargée d&#39;affaires">
<meta property="og:site_name" content="Acme">
<meta name="description" content="Rejoignez l'équipe">`

	r := &Result{}
	applyMetaTags(r, page, false)
	if str(r.JobTitle) != "Chargée d'affaires" || str(r.CompanyName) != "Acme" || str(r.JobDescription) != "Rejoignez l'équipe" {
		t.Errorf("company site: got %q / %q / %q", str(r.JobTitle), str(r.CompanyName), str(r.JobDescription))
	}

	// On a job board, og:site_name names the board, not the employer
	r = &Result{}
	applyMetaTags(r, page, true)
	if r.CompanyName != nil {
		t.Errorf("job board: company_name = %q, want nil", *r.CompanyName)
	}

	// Nor when it only repeats the source
	source := "acme.com"
	r = &Result{Source: &source}
	applyMetaTags(r, `<meta property="og:site_name" content="ACME.com">`, false)
	if r.CompanyName != nil {
		t.Errorf("site name equal to source: company_name = %q, want nil", *r.CompanyName)
	}

	// JSON-LD values are kept
	title := "From JSON-LD"
	r = &Result{JobTitle: &title}
	applyMetaTags(r, page, false)
	if str(r.JobTitle) != title {
		t.Errorf("job_title overwritten: %q", str(r.JobTitle))
	}
}

func TestExtractTitle(t *testing.T) {
	cases := map[string]string{
		"<title>Dev Go</title>":                         "Dev Go",
		"<title>\n  Développeur Go\n  - Acme\n</title>": "Développeur Go - Acme",
		`<TITLE lang="fr">Dev</TITLE>`:                  "Dev",
		"<html></html>":                                 "",
	}
	for page, want := range cases {
		if got := extractTitle(page); got != want {
			t.Errorf("extractTitle(%q) = %q, want %q", page, got, want)
		}
	}
}

// --- Text helpers ---

func TestTruncateKeepsUTF8(t *testing.T) {
	s := clean(strings.Repeat("a", 499) + "éé")
	if !utf8.ValidString(s) || s != strings.Repeat("a", 499) {
		t.Errorf("clean cut a character: len %d valid %v", len(s), utf8.ValidString(s))
	}

	s = cleanHTML("<p>" + strings.Repeat("a", 4999) + "é€</p>")
	if !utf8.ValidString(s) || len(s) > 5000 {
		t.Errorf("cleanHTML cut a character: len %d valid %v", len(s), utf8.ValidString(s))
	}

	if got := truncate("ééé", 3); got != "é" {
		t.Errorf("truncate = %q, want %q", got, "é")
	}
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("truncate short = %q", got)
	}
}

func TestCleanDomain(t *testing.T) {
	cases := map[string]string{
		"www.linkedin.com":          "LinkedIn",
		"fr.indeed.com":             "Indeed",
		"ie.indeed.com":             "Indeed",
		"jobs.linkedin.com":         "LinkedIn",
		"www.hellowork.com":         "HelloWork",
		"candidat.francetravail.fr": "France Travail",
		"www.seek.com.au":           "Seek",
		"www.irishjobs.ie":          "IrishJobs.ie",
		"www.jobs.ie":               "jobs.ie",
		"WWW.Example.COM":           "example.com",
		"jobs.acme.com":             "jobs.acme.com",
		"monsterenergy.com":         "monsterenergy.com",
	}
	for host, want := range cases {
		if got := cleanDomain(host); got != want {
			t.Errorf("cleanDomain(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestCompanyWebsite(t *testing.T) {
	cases := map[string]string{
		"https://www.acme.com/en/careers?x=1": "https://www.acme.com",
		"http://acme.com:8080/x":              "http://acme.com:8080",
		"HTTPS://ACME.com/":                   "https://acme.com",
		"https://acme.com./":                  "https://acme.com",
		"https://user:pw@acme.com/":           "https://acme.com",
		"https://société.fr/":                 "https://société.fr",
		"https://jobs.acme.com":               "https://jobs.acme.com",
		"https://www.monsterenergy.com":       "https://www.monsterenergy.com",
		"https://seekingalpha.com":            "https://seekingalpha.com",
		"https://careers.seekwell.io":         "https://careers.seekwell.io",
		"https://fr.indeed.com/cmp/acme":      "",
		"https://www.linkedin.com/company/x":  "",
		"https://www.glassdoor.fr/Overview":   "",
		"https://www.seek.com.au/companies/x": "",
		"https://www.jobs.ie/company/x":       "",
		"https://www.hellowork.com/fr-fr/x":   "",
		"javascript://acme.com/%0Aalert(1)":   "",
		"ftp://acme.com":                      "",
		`https://acme.com"onerror="alert(1)`:  "",
		"https://acme.com'x/":                 "",
		"https://a(b).com/":                   "",
		"https://[::1]/":                      "",
		"https://acme..com/":                  "",
		"/relative/path":                      "",
		"":                                    "",
	}
	for raw, want := range cases {
		if got := companyWebsite(raw); got != want {
			t.Errorf("companyWebsite(%q) = %q, want %q", raw, got, want)
		}
	}
}
