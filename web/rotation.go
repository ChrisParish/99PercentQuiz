package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	QuestionsPerDay = 15
	ExclusionDays   = 30 // a question may not reappear within this many weekdays

	// Each day draws at random from the oldest candidateFactor*QuestionsPerDay
	// eligible questions. Wide enough to keep groupings varied, narrow enough
	// that nothing goes stale.
	candidateFactor = 3

	// Weekdays per year, rounded up from 365/7*5.
	weekdaysPerYear = 261
	horizonYears    = 20
)

// Epoch must be a Monday. Weekday indices count forward from it.
var Epoch = time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)

// MaxDayIndex bounds schedule generation so a request for a far-future date
// cannot force an unbounded amount of work.
const MaxDayIndex = horizonYears * weekdaysPerYear

var (
	ErrBeforeEpoch    = errors.New("date is before the quiz start date")
	ErrBeyondHorizon  = errors.New("date is beyond the scheduling horizon")
	ErrInvalidSlot    = errors.New("slot is out of range")
	ErrNoAlternatives = errors.New("no alternative question is available for this slot")
)

// civilDay converts a calendar date to a day number, discarding clock time and
// zone so that arithmetic is unaffected by DST transitions.
func civilDay(t time.Time) int {
	y, m, d := t.Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

// ResolveWeekday moves Saturday and Sunday forward to the following Monday.
func ResolveWeekday(t time.Time) time.Time {
	y, m, d := t.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	switch day.Weekday() {
	case time.Saturday:
		return day.AddDate(0, 0, 2)
	case time.Sunday:
		return day.AddDate(0, 0, 1)
	default:
		return day
	}
}

// WeekdayIndex returns the number of whole weekdays from the Epoch to t,
// resolving weekends forward to the next Monday.
func WeekdayIndex(t time.Time) (int, error) {
	day := ResolveWeekday(t)
	delta := civilDay(day) - civilDay(Epoch)
	if delta < 0 {
		return 0, ErrBeforeEpoch
	}
	index := (delta/7)*5 + delta%7
	if index > MaxDayIndex {
		return 0, ErrBeyondHorizon
	}
	return index, nil
}

// persistedSchedule is the on-disk form of the rotation's mutable state: the
// questions actually shown (and swapped) on each day generated so far. Shared
// globally across every visitor, so a swap changes the day for everyone.
type persistedSchedule struct {
	Days []persistedDay `json:"days"`
}

type persistedDay struct {
	QuestionIDs []string `json:"questionIds"`
	// Questions already swapped away from this day. Kept so a rejected
	// question is not handed straight back on the next swap.
	DisplacedIDs []string `json:"displacedIds,omitempty"`
}

// loadSchedule reads the schedule file, tolerating a missing file (fresh
// start) but discarding anything it cannot cleanly map onto the current
// question pool rather than risk serving a corrupted half-state.
func loadSchedule(path string) *persistedSchedule {
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var sched persistedSchedule
	if err := json.Unmarshal(raw, &sched); err != nil {
		return nil
	}
	return &sched
}

// saveSchedule writes the schedule atomically (temp file + rename) so a crash
// mid-write cannot leave a truncated file behind.
func saveSchedule(path string, sched *persistedSchedule) error {
	if path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(sched, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type Rotation struct {
	questions []Question
	byID      map[string]int
	seed      uint64
	storePath string

	categories []string           // sorted, so iteration order is deterministic
	members    map[string][]int   // category -> question indices
	shares     map[string]float64 // category -> slots owed per day

	mu        sync.Mutex
	days      [][]int            // dayIndex -> chosen question indices (current, post-swap)
	displaced []map[int]bool     // dayIndex -> questions already rejected for that day
	lastUsed  []int              // question index -> day it last appeared, -1 for never
	credit    map[string]float64 // carries fractional entitlement between days
}

// NewRotation builds a rotation engine. storePath, if non-empty, is where the
// day-by-day schedule (including any swaps) is persisted so it survives a
// restart; pass "" for an in-memory-only rotation, as tests do.
func NewRotation(questions []Question, seed uint64, storePath string) *Rotation {
	last := make([]int, len(questions))
	for i := range last {
		last[i] = -1
	}

	byID := make(map[string]int, len(questions))
	members := make(map[string][]int)
	for i, q := range questions {
		byID[q.ID] = i
		members[q.Category] = append(members[q.Category], i)
	}

	categories := make([]string, 0, len(members))
	for c := range members {
		categories = append(categories, c)
	}
	sort.Strings(categories)

	shares := make(map[string]float64, len(members))
	for _, c := range categories {
		shares[c] = float64(len(members[c])) * QuestionsPerDay / float64(len(questions))
	}

	r := &Rotation{
		questions:  questions,
		byID:       byID,
		seed:       seed,
		storePath:  storePath,
		categories: categories,
		members:    members,
		shares:     shares,
		lastUsed:   last,
		credit:     make(map[string]float64, len(members)),
	}
	r.restore()
	return r
}

// restore replays a persisted schedule, if one exists and still maps cleanly
// onto the current question pool, so the server resumes exactly where it
// left off (including any swaps) rather than starting fresh after a restart.
func (r *Rotation) restore() {
	sched := loadSchedule(r.storePath)
	if sched == nil {
		return
	}
	days := make([][]int, 0, len(sched.Days))
	displaced := make([]map[int]bool, 0, len(sched.Days))
	for _, pd := range sched.Days {
		indices := make([]int, len(pd.QuestionIDs))
		for i, id := range pd.QuestionIDs {
			idx, ok := r.byID[id]
			if !ok {
				// The pool changed since this was written; it's not safe to
				// trust any of it, so start fresh instead of half-restoring.
				return
			}
			indices[i] = idx
		}
		rejected := make(map[int]bool, len(pd.DisplacedIDs))
		for _, id := range pd.DisplacedIDs {
			if idx, ok := r.byID[id]; ok {
				rejected[idx] = true
			}
		}
		days = append(days, indices)
		displaced = append(displaced, rejected)
	}
	r.days = days
	r.displaced = displaced
	r.rebuildLastUsed()
}

// rebuildLastUsed recomputes every question's most recent appearance from the
// schedule itself, so callers never have to patch the index by hand.
func (r *Rotation) rebuildLastUsed() {
	for i := range r.lastUsed {
		r.lastUsed[i] = -1
	}
	for day, indices := range r.days {
		for _, idx := range indices {
			if day > r.lastUsed[idx] {
				r.lastUsed[idx] = day
			}
		}
	}
}

// snapshotDays deep-copies the schedule so a failed write can be rolled back.
func (r *Rotation) snapshotDays() [][]int {
	out := make([][]int, len(r.days))
	for i, d := range r.days {
		out[i] = append([]int(nil), d...)
	}
	return out
}

// persist writes the full current schedule to disk. Called with mu held.
func (r *Rotation) persist() error {
	if r.storePath == "" {
		return nil
	}
	sched := &persistedSchedule{Days: make([]persistedDay, len(r.days))}
	for d, indices := range r.days {
		ids := make([]string, len(indices))
		for i, idx := range indices {
			ids[i] = r.questions[idx].ID
		}
		rejected := make([]string, 0, len(r.displaced[d]))
		for idx := range r.displaced[d] {
			rejected = append(rejected, r.questions[idx].ID)
		}
		sort.Strings(rejected) // map order would otherwise churn the file
		sched.Days[d] = persistedDay{QuestionIDs: ids, DisplacedIDs: rejected}
	}
	return saveSchedule(r.storePath, sched)
}

// extendTo ensures days up to and including i have been generated, persisting
// once at the end rather than after every individual day.
func (r *Rotation) extendTo(i int) {
	if len(r.days) > i {
		return
	}
	for len(r.days) <= i {
		r.days = append(r.days, r.generate(len(r.days)))
		r.displaced = append(r.displaced, make(map[int]bool))
	}
	// Generation is deterministic, so a lost write only costs a repeat of the
	// work next boot. A lost swap would silently discard a presenter's choice,
	// which is why Swap treats the same failure as fatal.
	if err := r.persist(); err != nil {
		log.Printf("warning: could not save the schedule to %s: %v", r.storePath, err)
	}
}

// Day returns the question indices for weekday index i. Every preceding day is
// generated and cached first, because the exclusion window depends on history.
func (r *Rotation) Day(i int) []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.extendTo(i)
	out := make([]int, len(r.days[i]))
	copy(out, r.days[i])
	return out
}

// Questions resolves a day index to the questions themselves.
func (r *Rotation) Questions(i int) []Question {
	indices := r.Day(i)
	out := make([]Question, len(indices))
	for n, idx := range indices {
		out[n] = r.questions[idx]
	}
	return out
}

// usedInRange collects every question scheduled on days lo..hi, clamped to the
// part of the schedule that has actually been generated.
func (r *Rotation) usedInRange(lo, hi int) map[int]bool {
	used := make(map[int]bool)
	if lo < 0 {
		lo = 0
	}
	if hi >= len(r.days) {
		hi = len(r.days) - 1
	}
	for d := lo; d <= hi; d++ {
		for _, idx := range r.days[d] {
			used[idx] = true
		}
	}
	return used
}

// windowUsed returns every question shown within ExclusionDays either side of
// center: the full no-repeat window a replacement should ideally respect.
func (r *Rotation) windowUsed(center int) map[int]bool {
	return r.usedInRange(center-ExclusionDays, center+ExclusionDays)
}

// pastUsed covers only the run-up to center. A 61-day window holds more slots
// than the pool holds questions, so once the schedule has been generated past
// center the strict window can leave nothing to choose from; days after center
// have not aired yet, so they are the safe half to relax.
func (r *Rotation) pastUsed(center int) map[int]bool {
	return r.usedInRange(center-ExclusionDays, center)
}

// Swap replaces the question in the given day/slot with a different eligible
// one. The change is global: every visitor sees it from then on. The question
// being replaced, and anything already swapped away from this day, are both
// excluded, so repeated presses keep moving forward instead of flip-flopping
// between two questions.
func (r *Rotation) Swap(dayIndex, slot int) (Question, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if slot < 0 || slot >= QuestionsPerDay {
		return Question{}, ErrInvalidSlot
	}
	r.extendTo(dayIndex)

	oldIdx := r.days[dayIndex][slot]
	category := r.questions[oldIdx].Category
	rejected := r.displaced[dayIndex]

	// Preference order, best first. Every candidate set already contains this
	// day, so the question being replaced and its 14 neighbours are excluded
	// automatically. Holding the day's category mix matters more than being
	// clean against days that have not aired, so a same-category pick outranks
	// a strictly-clean pick from another category.
	newIdx := -1
	for _, tier := range []struct {
		category string
		used     map[int]bool
	}{
		{category, r.windowUsed(dayIndex)},
		{category, r.pastUsed(dayIndex)},
		{"", r.windowUsed(dayIndex)},
		{"", r.pastUsed(dayIndex)},
	} {
		if newIdx = r.leastRecentlyUsed(tier.category, tier.used, rejected); newIdx >= 0 {
			break
		}
	}
	if newIdx < 0 {
		return Question{}, ErrNoAlternatives
	}

	snapshot := r.snapshotDays()
	r.days[dayIndex][slot] = newIdx
	rejected[oldIdx] = true
	r.clearLaterCopies(newIdx, dayIndex)
	r.rebuildLastUsed()

	if err := r.persist(); err != nil {
		// Roll back so what is being served matches what is on disk, rather
		// than reporting a swap that would vanish at the next restart.
		r.days = snapshot
		delete(rejected, oldIdx)
		r.rebuildLastUsed()
		return Question{}, fmt.Errorf("saving the swap failed: %w", err)
	}
	return r.questions[newIdx], nil
}

// leastRecentlyUsed picks the eligible question that has gone longest without
// airing, which both spreads the pool evenly and makes repeated swaps keep
// moving. Indexing into a freshly built candidate list was tried first and
// proved unstable, because the list shifts after every swap.
func (r *Rotation) leastRecentlyUsed(category string, used, rejected map[int]bool) int {
	pool := r.members[category]
	if category == "" {
		pool = make([]int, len(r.questions))
		for i := range pool {
			pool[i] = i
		}
	}
	best := -1
	for _, idx := range pool {
		if used[idx] || rejected[idx] {
			continue
		}
		if best < 0 || r.lastUsed[idx] < r.lastUsed[best] {
			best = idx
		}
	}
	return best
}

// clearLaterCopies re-draws any provisional later appearance of idx inside the
// exclusion window. Relaxing the forward check when choosing a replacement
// would otherwise let the same question air twice inside ExclusionDays.
func (r *Rotation) clearLaterCopies(idx, after int) {
	hi := after + ExclusionDays
	if hi >= len(r.days) {
		hi = len(r.days) - 1
	}
	for d := after + 1; d <= hi; d++ {
		for slot, got := range r.days[d] {
			if got != idx {
				continue
			}
			used := r.usedInRange(d-ExclusionDays, d+ExclusionDays)
			replacement := r.leastRecentlyUsed(r.questions[idx].Category, used, r.displaced[d])
			if replacement < 0 {
				replacement = r.leastRecentlyUsed("", used, r.displaced[d])
			}
			if replacement < 0 {
				return // nothing better available; a repeat beats a blank slot
			}
			r.days[d][slot] = replacement
		}
	}
}

// quotas splits the day's slots across categories in proportion to pool size,
// carrying the fraction forward so a category owed 2.5 a day alternates between
// 2 and 3 rather than always rounding the same way.
func (r *Rotation) quotas() map[string]int {
	quota := make(map[string]int, len(r.categories))
	total := 0
	for _, c := range r.categories {
		r.credit[c] += r.shares[c]
		n := int(math.Floor(r.credit[c] + 1e-9))
		if n < 0 {
			n = 0
		}
		quota[c] = n
		total += n
	}

	for total < QuestionsPerDay {
		pick, best := "", -math.MaxFloat64
		for _, c := range r.categories {
			if frac := r.credit[c] - float64(quota[c]); frac > best {
				pick, best = c, frac
			}
		}
		quota[pick]++
		total++
	}
	for total > QuestionsPerDay {
		pick, best := "", math.MaxFloat64
		for _, c := range r.categories {
			if quota[c] == 0 {
				continue
			}
			if frac := r.credit[c] - float64(quota[c]); frac < best {
				pick, best = c, frac
			}
		}
		quota[pick]--
		total--
	}

	for _, c := range r.categories {
		r.credit[c] -= float64(quota[c])
	}
	return quota
}

func (r *Rotation) generate(day int) []int {
	quota := r.quotas()

	eligible := make(map[string][]int, len(r.categories))
	for _, c := range r.categories {
		list := make([]int, 0, len(r.members[c]))
		for _, idx := range r.members[c] {
			if r.lastUsed[idx] < 0 || day-r.lastUsed[idx] > ExclusionDays {
				list = append(list, idx)
			}
		}
		sort.Slice(list, func(a, b int) bool {
			if r.lastUsed[list[a]] != r.lastUsed[list[b]] {
				return r.lastUsed[list[a]] < r.lastUsed[list[b]]
			}
			return list[a] < list[b]
		})
		eligible[c] = list
	}

	// A category can only supply what the exclusion window leaves it. Any
	// shortfall moves to a category with spare capacity, so balance gives way
	// before the no-repeat guarantee does.
	shortfall := 0
	for _, c := range r.categories {
		if len(eligible[c]) < quota[c] {
			shortfall += quota[c] - len(eligible[c])
			quota[c] = len(eligible[c])
		}
	}
	for shortfall > 0 {
		pick, best := "", 0
		for _, c := range r.categories {
			if spare := len(eligible[c]) - quota[c]; spare > best {
				pick, best = c, spare
			}
		}
		if pick == "" {
			break
		}
		quota[pick]++
		shortfall--
	}

	chosen := make([]int, 0, QuestionsPerDay)
	for _, c := range r.categories {
		n := quota[c]
		if n == 0 {
			continue
		}
		pool := eligible[c]
		window := candidateFactor * n
		if window > len(pool) {
			window = len(pool)
		}
		candidates := make([]int, window)
		copy(candidates, pool[:window])
		shuffle(candidates, newPRNG(mixSeed(r.seed^hashString(c), day)))
		chosen = append(chosen, candidates[:n]...)
	}

	// Without this the running order arrives grouped by category.
	shuffle(chosen, newPRNG(mixSeed(r.seed+0x5DEECE66D, day)))

	for _, idx := range chosen {
		r.lastUsed[idx] = day
	}
	return chosen
}

func shuffle(items []int, rng *prng) {
	for i := len(items) - 1; i > 0; i-- {
		j := rng.intn(i + 1)
		items[i], items[j] = items[j], items[i]
	}
}

func hashString(s string) uint64 {
	h := uint64(1469598103934665603)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// prng is splitmix64. Hand-rolled rather than math/rand so that a given seed
// yields the same schedule regardless of Go version.
type prng struct{ state uint64 }

func newPRNG(seed uint64) *prng { return &prng{state: seed} }

func (p *prng) next() uint64 {
	p.state += 0x9E3779B97F4A7C15
	z := p.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

func (p *prng) intn(n int) int { return int(p.next() % uint64(n)) }

func mixSeed(seed uint64, day int) uint64 {
	h := seed ^ (uint64(day)+0x9E3779B97F4A7C15)*0xBF58476D1CE4E5B9
	h ^= h >> 29
	h *= 0x94D049BB133111EB
	return h ^ (h >> 32)
}
