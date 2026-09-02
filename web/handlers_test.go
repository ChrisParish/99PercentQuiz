package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("Europe/London unavailable: %v", err)
	}
	qs := testPool()
	s, err := NewServer(NewRotation(qs, 20260105), len(qs), "Test Quiz", loc)
	if err != nil {
		t.Fatalf("building server: %v", err)
	}
	return s.Handler()
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestRoutesRespond(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{
		"/",
		"/2026-09-02",
		"/print",
		"/print/2026-09-02",
		"/healthz",
		"/static/style.css",
		"/static/quiz.js",
	} {
		if rec := get(t, h, path); rec.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", path, rec.Code)
		}
	}
}

func TestRejectsBadDates(t *testing.T) {
	h := newTestServer(t)
	cases := []struct {
		path string
		want int
		note string
	}{
		{"/2026-13-45", http.StatusNotFound, "impossible month and day"},
		{"/2026-1-5", http.StatusNotFound, "non-canonical, missing zero padding"},
		{"/not-a-date", http.StatusNotFound, "not a date at all"},
		{"/2026-09-02extra", http.StatusNotFound, "trailing junk"},
		{"/2025-12-31", http.StatusBadRequest, "valid date but before the epoch"},
		{"/2099-01-01", http.StatusBadRequest, "valid date but beyond the horizon"},
	}
	for _, c := range cases {
		if rec := get(t, h, c.path); rec.Code != c.want {
			t.Errorf("GET %s (%s): got %d, want %d", c.path, c.note, rec.Code, c.want)
		}
	}
}

func TestNoPathTraversal(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{
		"/static/../main.go",
		"/static/..%2fmain.go",
		"/%2e%2e/etc/passwd",
		"/static/../../questions.go",
	} {
		rec := get(t, h, path)
		if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "package main") {
			t.Errorf("GET %s leaked source", path)
		}
	}
}

func TestWeekendRollsForwardToMonday(t *testing.T) {
	h := newTestServer(t)

	sat := get(t, h, "/2026-09-05").Body.String() // Saturday
	sun := get(t, h, "/2026-09-06").Body.String() // Sunday
	mon := get(t, h, "/2026-09-07").Body.String() // Monday

	for name, body := range map[string]string{"Saturday": sat, "Sunday": sun} {
		if !strings.Contains(body, "Monday, 7 September 2026") {
			t.Errorf("%s page does not show the resolved Monday date", name)
		}
		if !strings.Contains(body, "showing the next weekday") {
			t.Errorf("%s page does not explain that it rolled forward", name)
		}
	}
	if !strings.Contains(mon, "Monday, 7 September 2026") {
		t.Error("Monday page shows the wrong date")
	}
	if strings.Contains(mon, "showing the next weekday") {
		t.Error("Monday page should not show the weekend notice")
	}
}

func TestWeekendServesTheSameQuestionsAsMonday(t *testing.T) {
	h := newTestServer(t)
	sat := questionsIn(get(t, h, "/2026-09-05").Body.String())
	mon := questionsIn(get(t, h, "/2026-09-07").Body.String())

	if len(mon) != QuestionsPerDay {
		t.Fatalf("Monday page rendered %d questions, want %d", len(mon), QuestionsPerDay)
	}
	if strings.Join(sat, "|") != strings.Join(mon, "|") {
		t.Error("Saturday and Monday served different question sets")
	}
}

func questionsIn(body string) []string {
	var out []string
	for _, part := range strings.Split(body, `<span class="question">`)[1:] {
		if end := strings.Index(part, "</span>"); end >= 0 {
			out = append(out, part[:end])
		}
	}
	if len(out) == 0 {
		for _, part := range strings.Split(body, `<p class="question">`)[1:] {
			if end := strings.Index(part, "</p>"); end >= 0 {
				out = append(out, part[:end])
			}
		}
	}
	return out
}

func TestPrintViewShowsAnswersInline(t *testing.T) {
	h := newTestServer(t)
	body := get(t, h, "/print/2026-09-02").Body.String()

	if strings.Contains(body, "<details>") {
		t.Error("print view should not hide answers behind a disclosure")
	}
	if !strings.Contains(body, `class="answer"`) {
		t.Error("print view does not render answers")
	}
	if got := len(questionsIn(body)); got != QuestionsPerDay {
		t.Errorf("print view rendered %d questions, want %d", got, QuestionsPerDay)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/2026-09-02")
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s: got %q, want %q", header, got, want)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP missing frame-ancestors: %q", csp)
	}
}

func TestHealthReportsTimezoneAndCount(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}

	var body struct {
		Status    string `json:"status"`
		Questions int    `json:"questions"`
		Timezone  string `json:"timezone"`
		Date      string `json:"date"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("health response is not JSON: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status %q, want ok", body.Status)
	}
	if body.Questions != 400 {
		t.Errorf("questions %d, want 400", body.Questions)
	}
	// Catches a runtime image with no tzdata, where this silently becomes UTC.
	if body.Timezone != "Europe/London" {
		t.Errorf("timezone %q, want Europe/London", body.Timezone)
	}
	if body.Date == "" {
		t.Error("health response has no date")
	}
}

func TestEmbeddedQuestionsRenderEndToEnd(t *testing.T) {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	qs, err := loadQuestions()
	if err != nil {
		t.Fatalf("loading embedded questions: %v", err)
	}
	s, err := NewServer(NewRotation(qs, 20260105), len(qs), "The 99% Quiz", loc)
	if err != nil {
		t.Fatal(err)
	}
	rec := get(t, s.Handler(), "/2026-09-02")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if got := len(questionsIn(rec.Body.String())); got != QuestionsPerDay {
		t.Errorf("rendered %d questions, want %d", got, QuestionsPerDay)
	}
}
