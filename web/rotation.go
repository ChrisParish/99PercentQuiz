package main

import (
	"errors"
	"math"
	"sort"
	"sync"
	"time"
)

const (
	QuestionsPerDay = 20
	ExclusionDays   = 15 // a question may not reappear within this many weekdays

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
	ErrBeforeEpoch   = errors.New("date is before the quiz start date")
	ErrBeyondHorizon = errors.New("date is beyond the scheduling horizon")
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

type Rotation struct {
	questions []Question
	seed      uint64

	categories []string           // sorted, so iteration order is deterministic
	members    map[string][]int   // category -> question indices
	shares     map[string]float64 // category -> slots owed per day

	mu       sync.Mutex
	days     [][]int
	lastUsed []int              // question index -> day it last appeared, -1 for never
	credit   map[string]float64 // carries fractional entitlement between days
}

func NewRotation(questions []Question, seed uint64) *Rotation {
	last := make([]int, len(questions))
	for i := range last {
		last[i] = -1
	}

	members := make(map[string][]int)
	for i, q := range questions {
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

	return &Rotation{
		questions:  questions,
		seed:       seed,
		categories: categories,
		members:    members,
		shares:     shares,
		lastUsed:   last,
		credit:     make(map[string]float64, len(members)),
	}
}

// Day returns the question indices for weekday index i. Every preceding day is
// generated and cached first, because the exclusion window depends on history.
func (r *Rotation) Day(i int) []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.days) <= i {
		r.days = append(r.days, r.generate(len(r.days)))
	}
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
