package ocr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func setupTestServer() (*httptest.Server, func()) {
	origOCREndpoint := mistralOCREndpoint
	origFilesEndpoint := mistralFilesEndpoint

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/ocr" {
			handleOCRRequest(w, r)
		} else if r.URL.Path == "/v1/files" {
			handleFileUploadRequest(w, r)
		} else if r.URL.Path == "/v1/files/test-file-id/url" {
			handleGetSignedURLRequest(w, r)
		} else if r.URL.Path == "/v1/files/test-file-id" && r.Method == http.MethodDelete {
			handleDeleteFileRequest(w, r)
		}
	}))

	mistralOCREndpoint = server.URL + "/v1/ocr"
	mistralFilesEndpoint = server.URL + "/v1/files"

	return server, func() {
		server.Close()
		mistralOCREndpoint = origOCREndpoint
		mistralFilesEndpoint = origFilesEndpoint
	}
}

func TestMistralOCRProvider_RetriesTransientOCRFailure(t *testing.T) {
	origOCREndpoint := mistralOCREndpoint
	requests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, `{"error":"overloaded"}`)
			return
		}
		handleOCRRequest(w, r)
	}))
	defer func() {
		server.Close()
		mistralOCREndpoint = origOCREndpoint
	}()

	mistralOCREndpoint = server.URL + "/v1/ocr"
	provider := &MistralOCRProvider{
		apiKey:         "test-key",
		model:          "mistral-ocr-latest",
		maxRetries:     1,
		backoffMaxWait: time.Millisecond,
		requestTimeout: time.Second,
	}

	req := MistralOCRRequest{Model: provider.model}
	req.Document.Type = "document_url"
	req.Document.DocumentURL = "https://test-document-url"

	text, _, err := provider.processDocument(context.Background(), req, log.WithField("test", "retry"))

	assert.NoError(t, err)
	assert.Equal(t, "Test OCR output", text)
	assert.Equal(t, 2, requests)
}

func TestMistralOCRProvider_DoesNotRetryPermanentOCRFailure(t *testing.T) {
	origOCREndpoint := mistralOCREndpoint
	requests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"error":"bad request"}`)
	}))
	defer func() {
		server.Close()
		mistralOCREndpoint = origOCREndpoint
	}()

	mistralOCREndpoint = server.URL + "/v1/ocr"
	provider := &MistralOCRProvider{
		apiKey:         "test-key",
		model:          "mistral-ocr-latest",
		maxRetries:     3,
		backoffMaxWait: time.Millisecond,
		requestTimeout: time.Second,
	}

	req := MistralOCRRequest{Model: provider.model}
	req.Document.Type = "document_url"
	req.Document.DocumentURL = "https://test-document-url"

	_, _, err := provider.processDocument(context.Background(), req, log.WithField("test", "no_retry"))

	assert.Error(t, err)
	assert.Equal(t, 1, requests)
}

func handleOCRRequest(w http.ResponseWriter, r *http.Request) {
	resp := MistralOCRResponse{
		Pages: []struct {
			Index      int               `json:"index"`
			Markdown   string            `json:"markdown"`
			Images     []MistralOCRImage `json:"images"`
			Tables     []MistralOCRTable `json:"tables,omitempty"`
			Dimensions struct {
				Dpi    int `json:"dpi"`
				Height int `json:"height"`
				Width  int `json:"width"`
			} `json:"dimensions"`
		}{
			{
				Index:    0,
				Markdown: "Test OCR output",
				Images:   []MistralOCRImage{},
				Dimensions: struct {
					Dpi    int `json:"dpi"`
					Height int `json:"height"`
					Width  int `json:"width"`
				}{
					Dpi:    300,
					Height: 1000,
					Width:  800,
				},
			},
		},
		Model: "mistral-ocr-latest",
		UsageInfo: struct {
			PagesProcessed int         `json:"pages_processed"`
			DocSizeBytes   interface{} `json:"doc_size_bytes"`
		}{
			PagesProcessed: 1,
			DocSizeBytes:   1024,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func handleFileUploadRequest(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	if r.FormValue("purpose") != "ocr" {
		http.Error(w, "missing or invalid purpose", http.StatusBadRequest)
		return
	}

	resp := MistralFileUploadResponse{
		ID:       "test-file-id",
		Object:   "file",
		Filename: "document.pdf",
		Purpose:  "ocr",
	}
	json.NewEncoder(w).Encode(resp)
}

func handleGetSignedURLRequest(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		URL string `json:"url"`
	}{
		URL: "https://signed-url-for-file",
	}
	json.NewEncoder(w).Encode(resp)
}

func handleDeleteFileRequest(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Deleted bool   `json:"deleted"`
	}{
		ID:      "test-file-id",
		Object:  "file",
		Deleted: true,
	}
	json.NewEncoder(w).Encode(resp)
}

func TestNewMistralOCRProvider(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			config: Config{
				MistralAPIKey: "test-key",
			},
			wantErr: false,
		},
		{
			name: "valid config with custom model",
			config: Config{
				MistralAPIKey: "test-key",
				MistralModel:  "custom-model",
			},
			wantErr: false,
		},
		{
			name: "valid config with image reference mode",
			config: Config{
				MistralAPIKey:                "test-key",
				MistralOCRImageReferenceMode: "strip",
			},
			wantErr: false,
		},
		{
			name:        "missing API key",
			config:      Config{},
			wantErr:     true,
			errContains: "missing required Mistral API key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := newMistralOCRProvider(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				mistralProvider := provider.(*MistralOCRProvider)
				assert.Equal(t, tt.config.MistralAPIKey, mistralProvider.apiKey)
				if tt.config.MistralModel != "" {
					assert.Equal(t, tt.config.MistralModel, mistralProvider.model)
				} else {
					assert.Equal(t, "mistral-ocr-latest", mistralProvider.model)
				}
				if tt.config.MistralOCRImageReferenceMode != "" {
					assert.Equal(t, tt.config.MistralOCRImageReferenceMode, mistralProvider.imageReferenceMode)
				} else {
					assert.Equal(t, mistralOCRImageReferenceModePreserve, mistralProvider.imageReferenceMode)
				}
			}
		})
	}
}

func TestMistralOCRProvider_ProcessImage(t *testing.T) {
	_, cleanup := setupTestServer()
	defer cleanup()

	// Create provider with mocked API endpoint
	provider := &MistralOCRProvider{
		apiKey: "test-key",
		model:  "mistral-ocr-latest",
	}

	// Test image processing
	testImage := []byte("test image data")
	result, err := provider.ProcessImage(context.Background(), testImage, 1)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Test OCR output", result.Text)
	assert.Equal(t, "mistral_ocr", result.Metadata["provider"])
	assert.Equal(t, "mistral-ocr-latest", result.Metadata["model"])
}

func TestExpandMistralTableReferences(t *testing.T) {
	tests := []struct {
		name           string
		markdown       string
		tables         []MistralOCRTable
		wantContains   []string
		wantNotContain []string
		wantCount      int
	}{
		{
			name:     "replaces table markdown link with content",
			markdown: "Before\n\n[tbl-0.md](tbl-0.md)\n\nAfter",
			tables: []MistralOCRTable{{
				ID:      "tbl-0.md",
				Content: "| Item | Qty |\n| --- | --- |\n| Switch | 2 |",
				Format:  "markdown",
			}},
			wantContains:   []string{"Before", "| Switch | 2 |", "After"},
			wantNotContain: []string{"[tbl-0.md](tbl-0.md)"},
			wantCount:      1,
		},
		{
			name:     "replaces path table id with basename link",
			markdown: "Details\n\n[tbl-1.md](tbl-1.md)",
			tables: []MistralOCRTable{{
				ID:      "tables/tbl-1.md",
				Content: "| Net | Tax |\n| --- | --- |\n| 10 | 1.90 |",
				Format:  "markdown",
			}},
			wantContains:   []string{"| Net | Tax |", "| 10 | 1.90 |"},
			wantNotContain: []string{"[tbl-1.md](tbl-1.md)"},
			wantCount:      1,
		},
		{
			name:     "appends table content when no placeholder exists",
			markdown: "Body text",
			tables: []MistralOCRTable{{
				ID:      "tbl-2.md",
				Content: "| A | B |\n| --- | --- |\n| 1 | 2 |",
				Format:  "markdown",
			}},
			wantContains: []string{"Body text", "| A | B |", "| 1 | 2 |"},
			wantCount:    1,
		},
		{
			name:     "does not duplicate already inline table content",
			markdown: "Body text\n\n| A | B |\n| --- | --- |\n| 1 | 2 |",
			tables: []MistralOCRTable{{
				ID:      "tbl-2.md",
				Content: "| A | B |\n| --- | --- |\n| 1 | 2 |",
				Format:  "markdown",
			}},
			wantContains: []string{"Body text", "| A | B |", "| 1 | 2 |"},
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count := expandMistralTableReferences(tt.markdown, tt.tables)

			assert.Equal(t, tt.wantCount, count)
			for _, expected := range tt.wantContains {
				assert.Contains(t, got, expected)
			}
			for _, unexpected := range tt.wantNotContain {
				assert.NotContains(t, got, unexpected)
			}
		})
	}
}

func TestApplyMistralImageReferenceMode(t *testing.T) {
	tests := []struct {
		name      string
		markdown  string
		images    []MistralOCRImage
		mode      string
		want      string
		wantCount int
	}{
		{
			name:      "preserves image references by default",
			markdown:  "Intro\n\n![img-0.jpeg](img-0.jpeg)\n\nCaption",
			images:    []MistralOCRImage{{ID: "img-0.jpeg"}},
			mode:      "",
			want:      "Intro\n\n![img-0.jpeg](img-0.jpeg)\n\nCaption",
			wantCount: 0,
		},
		{
			name:      "strips returned image reference",
			markdown:  "Intro\n\n![img-0.jpeg](img-0.jpeg)\n\nCaption",
			images:    []MistralOCRImage{{ID: "img-0.jpeg"}},
			mode:      "strip",
			want:      "Intro\n\nCaption",
			wantCount: 1,
		},
		{
			name:      "strips path basename references",
			markdown:  "Intro\n\n![img-1.jpeg](./img-1.jpeg)\n\nCaption",
			images:    []MistralOCRImage{{ID: "images/img-1.jpeg"}},
			mode:      "strip",
			want:      "Intro\n\nCaption",
			wantCount: 1,
		},
		{
			name:      "does not strip unrelated image reference",
			markdown:  "Intro\n\n![logo.jpeg](logo.jpeg)\n\nCaption",
			images:    []MistralOCRImage{{ID: "img-0.jpeg"}},
			mode:      "strip",
			want:      "Intro\n\n![logo.jpeg](logo.jpeg)\n\nCaption",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count := applyMistralImageReferenceMode(tt.markdown, tt.images, tt.mode)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantCount, count)
		})
	}
}

func TestMistralOCRProvider_UploadFile(t *testing.T) {
	_, cleanup := setupTestServer()
	defer cleanup()

	// Create provider with mocked API endpoint
	provider := &MistralOCRProvider{
		apiKey: "test-key",
		model:  "mistral-ocr-latest",
	}

	// Test file upload
	testPDF := []byte("test pdf data")
	fileID, err := provider.uploadFile(context.Background(), testPDF)

	assert.NoError(t, err)
	assert.Equal(t, "test-file-id", fileID)
}

func TestMistralOCRProvider_GetSignedURL(t *testing.T) {
	_, cleanup := setupTestServer()
	defer cleanup()

	// Create provider with mocked API endpoint
	provider := &MistralOCRProvider{
		apiKey: "test-key",
		model:  "mistral-ocr-latest",
	}

	// Test getting signed URL
	url, err := provider.getSignedURL(context.Background(), "test-file-id")

	assert.NoError(t, err)
	assert.Equal(t, "https://signed-url-for-file", url)
}

func TestMistralOCRProvider_ProcessDocument(t *testing.T) {
	_, cleanup := setupTestServer()
	defer cleanup()

	// Create provider with mocked API endpoint
	provider := &MistralOCRProvider{
		apiKey: "test-key",
		model:  "mistral-ocr-latest",
	}

	req := MistralOCRRequest{
		Model: provider.model,
	}
	req.Document.Type = "document_url"
	req.Document.DocumentURL = "https://test-document-url"

	logger := log.WithField("test", "process_document")
	text, generationInfo, err := provider.processDocument(context.Background(), req, logger)

	assert.NoError(t, err)
	assert.Equal(t, "Test OCR output", text)
	assert.Equal(t, "mistral_ocr", generationInfo["provider"])
	assert.Equal(t, "mistral-ocr-latest", generationInfo["model"])
	assert.Equal(t, 1, generationInfo["pages_processed"])
}

func TestMistralOCRProvider_ProcessPDFDeletesUploadedFile(t *testing.T) {
	origOCREndpoint := mistralOCREndpoint
	origFilesEndpoint := mistralFilesEndpoint
	deleteRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/files" && r.Method == http.MethodPost:
			handleFileUploadRequest(w, r)
		case r.URL.Path == "/v1/files/test-file-id/url" && r.Method == http.MethodGet:
			handleGetSignedURLRequest(w, r)
		case r.URL.Path == "/v1/ocr" && r.Method == http.MethodPost:
			handleOCRRequest(w, r)
		case r.URL.Path == "/v1/files/test-file-id" && r.Method == http.MethodDelete:
			deleteRequests++
			handleDeleteFileRequest(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer func() {
		server.Close()
		mistralOCREndpoint = origOCREndpoint
		mistralFilesEndpoint = origFilesEndpoint
	}()

	mistralOCREndpoint = server.URL + "/v1/ocr"
	mistralFilesEndpoint = server.URL + "/v1/files"

	provider := &MistralOCRProvider{
		apiKey: "test-key",
		model:  "mistral-ocr-latest",
	}

	result, err := provider.ProcessImage(context.Background(), []byte("%PDF-1.7\ncontent"), 1)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, deleteRequests)
}

func TestMistralOCRProvider_ProcessPDFDeletesUploadedFileAfterOCRFailure(t *testing.T) {
	origOCREndpoint := mistralOCREndpoint
	origFilesEndpoint := mistralFilesEndpoint
	deleteRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/files" && r.Method == http.MethodPost:
			handleFileUploadRequest(w, r)
		case r.URL.Path == "/v1/files/test-file-id/url" && r.Method == http.MethodGet:
			handleGetSignedURLRequest(w, r)
		case r.URL.Path == "/v1/ocr" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, `{"error":"bad request"}`)
		case r.URL.Path == "/v1/files/test-file-id" && r.Method == http.MethodDelete:
			deleteRequests++
			handleDeleteFileRequest(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer func() {
		server.Close()
		mistralOCREndpoint = origOCREndpoint
		mistralFilesEndpoint = origFilesEndpoint
	}()

	mistralOCREndpoint = server.URL + "/v1/ocr"
	mistralFilesEndpoint = server.URL + "/v1/files"

	provider := &MistralOCRProvider{
		apiKey:         "test-key",
		model:          "mistral-ocr-latest",
		maxRetries:     0,
		backoffMaxWait: time.Millisecond,
		requestTimeout: time.Second,
	}

	result, err := provider.ProcessImage(context.Background(), []byte("%PDF-1.7\ncontent"), 1)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 1, deleteRequests)
}

func TestMistralOCRProvider_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		response    string
		wantErr     bool
		errContains string
	}{
		{
			name:        "unauthorized",
			statusCode:  401,
			response:    `{"error": "Invalid API key"}`,
			wantErr:     true,
			errContains: "OCR request failed with status: 401",
		},
		{
			name:        "bad request",
			statusCode:  400,
			response:    `{"error": "Invalid request"}`,
			wantErr:     true,
			errContains: "OCR request failed with status: 400",
		},
		{
			name:       "successful response",
			statusCode: 200,
			response:   `{"pages":[{"index":0,"markdown":"Test OCR output","images":[],"dimensions":{"dpi":300,"height":1000,"width":800}}],"model":"mistral-ocr-latest","usage_info":{"pages_processed":1,"doc_size_bytes":1024}}`,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origOCREndpoint := mistralOCREndpoint
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				fmt.Fprintln(w, tt.response)
			}))
			defer func() {
				server.Close()
				mistralOCREndpoint = origOCREndpoint
			}()

			provider := &MistralOCRProvider{
				apiKey: "test-key",
				model:  "mistral-ocr-latest",
			}
			mistralOCREndpoint = server.URL + "/v1/ocr"

			req := MistralOCRRequest{
				Model: provider.model,
			}
			req.Document.Type = "document_url"
			req.Document.DocumentURL = "https://test-document-url"

			logger := log.WithField("test", "error_handling")
			text, _, err := provider.processDocument(context.Background(), req, logger)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Empty(t, text)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, text)
			}
		})
	}
}
