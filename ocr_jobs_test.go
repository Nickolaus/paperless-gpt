package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetOCRJobTestState(t *testing.T) {
	t.Helper()

	oldStore := jobStore
	oldQueue := jobQueue
	oldCancellers := jobCancellers

	jobStore = &JobStore{jobs: make(map[string]*Job)}
	jobQueue = make(chan *Job, 100)
	jobCancellers = make(map[string]context.CancelFunc)

	t.Cleanup(func() {
		jobStore = oldStore
		jobQueue = oldQueue
		jobCancellers = oldCancellers
	})
}

func TestSubmitOCRJobHandlerFailsFastWhenQueueFull(t *testing.T) {
	resetOCRJobTestState(t)
	gin.SetMode(gin.TestMode)
	jobQueue = make(chan *Job, 1)
	jobQueue <- &Job{ID: "queued"}

	app := &App{}
	router := gin.Default()
	router.POST("/api/documents/:id/ocr", app.submitOCRJobHandler)

	req, err := http.NewRequest(http.MethodPost, "/api/documents/123/ocr", bytes.NewBuffer(nil))
	require.NoError(t, err)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	jobs := jobStore.GetAllJobs()
	require.Len(t, jobs, 1)
	assert.Equal(t, "failed", jobs[0].Status)
	assert.Equal(t, "OCR queue is full", jobs[0].Result)
}

func TestStopOCRJobHandlerCancelsPendingJob(t *testing.T) {
	resetOCRJobTestState(t)
	gin.SetMode(gin.TestMode)

	app := &App{}
	router := gin.Default()
	router.POST("/api/ocr/jobs/:job_id/stop", app.stopOCRJobHandler)

	jobStore.addJob(&Job{ID: "pending-ocr", Status: "pending"})

	req, err := http.NewRequest(http.MethodPost, "/api/ocr/jobs/pending-ocr/stop", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	job, exists := jobStore.getJob("pending-ocr")
	require.True(t, exists)
	assert.Equal(t, "cancelled", job.Status)
	assert.Equal(t, "Job cancelled by user", job.Result)
}

func TestGetOCRJobStatusReturnsSnapshot(t *testing.T) {
	resetOCRJobTestState(t)
	gin.SetMode(gin.TestMode)

	app := &App{}
	router := gin.Default()
	router.GET("/api/jobs/ocr/:job_id", app.getJobStatusHandler)

	jobStore.addJob(&Job{ID: "snapshot-ocr", Status: "pending", DocumentID: 123})
	snapshot, exists := jobStore.getJob("snapshot-ocr")
	require.True(t, exists)
	snapshot.Status = "completed"

	req, err := http.NewRequest(http.MethodGet, "/api/jobs/ocr/snapshot-ocr", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "pending", response["status"])
}

func TestOCRJobTimeoutFromEnv(t *testing.T) {
	t.Setenv("OCR_JOB_TIMEOUT_SECONDS", "42")
	assert.Equal(t, 42, int(ocrJobTimeout().Seconds()))

	t.Setenv("OCR_JOB_TIMEOUT_SECONDS", "0")
	assert.Zero(t, ocrJobTimeout())

	t.Setenv("OCR_JOB_TIMEOUT_SECONDS", "invalid")
	assert.Zero(t, ocrJobTimeout())
}
