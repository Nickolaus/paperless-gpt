package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/sirupsen/logrus"
)

var (
	mistralOCREndpoint   = "https://api.mistral.ai/v1/ocr"
	mistralFilesEndpoint = "https://api.mistral.ai/v1/files"
)

// MistralOCRProvider implements the OCR Provider interface using Mistral's OCR API
type MistralOCRProvider struct {
	apiKey                      string
	model                       string
	maxRetries                  int
	backoffMaxWait              time.Duration
	requestTimeout              time.Duration
	confidenceScoresGranularity string
	tableFormat                 string
}

// MistralOCRRequest represents the request body for the Mistral OCR API
type MistralOCRRequest struct {
	Model    string `json:"model"`
	Document struct {
		Type        string `json:"type"`
		DocumentURL string `json:"document_url,omitempty"`
		ImageURL    string `json:"image_url,omitempty"`
	} `json:"document"`
	IncludeImageBase64          bool   `json:"include_image_base64,omitempty"`
	ConfidenceScoresGranularity string `json:"confidence_scores_granularity,omitempty"`
	TableFormat                 string `json:"table_format,omitempty"`
}

type MistralOCRTable struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Format  string `json:"format"`
}

// MistralOCRResponse represents the response from Mistral's OCR API
type MistralOCRResponse struct {
	Pages []struct {
		Index      int               `json:"index"`
		Markdown   string            `json:"markdown"`
		Images     []interface{}     `json:"images"`
		Tables     []MistralOCRTable `json:"tables,omitempty"`
		Dimensions struct {
			Dpi    int `json:"dpi"`
			Height int `json:"height"`
			Width  int `json:"width"`
		} `json:"dimensions"`
	} `json:"pages"`
	Model     string `json:"model"`
	UsageInfo struct {
		PagesProcessed int         `json:"pages_processed"`
		DocSizeBytes   interface{} `json:"doc_size_bytes"`
	} `json:"usage_info"`
	ConfidenceScores interface{} `json:"confidence_scores,omitempty"`
}

// MistralFileUploadResponse represents the response from Mistral's file upload API
type MistralFileUploadResponse struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
	SizeBytes int64  `json:"size_bytes"`
	CreatedAt int64  `json:"created_at"`
}

// NewMistralOCRProvider creates a new instance of the Mistral OCR provider
func newMistralOCRProvider(config Config) (Provider, error) {
	if config.MistralAPIKey == "" {
		return nil, fmt.Errorf("missing required Mistral API key")
	}
	return &MistralOCRProvider{
		apiKey: config.MistralAPIKey,
		model: func() string {
			if config.MistralModel == "" {
				return "mistral-ocr-latest" // Default model
			}
			return config.MistralModel
		}(),
		maxRetries: func() int {
			if config.MistralOCRMaxRetries < 0 {
				return 0
			}
			if config.MistralOCRMaxRetries == 0 {
				return 3
			}
			return config.MistralOCRMaxRetries
		}(),
		backoffMaxWait: func() time.Duration {
			if config.MistralOCRBackoffMaxWait <= 0 {
				return 10 * time.Second
			}
			return config.MistralOCRBackoffMaxWait
		}(),
		requestTimeout: func() time.Duration {
			if config.MistralOCRRequestTimeout <= 0 {
				return 60 * time.Second
			}
			return config.MistralOCRRequestTimeout
		}(),
		confidenceScoresGranularity: config.MistralOCRConfidenceScoresGranularity,
		tableFormat:                 config.MistralOCRTableFormat,
	}, nil
}

func (p *MistralOCRProvider) retryableClient(timeout time.Duration) *retryablehttp.Client {
	if timeout <= 0 {
		timeout = p.requestTimeout
	}
	client := retryablehttp.NewClient()
	client.RetryMax = p.maxRetries
	client.RetryWaitMin = time.Second
	client.RetryWaitMax = p.backoffMaxWait
	client.Logger = nil
	client.HTTPClient.Timeout = timeout
	client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil {
			return true, nil
		}
		if resp == nil {
			return false, nil
		}
		switch resp.StatusCode {
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true, nil
		default:
			return false, nil
		}
	}
	return client
}

// ProcessImage implements the OCR Provider interface
func (p *MistralOCRProvider) ProcessImage(ctx context.Context, data []byte, pageNumber int) (*OCRResult, error) {
	logger := log.WithFields(logrus.Fields{
		"page_number": pageNumber,
		"data_size":   len(data),
		"provider":    "mistral_ocr",
		"model":       p.model,
	})

	logger.Info("Processing image with Mistral OCR provider")

	// Detect the actual MIME type of the data
	mtype := mimetype.Detect(data)
	logger.WithField("detected_mime_type", mtype.String()).Debug("Detected content type")

	var req MistralOCRRequest
	req.Model = p.model
	req.ConfidenceScoresGranularity = p.confidenceScoresGranularity
	req.TableFormat = p.tableFormat

	// Handle different content types appropriately
	if mtype.String() == "application/pdf" {
		logger.Debug("Processing PDF content via file upload method")
		// For PDF content, we need to upload the file first and use document_url
		fileID, err := p.uploadFile(ctx, data)
		if err != nil {
			logger.WithError(err).Error("Failed to upload PDF file")
			return nil, fmt.Errorf("failed to upload PDF file: %w", err)
		}
		defer p.cleanupUploadedFile(ctx, fileID, logger)

		// Get signed URL for the uploaded file
		signedURL, err := p.getSignedURL(ctx, fileID)
		if err != nil {
			logger.WithError(err).Error("Failed to get signed URL")
			return nil, fmt.Errorf("failed to get signed URL: %w", err)
		}

		req.Document.Type = "document_url"
		req.Document.DocumentURL = signedURL
		logger.WithField("file_id", fileID).Debug("Using document URL method")
	} else {
		logger.Debug("Processing image content via base64 method")
		// For image content, use base64 encoding
		base64Data := base64.StdEncoding.EncodeToString(data)

		// Use the detected MIME type for the data URL
		dataURL := fmt.Sprintf("data:%s;base64,%s", mtype.String(), base64Data)

		req.Document.Type = "image_url"
		req.Document.ImageURL = dataURL
		logger.WithFields(logrus.Fields{
			"mime_type":       mtype.String(),
			"base64_length":   len(base64Data),
			"data_url_prefix": dataURL[:min(50, len(dataURL))],
		}).Debug("Using image URL method")
	}

	text, generationInfo, err := p.processDocument(ctx, req, logger)
	if err != nil {
		return nil, err
	}

	return &OCRResult{
		Text: text,
		Metadata: map[string]string{
			"provider":  "mistral_ocr",
			"model":     p.model,
			"mime_type": mtype.String(),
			"page":      fmt.Sprintf("%d", pageNumber),
		},
		GenerationInfo: generationInfo,
	}, nil
}

// uploadFile uploads a file to Mistral's files API
func (p *MistralOCRProvider) uploadFile(ctx context.Context, data []byte) (string, error) {
	logger := log.WithField("data_size", len(data))
	logger.Debug("Uploading file to Mistral")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the file
	part, err := writer.CreateFormFile("file", "document.pdf")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		return "", err
	}

	// Add purpose field
	if err := writer.WriteField("purpose", "ocr"); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := retryablehttp.NewRequestWithContext(ctx, "POST", mistralFilesEndpoint, body)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	logger.WithFields(logrus.Fields{
		"url":          mistralFilesEndpoint,
		"content_type": writer.FormDataContentType(),
		"body_size":    body.Len(),
	}).Debug("Sending file upload request")

	client := p.retryableClient(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		logger.WithError(err).Error("File upload request failed")
		return "", err
	}
	defer resp.Body.Close()

	// Read response body for debugging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.WithError(err).Error("Failed to read upload response body")
		return "", err
	}

	logger.WithFields(logrus.Fields{
		"status_code": resp.StatusCode,
		"body_length": len(bodyBytes),
		"headers":     resp.Header,
	}).Debug("File upload response")

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"body_length": len(bodyBytes),
		}).Error("File upload failed")
		return "", fmt.Errorf("file upload failed with status: %d, response length: %d", resp.StatusCode, len(bodyBytes))
	}

	var uploadResp MistralFileUploadResponse
	if err := json.Unmarshal(bodyBytes, &uploadResp); err != nil {
		logger.WithError(err).Error("Failed to parse upload response")
		return "", err
	}

	logger.WithField("file_id", uploadResp.ID).Info("File uploaded successfully")
	return uploadResp.ID, nil
}

// getSignedURL gets a signed URL for an uploaded file
func (p *MistralOCRProvider) getSignedURL(ctx context.Context, fileID string) (string, error) {
	logger := log.WithField("file_id", fileID)
	logger.Debug("Getting signed URL")

	url := fmt.Sprintf("%s/%s/url?expiry=24", mistralFilesEndpoint, fileID)
	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "application/json")

	logger.WithField("url", url).Debug("Sending signed URL request")

	client := p.retryableClient(180 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		logger.WithError(err).Error("Signed URL request failed")
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.WithError(err).Error("Failed to read signed URL response")
		return "", err
	}

	logger.WithFields(logrus.Fields{
		"status_code": resp.StatusCode,
		"body_length": len(bodyBytes),
	}).Debug("Signed URL response")

	if resp.StatusCode != http.StatusOK {
		logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"body_length": len(bodyBytes),
		}).Error("Failed to get signed URL")
		return "", fmt.Errorf("failed to get signed URL with status: %d, response length: %d", resp.StatusCode, len(bodyBytes))
	}

	var signedURLResp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(bodyBytes, &signedURLResp); err != nil {
		logger.WithError(err).Error("Failed to parse signed URL response")
		return "", err
	}

	logger.Debug("Got signed URL successfully")
	return signedURLResp.URL, nil
}

func (p *MistralOCRProvider) cleanupUploadedFile(ctx context.Context, fileID string, logger *logrus.Entry) {
	if fileID == "" {
		return
	}

	cleanupCtx := context.WithoutCancel(ctx)
	cleanupCtx, cancel := context.WithTimeout(cleanupCtx, 30*time.Second)
	defer cancel()

	if err := p.deleteFile(cleanupCtx, fileID); err != nil {
		logger.WithError(err).WithField("file_id", fileID).Warn("Failed to delete uploaded Mistral file")
		return
	}

	logger.WithField("file_id", fileID).Debug("Deleted uploaded Mistral file")
}

func (p *MistralOCRProvider) deleteFile(ctx context.Context, fileID string) error {
	url := fmt.Sprintf("%s/%s", mistralFilesEndpoint, fileID)
	req, err := retryablehttp.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := p.retryableClient(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete file failed with status: %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	var deleteResp struct {
		Deleted bool   `json:"deleted"`
		ID      string `json:"id"`
	}
	if err := json.Unmarshal(bodyBytes, &deleteResp); err != nil {
		return err
	}
	if !deleteResp.Deleted {
		return fmt.Errorf("delete file response did not confirm deletion for file ID %s", fileID)
	}

	return nil
}

func expandMistralTableReferences(markdown string, tables []MistralOCRTable) (string, int) {
	expanded := markdown
	tableCount := 0

	for _, table := range tables {
		content := strings.TrimSpace(table.Content)
		if content == "" || strings.Contains(expanded, content) {
			continue
		}

		replaced := false
		for _, candidate := range tableReferenceCandidates(table.ID) {
			for _, placeholder := range []string{
				fmt.Sprintf("[%s](%s)", candidate, candidate),
				fmt.Sprintf("[%s](./%s)", candidate, candidate),
				fmt.Sprintf("![%s](%s)", candidate, candidate),
				fmt.Sprintf("![%s](./%s)", candidate, candidate),
			} {
				if strings.Contains(expanded, placeholder) {
					expanded = strings.ReplaceAll(expanded, placeholder, content)
					replaced = true
				}
			}
		}

		if !replaced {
			expanded = strings.TrimRight(expanded, "\n") + "\n\n" + content
		}
		tableCount++
	}

	return expanded, tableCount
}

func tableReferenceCandidates(id string) []string {
	normalized := strings.TrimSpace(id)
	if normalized == "" {
		return nil
	}

	candidates := []string{normalized}
	base := path.Base(normalized)
	if base != normalized {
		candidates = append(candidates, base)
	}
	withoutExt := strings.TrimSuffix(base, path.Ext(base))
	if withoutExt != "" && withoutExt != base {
		candidates = append(candidates, withoutExt)
	}
	if path.Ext(base) == "" {
		candidates = append(candidates, base+".md")
	}

	seen := make(map[string]struct{}, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		unique = append(unique, candidate)
	}

	return unique
}

// processDocument sends the OCR request to Mistral's API
func (p *MistralOCRProvider) processDocument(ctx context.Context, req MistralOCRRequest, logger *logrus.Entry) (string, map[string]interface{}, error) {
	logger.Debug("Processing document with Mistral OCR API")

	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", nil, err
	}

	// Log the request (but mask sensitive data)
	reqCopy := req
	if reqCopy.Document.ImageURL != "" && len(reqCopy.Document.ImageURL) > 100 {
		reqCopy.Document.ImageURL = reqCopy.Document.ImageURL[:100] + "... [truncated]"
	}
	if reqCopy.Document.DocumentURL != "" {
		reqCopy.Document.DocumentURL = "[redacted]"
	}
	reqLogData, _ := json.Marshal(reqCopy)
	logger.WithField("request_body", string(reqLogData)).Debug("OCR request details")

	httpReq, err := retryablehttp.NewRequestWithContext(ctx, "POST", mistralOCREndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	logger.WithFields(logrus.Fields{
		"url":         mistralOCREndpoint,
		"method":      "POST",
		"body_size":   len(jsonData),
		"api_key_len": len(p.apiKey),
	}).Debug("Sending OCR request")

	client := p.retryableClient(p.requestTimeout)
	resp, err := client.Do(httpReq)
	if err != nil {
		logger.WithError(err).Error("OCR request failed")
		return "", nil, err
	}
	defer resp.Body.Close()

	// Read the full response body for debugging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.WithError(err).Error("Failed to read OCR response body")
		return "", nil, err
	}

	logger.WithFields(logrus.Fields{
		"status_code": resp.StatusCode,
		"status":      resp.Status,
		"headers":     resp.Header,
		"body_length": len(bodyBytes),
	}).Debug("OCR response details")

	if resp.StatusCode != http.StatusOK {
		logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"body_length": len(bodyBytes),
			"headers":     resp.Header,
		}).Error("OCR request failed with detailed error info")
		return "", nil, fmt.Errorf("OCR request failed with status: %d, response length: %d", resp.StatusCode, len(bodyBytes))
	}

	var ocrResp MistralOCRResponse
	if err := json.Unmarshal(bodyBytes, &ocrResp); err != nil {
		logger.WithError(err).WithField("body_length", len(bodyBytes)).Error("Failed to parse OCR response")
		return "", nil, fmt.Errorf("failed to parse OCR response: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"pages_count":     len(ocrResp.Pages),
		"pages_processed": ocrResp.UsageInfo.PagesProcessed,
		"model":           ocrResp.Model,
	}).Info("OCR processing completed")

	// Combine text from all pages
	var combinedText string
	totalTablesExpanded := 0
	for i, page := range ocrResp.Pages {
		pageText, tablesExpanded := expandMistralTableReferences(page.Markdown, page.Tables)
		totalTablesExpanded += tablesExpanded
		logger.WithFields(logrus.Fields{
			"page_index":      i,
			"page_markdown":   len(page.Markdown),
			"page_tables":     len(page.Tables),
			"tables_expanded": tablesExpanded,
			"page_dpi":        page.Dimensions.Dpi,
			"page_width":      page.Dimensions.Width,
			"page_height":     page.Dimensions.Height,
		}).Debug("Processing page content")

		combinedText += pageText + "\n"
	}

	// Remove trailing newline
	if len(combinedText) > 0 {
		combinedText = combinedText[:len(combinedText)-1]
	}

	logger.WithField("combined_text_length", len(combinedText)).Info("Successfully extracted text")
	generationInfo := map[string]interface{}{
		"provider":        "mistral_ocr",
		"model":           ocrResp.Model,
		"pages_processed": ocrResp.UsageInfo.PagesProcessed,
		"doc_size_bytes":  ocrResp.UsageInfo.DocSizeBytes,
		"tables_expanded": totalTablesExpanded,
	}
	if ocrResp.ConfidenceScores != nil {
		generationInfo["confidence_scores_present"] = true
		if confidenceBytes, err := json.Marshal(ocrResp.ConfidenceScores); err == nil {
			generationInfo["confidence_scores_bytes"] = len(confidenceBytes)
		}
	}

	return combinedText, generationInfo, nil
}

// Helper function for min (Go 1.21+ has this built-in)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
