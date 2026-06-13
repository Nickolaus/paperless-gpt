package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestJobStorePrunesExpiredTerminalJobs(t *testing.T) {
	now := time.Now()
	store := &JobStore{jobs: map[string]*Job{
		"old-completed": {
			ID:        "old-completed",
			Status:    "completed",
			CreatedAt: now.Add(-3 * time.Hour),
			UpdatedAt: now.Add(-3 * time.Hour),
		},
		"recent-failed": {
			ID:        "recent-failed",
			Status:    "failed",
			CreatedAt: now.Add(-30 * time.Minute),
			UpdatedAt: now.Add(-30 * time.Minute),
		},
		"old-active": {
			ID:        "old-active",
			Status:    "in_progress",
			CreatedAt: now.Add(-3 * time.Hour),
			UpdatedAt: now.Add(-3 * time.Hour),
		},
	}}

	store.pruneTerminalJobs(now, time.Hour, 10)

	assert.NotContains(t, store.jobs, "old-completed")
	assert.Contains(t, store.jobs, "recent-failed")
	assert.Contains(t, store.jobs, "old-active")
}

func TestSuggestionJobStorePrunesOldestTerminalJobsOverLimit(t *testing.T) {
	now := time.Now()
	store := &SuggestionJobStore{jobs: map[string]*SuggestionJob{
		"oldest-completed": {
			ID:        "oldest-completed",
			Status:    "completed",
			CreatedAt: now.Add(-4 * time.Hour),
			UpdatedAt: now.Add(-4 * time.Hour),
		},
		"older-cancelled": {
			ID:        "older-cancelled",
			Status:    "cancelled",
			CreatedAt: now.Add(-3 * time.Hour),
			UpdatedAt: now.Add(-3 * time.Hour),
		},
		"newest-failed": {
			ID:        "newest-failed",
			Status:    "failed",
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-2 * time.Hour),
		},
		"active": {
			ID:        "active",
			Status:    "pending",
			CreatedAt: now.Add(-5 * time.Hour),
			UpdatedAt: now.Add(-5 * time.Hour),
		},
	}}

	store.pruneTerminalJobs(now, 24*time.Hour, 2)

	assert.NotContains(t, store.jobs, "oldest-completed")
	assert.Contains(t, store.jobs, "older-cancelled")
	assert.Contains(t, store.jobs, "newest-failed")
	assert.Contains(t, store.jobs, "active")
}
