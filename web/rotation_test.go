package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Mirrors the real category quotas (doubled) so the tests exercise the same arithmetic.
var testCategories = []struct {
	name  string
	count int
}{
	{"maths", 40}, {"animals", 100}, {"science", 90}, {"general", 90},
	{"geography", 80}, {"food", 80}, {"sports", 80}, {"music", 80},
	{"truefalse", 80}, {"body", 80},
}

func testPool() []Question {
	var qs []Question
	for _, c := range testCategories {
		for i := 0; i < c.count; i++ {
			qs = append(qs, Question{
				ID:       fmt.Sprintf("%s-%03d", c.name, i),
				Question: fmt.Sprintf("%s question %d?", c.name, i),
				Answer:   fmt.Sprintf("%s-%d", c.name, i),
				Category: c.name,
			})
		}
	}
	return qs
}

// Each day must reflect the pool's category mix, not just avoid repeats. A flat
// draw produced days with seven true/false questions and none at all on others.
func TestCategoryBalancePerDay(t *testing.T) {
	const days = 200
	qs := testPool()
	r := NewRotation(qs, 20260105, "")

	for d := 0; d < days; d++ {
		perDay := make(map[string]int, len(testCategories))
		for _, idx := range r.Day(d) {
			perDay[qs[idx].Category]++
		}
		for _, c := range testCategories {
			share := float64(c.count) * QuestionsPerDay / float64(len(qs))
			lo, hi := int(math.Floor(share)), int(math.Ceil(share))
			if got := perDay[c.name]; got < lo || got > hi {
				t.Fatalf("day %d: %s had %d questions, expected %d to %d", d, c.name, got, lo, hi)
			}
		}
	}
}

// Fractional entitlement must average out rather than always rounding the same way.
func TestCategoryShareHoldsOverTime(t *testing.T) {
	const days = 400
	qs := testPool()
	r := NewRotation(qs, 20260105, "")

	totals := make(map[string]int, len(testCategories))
	for d := 0; d < days; d++ {
		for _, idx := range r.Day(d) {
			totals[qs[idx].Category]++
		}
	}
	for _, c := range testCategories {
		want := float64(c.count) * QuestionsPerDay / float64(len(qs)) * days
		got := float64(totals[c.name])
		if math.Abs(got-want)/want > 0.02 {
			t.Errorf("%s: %.0f slots over %d days, expected about %.0f", c.name, got, days, want)
		}
	}
}

// The whole point of the app: a question must never come round again inside the
// exclusion window, no matter how far the schedule runs.
func TestNoRepeatWithinExclusionWindow(t *testing.T) {
	const days = 10000
	r := NewRotation(testPool(), 20260105, "")

	lastSeen := make(map[int]int)
	minGap := math.MaxInt
	for d := 0; d < days; d++ {
		for _, idx := range r.Day(d) {
			if prev, seen := lastSeen[idx]; seen {
				gap := d - prev
				if gap < minGap {
					minGap = gap
				}
				if gap <= ExclusionDays {
					t.Fatalf("question %d reappeared after %d weekdays (day %d then day %d); must exceed %d",
						idx, gap, prev, d, ExclusionDays)
				}
			}
			lastSeen[idx] = d
		}
	}
	t.Logf("minimum recurrence gap over %d weekdays: %d", days, minGap)
}

func TestEveryDayIsFullAndDistinct(t *testing.T) {
	r := NewRotation(testPool(), 20260105, "")
	for d := 0; d < 500; d++ {
		day := r.Day(d)
		if len(day) != QuestionsPerDay {
			t.Fatalf("day %d has %d questions, want %d", d, len(day), QuestionsPerDay)
		}
		seen := make(map[int]bool, len(day))
		for _, idx := range day {
			if seen[idx] {
				t.Fatalf("day %d contains question %d twice", d, idx)
			}
			seen[idx] = true
		}
	}
}

func TestFairness(t *testing.T) {
	const days = 2000
	qs := testPool()
	r := NewRotation(qs, 20260105, "")

	counts := make([]int, len(qs))
	for d := 0; d < days; d++ {
		for _, idx := range r.Day(d) {
			counts[idx]++
		}
	}

	mean := float64(days*QuestionsPerDay) / float64(len(qs))
	worst := 0.0
	for i, c := range counts {
		dev := math.Abs(float64(c)-mean) / mean
		if dev > worst {
			worst = dev
		}
		// Looser than it looks: categories cycle at slightly different rates, and
		// per-category share is asserted far more tightly elsewhere.
		if dev > 0.08 {
			t.Errorf("question %d used %d times against a mean of %.1f (%.1f%% off)", i, c, mean, dev*100)
		}
	}
	t.Logf("worst usage deviation over %d weekdays: %.2f%%", days, worst*100)
}

func TestDeterministicAcrossInstances(t *testing.T) {
	a := NewRotation(testPool(), 42, "")
	b := NewRotation(testPool(), 42, "")
	for d := 0; d < 200; d++ {
		if !reflect.DeepEqual(a.Day(d), b.Day(d)) {
			t.Fatalf("day %d differs between two rotations built with the same seed", d)
		}
	}
}

func TestSeedChangesSchedule(t *testing.T) {
	a := NewRotation(testPool(), 1, "")
	b := NewRotation(testPool(), 2, "")
	identical := true
	for d := 0; d < 50; d++ {
		if !reflect.DeepEqual(a.Day(d), b.Day(d)) {
			identical = false
			break
		}
	}
	if identical {
		t.Error("different seeds produced the same schedule")
	}
}

// Fixed 20-day blocks would satisfy the gap rule but repeat the same groupings
// forever, which presenters would notice.
func TestGroupingsVaryBetweenCycles(t *testing.T) {
	r := NewRotation(testPool(), 20260105, "")
	first := r.Day(0)
	for _, d := range []int{20, 40, 60, 100} {
		if reflect.DeepEqual(first, r.Day(d)) {
			t.Errorf("day %d repeats the exact grouping of day 0", d)
		}
	}
}

func TestWeekdayIndex(t *testing.T) {
	cases := []struct {
		date string
		want int
		note string
	}{
		{"2026-01-05", 0, "epoch Monday"},
		{"2026-01-06", 1, "Tuesday"},
		{"2026-01-09", 4, "Friday"},
		{"2026-01-10", 5, "Saturday resolves to the following Monday"},
		{"2026-01-11", 5, "Sunday resolves to the following Monday"},
		{"2026-01-12", 5, "that Monday"},
		{"2026-01-13", 6, "Tuesday of week two"},
	}
	for _, c := range cases {
		d, err := time.Parse("2006-01-02", c.date)
		if err != nil {
			t.Fatalf("bad test date %s: %v", c.date, err)
		}
		got, err := WeekdayIndex(d)
		if err != nil {
			t.Fatalf("%s (%s): %v", c.date, c.note, err)
		}
		if got != c.want {
			t.Errorf("%s (%s): got index %d, want %d", c.date, c.note, got, c.want)
		}
	}
}

// A weekend must land on the same set as the Monday it resolves to.
func TestWeekendMapsToNextMonday(t *testing.T) {
	sat, _ := time.Parse("2006-01-02", "2026-09-05")
	sun, _ := time.Parse("2006-01-02", "2026-09-06")
	mon, _ := time.Parse("2006-01-02", "2026-09-07")

	si, _ := WeekdayIndex(sat)
	ui, _ := WeekdayIndex(sun)
	mi, _ := WeekdayIndex(mon)

	if si != mi || ui != mi {
		t.Errorf("Sat=%d Sun=%d Mon=%d; all three should match", si, ui, mi)
	}
	if ResolveWeekday(sat).Weekday() != time.Monday {
		t.Error("Saturday did not resolve to a Monday")
	}
}

// Clock time and DST transitions must not affect which day you get.
func TestIndexIgnoresClockTimeAcrossDST(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("Europe/London unavailable - is time/tzdata imported? %v", err)
	}

	start := time.Date(2027, time.March, 20, 0, 0, 0, 0, london)
	end := time.Date(2027, time.November, 5, 0, 0, 0, 0, london)

	prev := -1
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		early := time.Date(d.Year(), d.Month(), d.Day(), 0, 30, 0, 0, london)
		late := time.Date(d.Year(), d.Month(), d.Day(), 23, 30, 0, 0, london)

		a, err := WeekdayIndex(early)
		if err != nil {
			t.Fatalf("%s: %v", d.Format("2006-01-02"), err)
		}
		b, err := WeekdayIndex(late)
		if err != nil {
			t.Fatalf("%s: %v", d.Format("2006-01-02"), err)
		}
		if a != b {
			t.Fatalf("%s: index differs by time of day (%d at 00:30, %d at 23:30)",
				d.Format("2006-01-02"), a, b)
		}

		switch d.Weekday() {
		case time.Saturday, time.Sunday:
			// Resolves forward, so no monotonic step to check.
		default:
			if prev >= 0 && a != prev+1 {
				t.Fatalf("%s: index jumped from %d to %d", d.Format("2006-01-02"), prev, a)
			}
			prev = a
		}
	}
}

func TestDateBounds(t *testing.T) {
	before, _ := time.Parse("2006-01-02", "2025-12-31")
	if _, err := WeekdayIndex(before); err != ErrBeforeEpoch {
		t.Errorf("date before the epoch: got %v, want ErrBeforeEpoch", err)
	}

	far, _ := time.Parse("2006-01-02", "2099-01-01")
	if _, err := WeekdayIndex(far); err != ErrBeyondHorizon {
		t.Errorf("date beyond the horizon: got %v, want ErrBeyondHorizon", err)
	}
}

func TestEmbeddedQuestionsLoad(t *testing.T) {
	qs, err := loadQuestions()
	if err != nil {
		t.Fatalf("embedded pool.json failed to load: %v", err)
	}
	min := QuestionsPerDay * (ExclusionDays + 1)
	if len(qs) < min {
		t.Fatalf("only %d broadcast questions embedded, need at least %d", len(qs), min)
	}

	byCategory := make(map[string]int)
	for _, q := range qs {
		byCategory[q.Category]++
	}
	if len(byCategory) < 2 {
		t.Fatalf("expected several categories, got %d", len(byCategory))
	}
	for category, n := range byCategory {
		// A category must cover what the exclusion window locks away plus today's
		// draw, or it will keep borrowing slots from its neighbours.
		share := float64(n) * QuestionsPerDay / float64(len(qs))
		needed := int(math.Ceil(share*ExclusionDays)) + int(math.Ceil(share))
		if n < needed {
			t.Errorf("category %q has %d questions but needs %d to sustain %.2f per day",
				category, n, needed, share)
		}
	}
	t.Logf("%d broadcast questions across %d categories", len(qs), len(byCategory))
}

// Swapping must change the slot, keep it in the same category, and never
// introduce a question already showing elsewhere within the exclusion window.
func TestSwapReplacesWithEligibleAlternative(t *testing.T) {
	qs := testPool()
	r := NewRotation(qs, 20260105, "")

	before := r.Day(10)
	oldIdx := before[3]

	got, err := r.Swap(10, 3)
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}
	after := r.Day(10)
	if after[3] == oldIdx {
		t.Fatalf("slot 3 did not change after swap")
	}
	if got.Category != qs[oldIdx].Category {
		t.Errorf("swapped-in question category %q, want %q", got.Category, qs[oldIdx].Category)
	}

	seen := make(map[int]bool, len(after))
	for _, idx := range after {
		if seen[idx] {
			t.Fatalf("day 10 contains a duplicate question after swap")
		}
		seen[idx] = true
	}

	used := r.windowUsed(10)
	delete(used, after[3])
	if used[after[3]] {
		t.Errorf("swapped-in question was already used elsewhere in the exclusion window")
	}

	// Days 0-9 all exist and all sit inside the exclusion window for day 10.
	for d := 0; d < 10; d++ {
		for _, idx := range r.Day(d) {
			if idx == after[3] {
				t.Errorf("swapped-in question also airs on day %d, inside the exclusion window", d)
			}
		}
	}
}

// The button must never hand back the question it was asked to replace, nor
// any it has already been through: a swap that lands on something previously
// seen reads as a broken button.
func TestSwapKeepsMovingForward(t *testing.T) {
	qs := testPool()
	r := NewRotation(qs, 20260105, "")

	seen := map[string]bool{qs[r.Day(7)[0]].ID: true}
	for i := 1; i <= 20; i++ {
		got, err := r.Swap(7, 0)
		if err != nil {
			t.Fatalf("swap %d: %v", i, err)
		}
		if seen[got.ID] {
			t.Fatalf("swap %d went back to %s, which this slot has already shown", i, got.ID)
		}
		if served := qs[r.Day(7)[0]].ID; served != got.ID {
			t.Fatalf("swap %d reported %s but the day serves %s", i, got.ID, served)
		}
		seen[got.ID] = true
	}
}

// Browsing to a far-future date generates every day in between, which fills
// the forward half of the exclusion window. That used to leave no eligible
// candidate at all, so swapping an earlier day failed outright.
func TestSwapWorksWhenTheScheduleRunsFarAhead(t *testing.T) {
	qs := testPool()
	r := NewRotation(qs, 20260105, "")
	r.Day(200)

	const day = 100
	previous := qs[r.Day(day)[0]].ID
	for i := 1; i <= 10; i++ {
		got, err := r.Swap(day, 0)
		if err != nil {
			t.Fatalf("swap %d failed on a fully generated schedule: %v", i, err)
		}
		if got.ID == previous {
			t.Fatalf("swap %d returned %s, the question it was meant to replace", i, got.ID)
		}
		previous = got.ID
	}
}

// Relaxing the forward check must not leave the same question airing twice
// inside the window; the later, not-yet-aired copy gives way instead.
func TestSwapDoesNotCreateARepeatInsideTheWindow(t *testing.T) {
	r := NewRotation(testPool(), 20260105, "")
	r.Day(200)

	const day = 100
	for i := 0; i < 10; i++ {
		if _, err := r.Swap(day, i%QuestionsPerDay); err != nil {
			t.Fatalf("swap %d: %v", i, err)
		}
	}

	for _, idx := range r.Day(day) {
		for d := day - ExclusionDays; d <= day+ExclusionDays; d++ {
			if d < 0 || d == day {
				continue
			}
			for _, other := range r.Day(d) {
				if other == idx {
					t.Fatalf("question %d airs on both day %d and day %d", idx, day, d)
				}
			}
		}
	}
}

// A swap that cannot be written must not be reported as successful, or it
// silently disappears at the next restart.
func TestSwapRollsBackWhenItCannotPersist(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("a file, not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRotation(testPool(), 20260105, filepath.Join(blocker, "schedule.json"))

	before := r.Day(2)[0]
	if _, err := r.Swap(2, 0); err == nil {
		t.Fatal("swap reported success even though the schedule could not be written")
	}
	if after := r.Day(2)[0]; after != before {
		t.Errorf("slot 0 changed to %d after a failed write, want it left at %d", after, before)
	}
}

// Repeated swaps on the same slot must cycle through distinct alternatives
// rather than repeating the same one or bouncing between just two.
func TestSwapCyclesThroughAlternatives(t *testing.T) {
	r := NewRotation(testPool(), 20260105, "")
	r.Day(5)

	seenQuestions := map[string]bool{}
	for i := 0; i < 5; i++ {
		got, err := r.Swap(5, 0)
		if err != nil {
			t.Fatalf("swap %d: %v", i, err)
		}
		seenQuestions[got.Question] = true
	}
	if len(seenQuestions) < 4 {
		t.Errorf("5 swaps produced only %d distinct questions, want at least 4", len(seenQuestions))
	}
}

func TestSwapRejectsInvalidSlot(t *testing.T) {
	r := NewRotation(testPool(), 20260105, "")
	if _, err := r.Swap(0, -1); !errors.Is(err, ErrInvalidSlot) {
		t.Errorf("slot -1: got %v, want ErrInvalidSlot", err)
	}
	if _, err := r.Swap(0, QuestionsPerDay); !errors.Is(err, ErrInvalidSlot) {
		t.Errorf("slot %d: got %v, want ErrInvalidSlot", QuestionsPerDay, err)
	}
}

// A swap must persist to disk and be visible to a brand new Rotation instance
// pointed at the same file, simulating a server restart.
func TestSwapPersistsAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedule.json")
	qs := testPool()

	a := NewRotation(qs, 20260105, path)
	a.Day(3)
	swapped, err := a.Swap(3, 2)
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}

	b := NewRotation(qs, 20260105, path)
	restored := b.Questions(3)
	if restored[2].ID != swapped.ID {
		t.Fatalf("after restart, slot 2 on day 3 is %q, want the swapped question %q",
			restored[2].ID, swapped.ID)
	}

	// The rest of day 3 must also match exactly, not just the swapped slot.
	original := a.Questions(3)
	for i := range original {
		if original[i].ID != restored[i].ID {
			t.Errorf("slot %d: got %q after restart, want %q", i, restored[i].ID, original[i].ID)
		}
	}
}

// A schedule file that no longer matches the question pool (e.g. after
// content changes) must be discarded rather than half-applied.
func TestStaleScheduleFileIsDiscarded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedule.json")
	if err := saveSchedule(path, &persistedSchedule{
		Days: []persistedDay{{QuestionIDs: []string{"no-such-id"}}},
	}); err != nil {
		t.Fatalf("saveSchedule: %v", err)
	}

	r := NewRotation(testPool(), 20260105, path)
	day := r.Day(0)
	if len(day) != QuestionsPerDay {
		t.Fatalf("got %d questions for day 0, want %d (stale file should have been ignored)",
			len(day), QuestionsPerDay)
	}
}
