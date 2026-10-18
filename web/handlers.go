package main

import (
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

const isoLayout = "2006-01-02"

type Server struct {
	rotation  *Rotation
	templates *template.Template
	title     string
	loc       *time.Location
	questions int
}

func NewServer(rotation *Rotation, questionCount int, title string, loc *time.Location) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{
		rotation:  rotation,
		templates: tmpl,
		title:     title,
		loc:       loc,
		questions: questionCount,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleToday)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /print", s.handlePrintToday)
	mux.HandleFunc("GET /print/{date}", s.handlePrintDate)
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("POST /{date}/swap/{slot}", s.handleSwap)
	mux.HandleFunc("GET /{date}", s.handleDate)
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		// "no-referrer" would also blank the Origin header on the swap form's
		// same-origin POST (browsers coerce it to the literal string "null"
		// when the referrer policy is no-referrer), breaking the CSRF check.
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy",
			"default-src 'none'; style-src 'self'; script-src 'self'; img-src 'self' data:; "+
				"base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

type pageData struct {
	Title     string
	DateISO   string
	DateLong  string
	DayNumber int
	Questions []Question
	PrevISO   string
	NextISO   string
	PrintISO  string
	Resolved  bool // true when a weekend was rolled forward
}

func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, time.Now().In(s.loc), "quiz.html")
}

func (s *Server) handlePrintToday(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, time.Now().In(s.loc), "print.html")
}

func (s *Server) handleDate(w http.ResponseWriter, r *http.Request) {
	day, ok := parseISODate(r.PathValue("date"))
	if !ok {
		http.Error(w, "Not found. Use a date in the form /YYYY-MM-DD.", http.StatusNotFound)
		return
	}
	s.render(w, r, day, "quiz.html")
}

func (s *Server) handlePrintDate(w http.ResponseWriter, r *http.Request) {
	day, ok := parseISODate(r.PathValue("date"))
	if !ok {
		http.Error(w, "Not found. Use a date in the form /print/YYYY-MM-DD.", http.StatusNotFound)
		return
	}
	s.render(w, r, day, "print.html")
}

func (s *Server) render(w http.ResponseWriter, _ *http.Request, day time.Time, tmpl string) {
	index, err := WeekdayIndex(day)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resolved := ResolveWeekday(day)

	data := pageData{
		Title:     s.title,
		DateISO:   resolved.Format(isoLayout),
		DateLong:  resolved.Format("Monday, 2 January 2006"),
		DayNumber: index + 1,
		Questions: s.rotation.Questions(index),
		PrevISO:   prevWeekday(resolved).Format(isoLayout),
		NextISO:   nextWeekday(resolved).Format(isoLayout),
		PrintISO:  resolved.Format(isoLayout),
		Resolved:  !sameDate(day, resolved),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.templates.ExecuteTemplate(w, tmpl, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// handleSwap replaces the question in one slot of one day with the next
// eligible alternative. The change is global - it is visible to every
// subsequent visitor to that date, not just the one who clicked.
func (s *Server) handleSwap(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "cross-origin requests are not allowed", http.StatusForbidden)
		return
	}

	day, ok := parseISODate(r.PathValue("date"))
	if !ok {
		http.Error(w, "Not found. Use a date in the form /YYYY-MM-DD/swap/N.", http.StatusNotFound)
		return
	}
	slot, err := strconv.Atoi(r.PathValue("slot"))
	if err != nil || slot < 0 || slot >= QuestionsPerDay {
		http.Error(w, "slot must be between 0 and "+strconv.Itoa(QuestionsPerDay-1), http.StatusBadRequest)
		return
	}

	index, err := WeekdayIndex(day)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, err := s.rotation.Swap(index, slot); err != nil {
		switch {
		case errors.Is(err, ErrNoAlternatives):
			http.Error(w, "no alternative question is available for this slot", http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	resolved := ResolveWeekday(day)
	http.Redirect(w, r, "/"+resolved.Format(isoLayout), http.StatusSeeOther)
}

// sameOrigin is a lightweight CSRF guard: there is no session or auth to rely
// on, so a swap is only accepted if the request's own Origin (or failing
// that, Referer) names this same host. Requests with neither header - not
// expected from a browser form post - are rejected too.
func sameOrigin(r *http.Request) bool {
	check := func(raw string) (bool, bool) {
		// "null" is a legitimate opaque origin (e.g. a sandboxed context), not
		// an absent header - but it can never match a real host, so treat it
		// as present-and-mismatched rather than falling through to Referer.
		if raw == "" {
			return false, false
		}
		u, err := url.Parse(raw)
		if err != nil {
			return false, true
		}
		return u.Host == r.Host, true
	}
	if ok, present := check(r.Header.Get("Origin")); present {
		return ok
	}
	if ok, present := check(r.Header.Get("Referer")); present {
		return ok
	}
	return false
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().In(s.loc)
	index, err := WeekdayIndex(now)

	status := http.StatusOK
	body := map[string]any{
		"status":    "ok",
		"questions": s.questions,
		"timezone":  s.loc.String(),
		"localTime": now.Format(time.RFC3339),
	}
	if err != nil {
		status = http.StatusServiceUnavailable
		body["status"] = "error"
		body["error"] = err.Error()
	} else {
		body["dayNumber"] = index + 1
		body["date"] = ResolveWeekday(now).Format(isoLayout)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// parseISODate accepts only a canonical YYYY-MM-DD date. Anything else, including
// a shape Go would otherwise tolerate such as 2026-1-5, is rejected.
func parseISODate(s string) (time.Time, bool) {
	t, err := time.Parse(isoLayout, s)
	if err != nil || t.Format(isoLayout) != s {
		return time.Time{}, false
	}
	return t, true
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func prevWeekday(d time.Time) time.Time {
	d = d.AddDate(0, 0, -1)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

func nextWeekday(d time.Time) time.Time {
	d = d.AddDate(0, 0, 1)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}
