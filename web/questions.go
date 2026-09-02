package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// data/pool.json is copied in from working/pool.json by the build scripts and
// the Dockerfile, since go:embed cannot reach outside the package directory.
// It carries the category each question belongs to, which the briefed
// questions.json deliberately does not.
//
//go:embed data/pool.json
var poolJSON []byte

type Question struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Category string `json:"category"`
	Reserve  bool   `json:"reserve"`
}

func loadQuestions() ([]Question, error) {
	// PowerShell writes UTF-8 with a BOM, which is not valid JSON.
	raw := bytes.TrimPrefix(poolJSON, []byte{0xEF, 0xBB, 0xBF})

	var all []Question
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("parsing pool.json: %w", err)
	}

	qs := make([]Question, 0, len(all))
	for _, q := range all {
		if !q.Reserve {
			qs = append(qs, q)
		}
	}

	// Below this the exclusion window could leave fewer than a full day's worth
	// of eligible questions, and the rotation would dead-end.
	min := QuestionsPerDay * (ExclusionDays + 1)
	if len(qs) < min {
		return nil, fmt.Errorf("need at least %d broadcast questions for a %d-weekday exclusion window, got %d",
			min, ExclusionDays, len(qs))
	}

	seen := make(map[string]int, len(qs))
	for i, q := range qs {
		if strings.TrimSpace(q.Question) == "" {
			return nil, fmt.Errorf("question %d has an empty question", i)
		}
		if strings.TrimSpace(q.Answer) == "" {
			return nil, fmt.Errorf("question %d (%q) has an empty answer", i, q.Question)
		}
		if strings.TrimSpace(q.Category) == "" {
			return nil, fmt.Errorf("question %d (%q) has no category", i, q.Question)
		}
		key := strings.ToLower(strings.Join(strings.Fields(q.Question), " "))
		if prev, dup := seen[key]; dup {
			return nil, fmt.Errorf("questions %d and %d are duplicates: %q", prev, i, q.Question)
		}
		seen[key] = i
	}
	return qs, nil
}
