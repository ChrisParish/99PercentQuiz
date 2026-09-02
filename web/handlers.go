package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
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
	mux.HandleFunc("GET /{date}", s.handleDate)
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy",
			"default-src 'none'; style-src 'self'; script-src 'self'; img-src 'self' data:; "+
				"base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
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
