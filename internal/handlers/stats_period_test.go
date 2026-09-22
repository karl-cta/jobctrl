package handlers

import (
	"testing"
	"time"
)

func utcTime(y int, mo time.Month, d, h, mi, s, ns int) time.Time {
	return time.Date(y, mo, d, h, mi, s, ns, time.UTC)
}

func TestParseDBTime(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   time.Time
		wantOK bool
	}{
		{
			name:   "sqlite datetime",
			in:     "2024-03-15 09:30:00",
			want:   utcTime(2024, time.March, 15, 9, 30, 0, 0),
			wantOK: true,
		},
		{
			name:   "sqlite datetime surrounded by whitespace",
			in:     "  2024-03-15 09:30:00\n",
			want:   utcTime(2024, time.March, 15, 9, 30, 0, 0),
			wantOK: true,
		},
		{
			name:   "sqlite datetime with fractional seconds",
			in:     "2024-03-15 09:30:00.500",
			want:   utcTime(2024, time.March, 15, 9, 30, 0, 500000000),
			wantOK: true,
		},
		{
			name:   "rfc3339 zulu",
			in:     "2024-03-15T09:30:00Z",
			want:   utcTime(2024, time.March, 15, 9, 30, 0, 0),
			wantOK: true,
		},
		{
			name:   "rfc3339 nano zulu",
			in:     "2024-03-15T09:30:00.123456789Z",
			want:   utcTime(2024, time.March, 15, 9, 30, 0, 123456789),
			wantOK: true,
		},
		{
			name:   "rfc3339 offset is normalised to utc",
			in:     "2024-03-15T11:30:00+02:00",
			want:   utcTime(2024, time.March, 15, 9, 30, 0, 0),
			wantOK: true,
		},
		{
			name:   "iso without zone is read as utc",
			in:     "2024-03-15T09:30:00",
			want:   utcTime(2024, time.March, 15, 9, 30, 0, 0),
			wantOK: true,
		},
		{
			name:   "space separated with nano offset",
			in:     "2024-03-15 11:30:00.250000000+02:00",
			want:   utcTime(2024, time.March, 15, 9, 30, 0, 250000000),
			wantOK: true,
		},
		{
			name:   "space separated with offset",
			in:     "2024-03-15 11:30:00+02:00",
			want:   utcTime(2024, time.March, 15, 9, 30, 0, 0),
			wantOK: true,
		},
		{
			name:   "date only",
			in:     "2024-03-15",
			want:   utcTime(2024, time.March, 15, 0, 0, 0, 0),
			wantOK: true,
		},
		{name: "empty string", in: "", wantOK: false},
		{name: "whitespace only", in: "   \t\n", wantOK: false},
		{name: "free text", in: "not a time", wantOK: false},
		{name: "day first format", in: "15/03/2024", wantOK: false},
		{name: "unix seconds", in: "1710495000", wantOK: false},
		{name: "time only", in: "09:30:00", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseDBTime(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("parseDBTime(%q) ok = %v, want %v (got %v)", tc.in, ok, tc.wantOK, got)
			}
			if !tc.wantOK {
				if !got.IsZero() {
					t.Errorf("parseDBTime(%q) should return the zero time on failure, got %v", tc.in, got)
				}
				return
			}
			if !got.Equal(tc.want) {
				t.Errorf("parseDBTime(%q) = %v, want %v", tc.in, got, tc.want)
			}
			if loc := got.Location(); loc != time.UTC {
				t.Errorf("parseDBTime(%q) location = %v, want UTC", tc.in, loc)
			}
		})
	}
}

func TestScanTime(t *testing.T) {
	want := utcTime(2024, time.March, 15, 9, 30, 0, 0)
	tests := []struct {
		name   string
		in     any
		want   time.Time
		wantOK bool
	}{
		{"nil", nil, time.Time{}, false},
		{"time.Time in a non-utc zone", want.In(time.FixedZone("CEST", 2*3600)), want, true},
		{"string", "2024-03-15 09:30:00", want, true},
		{"bytes", []byte("2024-03-15 09:30:00"), want, true},
		{"unparsable string", "nope", time.Time{}, false},
		{"unsupported type", int64(42), time.Time{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scanTime(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("scanTime(%v) ok = %v, want %v", tc.in, ok, tc.wantOK)
			}
			if ok && !got.Equal(tc.want) {
				t.Errorf("scanTime(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestMondayOf(t *testing.T) {
	tests := []struct {
		name string
		in   string // RFC3339
		want string // YYYY-MM-DD
	}{
		{"monday at midnight stays put", "2024-03-11T00:00:00Z", "2024-03-11"},
		{"monday at the last second of the day", "2024-03-11T23:59:59Z", "2024-03-11"},
		{"tuesday", "2024-03-12T13:45:00Z", "2024-03-11"},
		{"saturday", "2024-03-16T08:00:00Z", "2024-03-11"},
		{"sunday is the end of the week, not the start", "2024-03-17T23:00:00Z", "2024-03-11"},
		{"next monday rolls over", "2024-03-18T00:00:00Z", "2024-03-18"},
		{"offset is converted to utc before bucketing", "2024-03-11T00:30:00+02:00", "2024-03-04"},
		{"week spanning a month boundary", "2024-03-01T12:00:00Z", "2024-02-26"},
		{"week spanning a leap day", "2024-02-29T12:00:00Z", "2024-02-26"},
		{"week spanning a year boundary", "2025-01-01T12:00:00Z", "2024-12-30"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in, err := time.Parse(time.RFC3339, tc.in)
			if err != nil {
				t.Fatalf("bad test input %q: %v", tc.in, err)
			}
			got := mondayOf(in)
			if got.Format("2006-01-02") != tc.want {
				t.Errorf("mondayOf(%s) = %s, want %s", tc.in, got.Format("2006-01-02"), tc.want)
			}
			if got.Weekday() != time.Monday {
				t.Errorf("mondayOf(%s) weekday = %v, want Monday", tc.in, got.Weekday())
			}
			if h, m, s := got.Clock(); h != 0 || m != 0 || s != 0 || got.Nanosecond() != 0 {
				t.Errorf("mondayOf(%s) = %v, want midnight", tc.in, got)
			}
			if got.Location() != time.UTC {
				t.Errorf("mondayOf(%s) location = %v, want UTC", tc.in, got.Location())
			}
			// Idempotent: the Monday of a Monday is itself.
			if again := mondayOf(got); !again.Equal(got) {
				t.Errorf("mondayOf not idempotent: %v -> %v", got, again)
			}
		})
	}
}

func TestBucketIndex(t *testing.T) {
	start := utcTime(2024, time.March, 1, 0, 0, 0, 0)
	end := start.Add(8 * time.Hour)

	tests := []struct {
		name  string
		t     time.Time
		start time.Time
		end   time.Time
		n     int
		want  int
	}{
		{"start of range is the first bucket", start, start, end, 8, 0},
		{"one nanosecond in is still the first bucket", start.Add(time.Nanosecond), start, end, 8, 0},
		{"last instant of the first bucket", start.Add(time.Hour - time.Nanosecond), start, end, 8, 0},
		{"exact bucket boundary belongs to the later bucket", start.Add(time.Hour), start, end, 8, 1},
		{"midway through a bucket", start.Add(3*time.Hour + 30*time.Minute), start, end, 8, 3},
		{"last instant of the range", end.Add(-time.Nanosecond), start, end, 8, 7},
		{"end is exclusive", end, start, end, 8, -1},
		{"after the range", end.Add(time.Hour), start, end, 8, -1},
		{"before the range", start.Add(-time.Nanosecond), start, end, 8, -1},
		{"single bucket covers the whole range", start.Add(5 * time.Hour), start, end, 1, 0},
		{"zero buckets", start, start, end, 0, -1},
		{"negative bucket count", start, start, end, -3, -1},
		{"empty range", start, start, start, 8, -1},
		{"reversed range", start, end, start, 8, -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := bucketIndex(tc.t, tc.start, tc.end, tc.n); got != tc.want {
				t.Errorf("bucketIndex(%v, %v, %v, %d) = %d, want %d",
					tc.t, tc.start, tc.end, tc.n, got, tc.want)
			}
		})
	}
}

func TestStatusChangeTarget(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		{"standard message", "Status changed from Applied to Screening", "Screening"},
		{"target with spaces", "Status changed from Applied to On hold", "On hold"},
		{"no separator", "Application created", ""},
		{"empty", "", ""},
		{"last separator wins", "Status changed from Applied to Offer to Accepted", "Accepted"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusChangeTarget(tc.desc); got != tc.want {
				t.Errorf("statusChangeTarget(%q) = %q, want %q", tc.desc, got, tc.want)
			}
		})
	}
}

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{"empty means default", "", defaultPeriodDays},
		{"thirty", "30", 30},
		{"ninety", "90", 90},
		{"year", "365", 365},
		{"all time is zero", "all", 0},
		{"padded value is trimmed", "  30  ", 30},
		{"unknown value falls back to default", "bogus", defaultPeriodDays},
		{"numeric but unsupported", "7", defaultPeriodDays},
		{"negative", "-30", defaultPeriodDays},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parsePeriod(tc.raw); got != tc.want {
				t.Errorf("parsePeriod(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}
