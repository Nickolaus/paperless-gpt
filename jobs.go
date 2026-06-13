package main

import (
	"context"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

var (
	jobCancellersMu sync.Mutex
	jobCancellers   = make(map[string]context.CancelFunc)

	reOcrCancellersMu sync.Mutex
	reOcrCancellers   = make(map[string]context.CancelFunc)
)

// Job represents an OCR job
type Job struct {
	ID         string
	DocumentID int
	Status     string // "pending", "in_progress", "completed", "failed", "cancelled"
	Result     string // OCR result (combined text) or error message
	CreatedAt  time.Time
	UpdatedAt  time.Time
	PagesDone  int        // Number of pages processed
	TotalPages int        // Total number of pages in the document
	Options    OCROptions // OCR processing options
}

// JobStore manages jobs and their statuses
type JobStore struct {
	sync.RWMutex
	jobs map[string]*Job
}

var (
	logger = logrus.New()

	jobStore = &JobStore{
		jobs: make(map[string]*Job),
	}
	jobQueue = make(chan *Job, 100) // Buffered channel with capacity of 100 jobs
)

const (
	defaultJobRetentionSeconds = 24 * 60 * 60
	defaultMaxTerminalJobs     = 200
)

func init() {

	// Initialize logger
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	logger.SetLevel(logrus.InfoLevel)
	logger.WithField("prefix", "OCR_JOB")
}

func generateJobID() string {
	return uuid.New().String()
}

func (store *JobStore) addJob(job *Job) {
	store.Lock()
	defer store.Unlock()
	job.PagesDone = 0 // Initialize PagesDone to 0
	store.jobs[job.ID] = job
	store.pruneTerminalJobsLocked(time.Now(), ocrJobRetention(), ocrJobMaxTerminal())
	logger.Infof("Job added: %v", job)
}

func (store *JobStore) getJob(jobID string) (*Job, bool) {
	store.pruneTerminalJobs(time.Now(), ocrJobRetention(), ocrJobMaxTerminal())

	store.RLock()
	defer store.RUnlock()
	job, exists := store.jobs[jobID]
	return job, exists
}

func (store *JobStore) GetAllJobs() []*Job {
	store.pruneTerminalJobs(time.Now(), ocrJobRetention(), ocrJobMaxTerminal())

	store.RLock()
	defer store.RUnlock()

	jobs := make([]*Job, 0, len(store.jobs))
	for _, job := range store.jobs {
		jobs = append(jobs, job)
	}

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})

	return jobs
}

func (store *JobStore) pruneTerminalJobs(now time.Time, retention time.Duration, maxTerminalJobs int) {
	store.Lock()
	defer store.Unlock()
	store.pruneTerminalJobsLocked(now, retention, maxTerminalJobs)
}

func (store *JobStore) pruneTerminalJobsLocked(now time.Time, retention time.Duration, maxTerminalJobs int) {
	for id, job := range store.jobs {
		if isTerminalJobStatus(job.Status) && retention > 0 && now.Sub(jobRetentionTimestamp(job.CreatedAt, job.UpdatedAt)) > retention {
			delete(store.jobs, id)
		}
	}

	if maxTerminalJobs <= 0 {
		return
	}

	terminalJobs := make([]*Job, 0, len(store.jobs))
	for _, job := range store.jobs {
		if isTerminalJobStatus(job.Status) {
			terminalJobs = append(terminalJobs, job)
		}
	}

	if len(terminalJobs) <= maxTerminalJobs {
		return
	}

	sort.Slice(terminalJobs, func(i, j int) bool {
		return jobRetentionTimestamp(terminalJobs[i].CreatedAt, terminalJobs[i].UpdatedAt).Before(jobRetentionTimestamp(terminalJobs[j].CreatedAt, terminalJobs[j].UpdatedAt))
	})

	for _, job := range terminalJobs[:len(terminalJobs)-maxTerminalJobs] {
		delete(store.jobs, job.ID)
	}
}

func isTerminalJobStatus(status string) bool {
	return status == "completed" || status == "failed" || status == "cancelled"
}

func jobRetentionTimestamp(createdAt, updatedAt time.Time) time.Time {
	if !updatedAt.IsZero() {
		return updatedAt
	}
	return createdAt
}

func ocrJobRetention() time.Duration {
	return jobRetentionFromEnv("OCR_JOB_RETENTION_SECONDS")
}

func suggestionJobRetention() time.Duration {
	return jobRetentionFromEnv("SUGGESTION_JOB_RETENTION_SECONDS")
}

func jobRetentionFromEnv(envName string) time.Duration {
	value := os.Getenv(envName)
	if value == "" {
		return time.Duration(defaultJobRetentionSeconds) * time.Second
	}

	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return time.Duration(defaultJobRetentionSeconds) * time.Second
	}
	if seconds == 0 {
		return 0
	}

	return time.Duration(seconds) * time.Second
}

func ocrJobMaxTerminal() int {
	return maxTerminalJobsFromEnv("OCR_JOB_MAX_TERMINAL")
}

func suggestionJobMaxTerminal() int {
	return maxTerminalJobsFromEnv("SUGGESTION_JOB_MAX_TERMINAL")
}

func maxTerminalJobsFromEnv(envName string) int {
	value := os.Getenv(envName)
	if value == "" {
		return defaultMaxTerminalJobs
	}

	maxJobs, err := strconv.Atoi(value)
	if err != nil || maxJobs < 0 {
		return defaultMaxTerminalJobs
	}

	return maxJobs
}

func (store *JobStore) updateJobStatus(jobID, status, result string) {
	store.Lock()
	defer store.Unlock()
	if job, exists := store.jobs[jobID]; exists {
		job.Status = status
		if result != "" {
			job.Result = result
		}
		job.UpdatedAt = time.Now()
		logger.Infof("Job status updated: %v", job)
	}
}

func (store *JobStore) updatePagesDone(jobID string, pagesDone int) {
	store.Lock()
	defer store.Unlock()
	if job, exists := store.jobs[jobID]; exists {
		job.PagesDone = pagesDone
		job.UpdatedAt = time.Now()
		logger.Infof("Job pages done updated: %v", job)
	}
}

func startWorkerPool(app *App, numWorkers int) {
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			logger.Infof("Worker %d started", workerID)
			for job := range jobQueue {
				logger.Infof("Worker %d processing job: %s", workerID, job.ID)
				processJob(app, job)
			}
		}(i)
	}
}

func processJob(app *App, job *Job) {
	jobStore.updateJobStatus(job.ID, "in_progress", "")

	jobCtx, cancel := context.WithCancel(context.Background())
	jobCancellersMu.Lock()
	jobCancellers[job.ID] = cancel
	jobCancellersMu.Unlock()
	defer func() {
		cancel()
		jobCancellersMu.Lock()
		delete(jobCancellers, job.ID)
		jobCancellersMu.Unlock()
	}()

	// Delete old OCR page results for this document before starting new OCR
	if err := DeleteOcrPageResults(app.Database, job.DocumentID); err != nil {
		logger.Errorf("Failed to delete old OCR page results for document %d: %v", job.DocumentID, err)
		// Continue processing even if deletion fails
	}

	// Create OCR options from job options or app defaults
	options := job.Options
	if (options == OCROptions{}) {
		// Use app defaults if job options are not set
		options = OCROptions{
			UploadPDF:       app.pdfUpload,
			ReplaceOriginal: app.pdfReplace,
			CopyMetadata:    app.pdfCopyMetadata,
			LimitPages:      limitOcrPages,
		}
	}

	processedDoc, err := app.ProcessDocumentOCR(jobCtx, job.DocumentID, options, job.ID)
	if err != nil {
		if jobCtx.Err() == context.Canceled {
			jobStore.updateJobStatus(job.ID, "cancelled", "Job cancelled by user")
			logger.Infof("Job cancelled: %s", job.ID)
		} else {
			logger.Errorf("Error processing document OCR for job %s: %v", job.ID, err)
			jobStore.updateJobStatus(job.ID, "failed", err.Error())
		}
		return
	}
	if processedDoc == nil {
		logger.Infof("OCR processing skipped for job %s (document %d)", job.ID, job.DocumentID)
		jobStore.updateJobStatus(job.ID, "completed", "Skipped (already processed or other reason)")
		return
	}

	jobStore.updateJobStatus(job.ID, "completed", processedDoc.Text)
	logger.Infof("Job completed: %s", job.ID)
}
