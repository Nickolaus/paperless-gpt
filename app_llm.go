package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	_ "image/jpeg"

	"github.com/sirupsen/logrus"
	"github.com/tmc/langchaingo/llms"
)

func formatTagTaxonomy(tags []Tag) string {
	if len(tags) == 0 {
		return ""
	}

	tagsByID := make(map[int]Tag, len(tags))
	childrenByParentID := make(map[int][]Tag)
	roots := make([]Tag, 0, len(tags))

	for _, tag := range tags {
		tagsByID[tag.ID] = tag
	}

	for _, tag := range tags {
		if tag.ParentID != nil {
			if _, exists := tagsByID[*tag.ParentID]; exists {
				childrenByParentID[*tag.ParentID] = append(childrenByParentID[*tag.ParentID], tag)
				continue
			}
		}
		roots = append(roots, tag)
	}

	sortTagsByName(roots)
	for parentID := range childrenByParentID {
		sortTagsByName(childrenByParentID[parentID])
	}

	lines := []string{}
	visited := map[int]bool{}
	var appendTag func(tag Tag, depth int)
	appendTag = func(tag Tag, depth int) {
		if visited[tag.ID] {
			return
		}
		visited[tag.ID] = true

		lines = append(lines, fmt.Sprintf("%s- %s", strings.Repeat("  ", depth), tag.Name))
		for _, child := range childrenByParentID[tag.ID] {
			appendTag(child, depth+1)
		}
	}

	for _, root := range roots {
		appendTag(root, 0)
	}

	return strings.Join(lines, "\n")
}

func sortTagsByName(tags []Tag) {
	slices.SortFunc(tags, func(left, right Tag) int {
		return strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
	})
}

func cleanLLMScalar(value string) string {
	value = stripMarkdown(stripReasoning(value))
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'`")
	return strings.TrimSpace(value)
}

func filterSuggestedTags(suggestedTags []string, originalTags []string, availableTags []string, allowNewTags bool) []string {
	filteredTags, _ := filterSuggestedTagsWithParents(suggestedTags, originalTags, availableTags, nil, allowNewTags)
	return filteredTags
}

func filterSuggestedRemoveTags(suggestedRemoveTags []string, originalTags []string) []string {
	originalByName := map[string]string{}
	for _, tag := range originalTags {
		tag = cleanLLMScalar(tag)
		if tag != "" {
			originalByName[strings.ToLower(tag)] = tag
		}
	}

	removedTags := []string{}
	for _, tag := range suggestedRemoveTags {
		tag = cleanLLMScalar(tag)
		if tag == "" {
			continue
		}
		if originalTag, exists := originalByName[strings.ToLower(tag)]; exists {
			removedTags = appendUniqueStrings(removedTags, originalTag)
		}
	}
	return removedTags
}

func appendUniqueStrings(values []string, additions ...string) []string {
	for _, addition := range additions {
		addition = cleanLLMScalar(addition)
		if addition == "" {
			continue
		}
		exists := false
		for _, value := range values {
			if strings.EqualFold(value, addition) {
				exists = true
				break
			}
		}
		if !exists {
			values = append(values, addition)
		}
	}
	return values
}

func filterSuggestedTagsWithParents(suggestedTags []string, originalTags []string, availableTags []string, detailedTags []DetailedTag, allowNewTags bool) ([]string, map[string]int) {
	filteredTags := []string{}
	addTagParents := map[string]int{}
	originalTagNames := tagNameSet(originalTags)
	availableByName := map[string]string{}
	for _, availableTag := range availableTags {
		availableByName[strings.ToLower(availableTag)] = availableTag
	}
	detailedByName := map[string]DetailedTag{}
	detailedByPath := map[string]DetailedTag{}
	parentCandidatesByPath := map[string]DetailedTag{}
	for _, tag := range detailedTags {
		detailedByName[strings.ToLower(tag.Name)] = tag
		if tag.Path != "" {
			detailedByPath[strings.ToLower(tag.Path)] = tag
		}
		if tag.IsParentCandidate {
			parentCandidatesByPath[strings.ToLower(tag.Path)] = tag
			parentCandidatesByPath[strings.ToLower(tag.Name)] = tag
		}
	}
	appendTag := func(tagName string) {
		if !tagNameSet(filteredTags)[strings.ToLower(tagName)] {
			filteredTags = append(filteredTags, tagName)
		}
	}

	for _, tag := range suggestedTags {
		tag = cleanLLMScalar(tag)
		if tag == "" {
			continue
		}

		if availableTag, exists := availableByName[strings.ToLower(tag)]; exists {
			appendTag(availableTag)
			continue
		}

		if detailedTag, exists := detailedByPath[strings.ToLower(tag)]; exists {
			if availableTag, available := availableByName[strings.ToLower(detailedTag.Name)]; available {
				appendTag(availableTag)
			}
			continue
		}

		if childName, parentID, ok := parseSuggestedChildTagPath(tag, parentCandidatesByPath, detailedByName); ok {
			if availableTag, exists := availableByName[strings.ToLower(childName)]; exists {
				appendTag(availableTag)
				continue
			}
			if allowNewTags {
				appendTag(childName)
				addTagParents[childName] = parentID
			}
			continue
		}

		if allowNewTags && currentTagSelectionMode() != tagSelectionModeApplicable {
			appendTag(tag)
		}
	}
	for _, tag := range originalTags {
		tag = cleanLLMScalar(tag)
		if tag != "" && originalTagNames[strings.ToLower(tag)] {
			appendTag(tag)
		}
	}

	slices.Sort(filteredTags)
	if len(addTagParents) == 0 {
		return slices.Compact(filteredTags), nil
	}
	return slices.Compact(filteredTags), addTagParents
}

func parseSuggestedChildTagPath(tagPath string, parentCandidatesByPath map[string]DetailedTag, detailedByName map[string]DetailedTag) (string, int, bool) {
	parts := strings.Split(tagPath, "/")
	if len(parts) < 2 {
		return "", 0, false
	}
	cleanedParts := make([]string, 0, len(parts))
	for _, part := range parts {
		part = cleanLLMScalar(part)
		if part != "" {
			cleanedParts = append(cleanedParts, part)
		}
	}
	if len(cleanedParts) < 2 {
		return "", 0, false
	}

	childName := cleanedParts[len(cleanedParts)-1]
	parentPath := strings.Join(cleanedParts[:len(cleanedParts)-1], " / ")
	parent, exists := parentCandidatesByPath[strings.ToLower(parentPath)]
	if !exists {
		return "", 0, false
	}
	if existingTag, exists := detailedByName[strings.ToLower(childName)]; exists && existingTag.ParentID != nil && *existingTag.ParentID == parent.ID {
		return existingTag.Name, parent.ID, true
	}
	return childName, parent.ID, true
}

func validateSuggestedCorrespondent(suggestion string, availableCorrespondents []string, blacklist []string) string {
	suggestion = cleanLLMScalar(suggestion)
	if suggestion == "" || strings.EqualFold(suggestion, "unknown") {
		return ""
	}
	for _, blacklisted := range blacklist {
		if strings.EqualFold(suggestion, blacklisted) {
			return ""
		}
	}
	for _, availableCorrespondent := range availableCorrespondents {
		if strings.EqualFold(suggestion, availableCorrespondent) {
			return availableCorrespondent
		}
	}
	return suggestion
}

func isLikelyNarrowDocumentType(suggestion string) bool {
	normalized := strings.ToLower(cleanLLMScalar(suggestion))
	tokens := strings.FieldsFunc(normalized, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '/' || r == ':' || r == ';' || r == ',' || r == '.'
	})
	narrowTerms := []string{
		"liste",
		"übersicht",
		"uebersicht",
		"aufstellung",
		"zusammenfassung",
		"summary",
		"overview",
		"list",
	}
	for _, term := range narrowTerms {
		if normalized == term {
			return true
		}
		for _, token := range tokens {
			if token == term {
				return true
			}
		}
	}
	return false
}

func validateSuggestedDocumentType(suggestion string, availableDocumentTypes []string, allowNewDocumentTypes bool, logger *logrus.Entry) string {
	suggestion = cleanLLMScalar(suggestion)
	if suggestion == "" {
		return ""
	}

	for _, docType := range availableDocumentTypes {
		if strings.EqualFold(suggestion, docType) {
			return docType
		}
	}

	if allowNewDocumentTypes && !isLikelyNarrowDocumentType(suggestion) {
		return suggestion
	}

	if logger != nil {
		logger.Warnf("LLM suggested document type '%s' not accepted as a reusable document type", suggestion)
	}
	return ""
}

// getSuggestedCorrespondent generates a suggested correspondent for a document using the LLM
func (app *App) getSuggestedCorrespondent(ctx context.Context, content string, suggestedTitle string, availableCorrespondents []string, correspondentBlackList []string) (string, error) {
	likelyLanguage := getLikelyLanguage()

	templateMutex.RLock()
	defer templateMutex.RUnlock()

	// Get available tokens for content
	templateData := map[string]interface{}{
		"Language":                likelyLanguage,
		"AvailableCorrespondents": availableCorrespondents,
		"BlackList":               correspondentBlackList,
		"Title":                   suggestedTitle,
	}

	availableTokens, err := getAvailableTokensForContent(correspondentTemplate, templateData)
	if err != nil {
		return "", fmt.Errorf("error calculating available tokens: %v", err)
	}

	// Truncate content if needed
	truncatedContent, err := truncateContentByTokens(content, availableTokens)
	if err != nil {
		return "", fmt.Errorf("error truncating content: %v", err)
	}

	// Execute template with truncated content
	var promptBuffer bytes.Buffer
	templateData["Content"] = truncatedContent
	err = correspondentTemplate.Execute(&promptBuffer, templateData)
	if err != nil {
		return "", fmt.Errorf("error executing correspondent template: %v", err)
	}

	prompt := promptBuffer.String()
	log.WithField("prompt_length", len(prompt)).Debug("Correspondent suggestion prompt prepared")

	completion, err := app.LLM.GenerateContent(ctx, []llms.MessageContent{
		{
			Parts: []llms.ContentPart{
				llms.TextContent{
					Text: prompt,
				},
			},
			Role: llms.ChatMessageTypeHuman,
		},
	})
	if err != nil {
		return "", fmt.Errorf("error getting response from LLM: %v", err)
	}

	response := stripReasoning(strings.TrimSpace(completion.Choices[0].Content))
	return response, nil
}

// getSuggestedTags generates suggested tags for a document using the LLM
func (app *App) getSuggestedTags(
	ctx context.Context,
	content string,
	suggestedTitle string,
	availableTags []string,
	availableTagContext string,
	originalTags []string,
	logger *logrus.Entry) ([]string, error) {
	likelyLanguage := getLikelyLanguage()

	templateMutex.RLock()
	defer templateMutex.RUnlock()

	// Remove all paperless-gpt related tags from available tags
	availableTags = removeTagFromList(availableTags, manualTag)
	availableTags = removeTagFromList(availableTags, autoTag)
	availableTags = removeTagFromList(availableTags, autoOcrTag)

	// Get available tokens for content
	templateData := map[string]interface{}{
		"Language":            likelyLanguage,
		"AvailableTags":       availableTags,
		"AvailableTagContext": availableTagContext,
		"OriginalTags":        originalTags,
		"Title":               suggestedTitle,
		"CreateNewTags":       createNewTags,
	}

	availableTokens, err := getAvailableTokensForContent(tagTemplate, templateData)
	if err != nil {
		logger.Errorf("Error calculating available tokens: %v", err)
		return nil, fmt.Errorf("error calculating available tokens: %v", err)
	}

	// Truncate content if needed
	truncatedContent, err := truncateContentByTokens(content, availableTokens)
	if err != nil {
		logger.Errorf("Error truncating content: %v", err)
		return nil, fmt.Errorf("error truncating content: %v", err)
	}

	// Execute template with truncated content
	var promptBuffer bytes.Buffer
	templateData["Content"] = truncatedContent
	err = tagTemplate.Execute(&promptBuffer, templateData)
	if err != nil {
		logger.Errorf("Error executing tag template: %v", err)
		return nil, fmt.Errorf("error executing tag template: %v", err)
	}

	prompt := promptBuffer.String()
	logger.WithField("prompt_length", len(prompt)).Debug("Tag suggestion prompt prepared")

	completion, err := app.LLM.GenerateContent(ctx, []llms.MessageContent{
		{
			Parts: []llms.ContentPart{
				llms.TextContent{
					Text: prompt,
				},
			},
			Role: llms.ChatMessageTypeHuman,
		},
	})
	if err != nil {
		logger.Errorf("Error getting response from LLM: %v", err)
		return nil, fmt.Errorf("error getting response from LLM: %v", err)
	}

	response := stripReasoning(completion.Choices[0].Content)

	suggestedTags := strings.Split(response, ",")
	return filterSuggestedTags(suggestedTags, originalTags, availableTags, createNewTags), nil
}

func findExactDocumentTypeMatch(text string, availableDocumentTypes []string) string {
	normalizedText := strings.ToLower(text)
	matches := []string{}
	for _, docType := range availableDocumentTypes {
		if docType == "" {
			continue
		}
		if strings.Contains(normalizedText, strings.ToLower(docType)) {
			matches = append(matches, docType)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return ""
}

// getSuggestedDocumentType generates a suggested document type for a document using the LLM
func (app *App) getSuggestedDocumentType(
	ctx context.Context,
	content string,
	suggestedTitle string,
	availableDocumentTypes []string,
	logger *logrus.Entry) (string, error) {
	likelyLanguage := getLikelyLanguage()

	if exactMatch := findExactDocumentTypeMatch(suggestedTitle, availableDocumentTypes); exactMatch != "" {
		logger.WithField("document_type", exactMatch).Debug("Using exact document type match from title")
		return exactMatch, nil
	}

	templateMutex.RLock()
	defer templateMutex.RUnlock()

	// Get available tokens for content
	templateData := map[string]interface{}{
		"Language":                     likelyLanguage,
		"AvailableDocumentTypes":       availableDocumentTypes,
		"AvailableDocumentTypeContext": strings.Join(availableDocumentTypes, "\n"),
		"Title":                        suggestedTitle,
		"CreateNewDocumentTypes":       createNewDocumentTypes,
	}

	availableTokens, err := getAvailableTokensForContent(documentTypeTemplate, templateData)
	if err != nil {
		logger.Errorf("Error calculating available tokens: %v", err)
		return "", fmt.Errorf("error calculating available tokens: %v", err)
	}

	// Truncate content if needed
	truncatedContent, err := truncateContentByTokens(content, availableTokens)
	if err != nil {
		logger.Errorf("Error truncating content: %v", err)
		return "", fmt.Errorf("error truncating content: %v", err)
	}

	// Execute template with truncated content
	var promptBuffer bytes.Buffer
	templateData["Content"] = truncatedContent
	err = documentTypeTemplate.Execute(&promptBuffer, templateData)
	if err != nil {
		logger.Errorf("Error executing document type template: %v", err)
		return "", fmt.Errorf("error executing document type template: %v", err)
	}

	prompt := promptBuffer.String()
	logger.WithField("prompt_length", len(prompt)).Debug("Document type suggestion prompt prepared")

	completion, err := app.LLM.GenerateContent(ctx, []llms.MessageContent{
		{
			Parts: []llms.ContentPart{
				llms.TextContent{
					Text: prompt,
				},
			},
			Role: llms.ChatMessageTypeHuman,
		},
	})
	if err != nil {
		logger.Errorf("Error getting response from LLM: %v", err)
		return "", fmt.Errorf("error getting response from LLM: %v", err)
	}

	response := strings.TrimSpace(stripReasoning(completion.Choices[0].Content))

	return validateSuggestedDocumentType(response, availableDocumentTypes, createNewDocumentTypes, logger), nil
}

// getSuggestedTitle generates a suggested title for a document using the LLM
func (app *App) getSuggestedTitle(ctx context.Context, content string, originalTitle string, generationContext suggestionGenerationContext, logger *logrus.Entry) (string, error) {
	likelyLanguage := getLikelyLanguage()

	settingsMutex.RLock()
	titleSchema := settings.TitleSchema
	settingsMutex.RUnlock()
	if titleSchema == "" {
		titleSchema = defaultTitleSchema
	}

	templateMutex.RLock()
	defer templateMutex.RUnlock()

	// Get available tokens for content
	templateData := map[string]interface{}{
		"Language":                     likelyLanguage,
		"Content":                      content,
		"Title":                        originalTitle,
		"TitleSchema":                  titleSchema,
		"AvailableTagContext":          generationContext.availableTagContext,
		"AvailableDocumentTypeContext": generationContext.availableDocumentTypeContext,
	}

	availableTokens, err := getAvailableTokensForContent(titleTemplate, templateData)
	if err != nil {
		logger.Errorf("Error calculating available tokens: %v", err)
		return "", fmt.Errorf("error calculating available tokens: %v", err)
	}

	// Truncate content if needed
	truncatedContent, err := truncateContentByTokens(content, availableTokens)
	if err != nil {
		logger.Errorf("Error truncating content: %v", err)
		return "", fmt.Errorf("error truncating content: %v", err)
	}

	// Execute template with truncated content
	var promptBuffer bytes.Buffer
	templateData["Content"] = truncatedContent
	err = titleTemplate.Execute(&promptBuffer, templateData)

	if err != nil {
		return "", fmt.Errorf("error executing title template: %v", err)
	}

	prompt := promptBuffer.String()
	logger.WithField("prompt_length", len(prompt)).Debug("Title suggestion prompt prepared")

	completion, err := app.LLM.GenerateContent(ctx, []llms.MessageContent{
		{
			Parts: []llms.ContentPart{
				llms.TextContent{
					Text: prompt,
				},
			},
			Role: llms.ChatMessageTypeHuman,
		},
	})
	if err != nil {
		return "", fmt.Errorf("error getting response from LLM: %v", err)
	}
	result := stripReasoning(completion.Choices[0].Content)
	return strings.TrimSpace(strings.Trim(result, "\"")), nil
}

// getSuggestedCreatedDate generates a suggested createdDate for a document using the LLM
func (app *App) getSuggestedCreatedDate(ctx context.Context, content string, logger *logrus.Entry) (string, error) {
	likelyLanguage := getLikelyLanguage()

	templateMutex.RLock()
	defer templateMutex.RUnlock()

	// Get available tokens for content
	templateData := map[string]interface{}{
		"Language": likelyLanguage,
		"Content":  content,
		"Today":    getTodayDate(), // must be in YYYY-MM-DD format
	}

	availableTokens, err := getAvailableTokensForContent(createdDateTemplate, templateData)
	if err != nil {
		logger.Errorf("Error calculating available tokens: %v", err)
		return "", fmt.Errorf("error calculating available tokens: %v", err)
	}

	// Truncate content if needed
	truncatedContent, err := truncateContentByTokens(content, availableTokens)
	if err != nil {
		logger.Errorf("Error truncating content: %v", err)
		return "", fmt.Errorf("error truncating content: %v", err)
	}

	// Execute template with truncated content
	var promptBuffer bytes.Buffer
	templateData["Content"] = truncatedContent
	err = createdDateTemplate.Execute(&promptBuffer, templateData)

	if err != nil {
		return "", fmt.Errorf("error executing createdDate template: %v", err)
	}

	prompt := promptBuffer.String()
	logger.WithField("prompt_length", len(prompt)).Debug("CreatedDate suggestion prompt prepared")

	completion, err := app.LLM.GenerateContent(ctx, []llms.MessageContent{
		{
			Parts: []llms.ContentPart{
				llms.TextContent{
					Text: prompt,
				},
			},
			Role: llms.ChatMessageTypeHuman,
		},
	})
	if err != nil {
		return "", fmt.Errorf("error getting response from LLM: %v", err)
	}
	result := stripReasoning(completion.Choices[0].Content)
	return strings.TrimSpace(strings.Trim(result, "\"")), nil
}

// getSuggestedCustomFields generates suggested custom fields for a document using the LLM
func (app *App) getSuggestedCustomFields(ctx context.Context, doc Document, selectedFieldIDs []int, logger *logrus.Entry) ([]CustomFieldSuggestion, error) {
	// Fetch all available custom fields
	allCustomFields, err := app.Client.GetCustomFields(ctx)
	if err != nil {
		return nil, fmt.Errorf("error fetching all custom fields: %v", err)
	}

	// Filter to get only the selected custom fields
	var selectedCustomFields []CustomField
	for _, field := range allCustomFields {
		for _, selectedID := range selectedFieldIDs {
			if field.ID == selectedID {
				selectedCustomFields = append(selectedCustomFields, field)
				break
			}
		}
	}

	if len(selectedCustomFields) == 0 {
		return nil, nil // No fields to process
	}

	// Generate XML for the prompt
	var xmlBuilder strings.Builder
	xmlBuilder.WriteString("<custom_fields>\n")
	for _, field := range selectedCustomFields {
		xmlBuilder.WriteString(fmt.Sprintf("  <field name=\"%s\" type=\"%s\"></field>\n", field.Name, field.DataType))
	}
	xmlBuilder.WriteString("</custom_fields>")
	customFieldsXML := xmlBuilder.String()

	templateMutex.RLock()
	defer templateMutex.RUnlock()

	templateData := map[string]interface{}{
		"Language":        getLikelyLanguage(),
		"Title":           doc.Title,
		"CreatedDate":     doc.CreatedDate,
		"DocumentType":    doc.DocumentTypeName,
		"CustomFieldsXML": customFieldsXML,
	}

	availableTokens, err := getAvailableTokensForContent(customFieldTemplate, templateData)
	if err != nil {
		return nil, fmt.Errorf("error calculating available tokens for custom fields: %v", err)
	}

	truncatedContent, err := truncateContentByTokens(doc.Content, availableTokens)
	if err != nil {
		return nil, fmt.Errorf("error truncating content for custom fields: %v", err)
	}

	var promptBuffer bytes.Buffer
	templateData["Content"] = truncatedContent
	err = customFieldTemplate.Execute(&promptBuffer, templateData)
	if err != nil {
		return nil, fmt.Errorf("error executing custom field template: %v", err)
	}

	prompt := promptBuffer.String()
	logger.WithField("prompt_length", len(prompt)).Debug("Custom field suggestion prompt prepared")

	completion, err := app.LLM.GenerateContent(ctx, []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: prompt},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error getting response from LLM for custom fields: %v", err)
	}

	response := stripReasoning(completion.Choices[0].Content)
	response = stripMarkdown(response)
	logger.WithField("response_length", len(response)).Debug("LLM response for custom fields received")

	// Temporary struct to unmarshal LLM response with field name
	type LLMCustomFieldResponse struct {
		Field string      `json:"field"`
		Value interface{} `json:"value"`
	}

	var llmSuggestedFields []LLMCustomFieldResponse
	// Handle empty or non-JSON response gracefully
	if strings.TrimSpace(response) == "" || !strings.HasPrefix(strings.TrimSpace(response), "[") {
		return []CustomFieldSuggestion{}, nil
	}

	err = json.Unmarshal([]byte(response), &llmSuggestedFields)
	if err != nil {
		logger.Errorf("Error unmarshalling custom fields JSON from LLM response: %v. Response: %s", err, response)
		return []CustomFieldSuggestion{}, nil // Return empty slice on parsing error
	}

	// Map field names back to IDs
	fieldNameIdMap := make(map[string]int)
	for _, field := range allCustomFields {
		fieldNameIdMap[field.Name] = field.ID
	}

	var finalSuggestedFields []CustomFieldSuggestion
	for _, llmField := range llmSuggestedFields {
		if id, ok := fieldNameIdMap[llmField.Field]; ok {
			finalSuggestedFields = append(finalSuggestedFields, CustomFieldSuggestion{
				ID:    id,
				Name:  llmField.Field,
				Value: llmField.Value,
			})
		} else {
			logger.Warnf("LLM returned unknown custom field name '%s', skipping.", llmField.Field)
		}
	}

	return finalSuggestedFields, nil
}

type suggestionGenerationContext struct {
	availableTagNames            []string
	availableTagContext          string
	availableDetailedTags        []DetailedTag
	availableParentTagPaths      []string
	availableCorrespondentNames  []string
	availableDocumentTypeNames   []string
	availableDocumentTypeContext string
}

type coreMetadataSuggestion struct {
	Title           string
	Tags            []string
	RemoveTags      []string
	AddTagParents   map[string]int
	Correspondent   string
	DocumentType    string
	CreatedDate     string
	GeneratedFields int
}

type coreMetadataLLMResponse struct {
	Title         string   `json:"title"`
	Tags          []string `json:"tags"`
	RemoveTags    []string `json:"remove_tags"`
	Correspondent string   `json:"correspondent"`
	DocumentType  string   `json:"document_type"`
	CreatedDate   string   `json:"created_date"`
}

// generateDocumentSuggestions generates suggestions for a set of documents.
func (app *App) generateDocumentSuggestions(ctx context.Context, suggestionRequest GenerateSuggestionsRequest, logger *logrus.Entry) ([]DocumentSuggestion, error) {
	return app.generateDocumentSuggestionsSequential(ctx, suggestionRequest, "", logger)
}

func (app *App) generateDocumentSuggestionsSequential(ctx context.Context, suggestionRequest GenerateSuggestionsRequest, jobID string, logger *logrus.Entry) ([]DocumentSuggestion, error) {
	generationContext, err := app.prepareSuggestionGenerationContext(ctx, suggestionRequest)
	if err != nil {
		return nil, err
	}

	documentSuggestions := make([]DocumentSuggestion, 0, len(suggestionRequest.Documents))
	for index, doc := range suggestionRequest.Documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if jobID != "" {
			suggestionJobStore.updateProgress(jobID, index, doc.ID)
		}

		suggestion, err := app.generateSingleDocumentSuggestion(ctx, suggestionRequest, doc, generationContext, logger)
		if err != nil {
			return nil, err
		}

		documentSuggestions = append(documentSuggestions, suggestion)
		if jobID != "" {
			suggestionJobStore.updateProgress(jobID, index+1, 0)
		}
	}

	return documentSuggestions, nil
}

func (app *App) prepareSuggestionGenerationContext(ctx context.Context, suggestionRequest GenerateSuggestionsRequest) (suggestionGenerationContext, error) {
	generationContext := suggestionGenerationContext{}

	if suggestionRequest.GenerateTags {
		availableTags, err := app.Client.GetAllTagsDetailed(ctx)
		if err != nil {
			return suggestionGenerationContext{}, fmt.Errorf("failed to fetch available tags: %v", err)
		}

		generationContext.availableTagNames = make([]string, 0, len(availableTags))
		filteredTags := make([]Tag, 0, len(availableTags))
		detailedTags := buildDetailedTagsWithParentCandidates(availableTags, currentTagSelectionMode(), configuredNonClassificationTagNames(), configuredTagParentCandidateNames())
		availableByID := make(map[int]DetailedTag, len(detailedTags))
		for _, tag := range detailedTags {
			availableByID[tag.ID] = tag
		}
		for _, tag := range availableTags {
			detailedTag, exists := availableByID[tag.ID]
			if !exists || detailedTag.IsWorkflow || detailedTag.IsSystem {
				continue
			}
			if detailedTag.IsApplicable {
				generationContext.availableTagNames = append(generationContext.availableTagNames, tag.Name)
			}
			if detailedTag.IsParentCandidate {
				generationContext.availableParentTagPaths = append(generationContext.availableParentTagPaths, detailedTag.Path)
			}
			generationContext.availableDetailedTags = append(generationContext.availableDetailedTags, detailedTag)
			filteredTags = append(filteredTags, tag)
		}
		slices.Sort(generationContext.availableTagNames)
		slices.Sort(generationContext.availableParentTagPaths)
		generationContext.availableTagContext = formatTagTaxonomy(filteredTags)
	}

	if suggestionRequest.GenerateCorrespondents {
		availableCorrespondentsMap, err := app.Client.GetAllCorrespondents(ctx)
		if err != nil {
			return suggestionGenerationContext{}, fmt.Errorf("failed to fetch available correspondents: %v", err)
		}

		generationContext.availableCorrespondentNames = make([]string, 0, len(availableCorrespondentsMap))
		for correspondentName := range availableCorrespondentsMap {
			generationContext.availableCorrespondentNames = append(generationContext.availableCorrespondentNames, correspondentName)
		}
	}

	if suggestionRequest.GenerateDocumentTypes {
		availableDocumentTypes, err := app.Client.GetAllDocumentTypes(ctx)
		if err != nil {
			return suggestionGenerationContext{}, fmt.Errorf("failed to fetch available document types: %v", err)
		}

		generationContext.availableDocumentTypeNames = make([]string, 0, len(availableDocumentTypes))
		for _, docType := range availableDocumentTypes {
			generationContext.availableDocumentTypeNames = append(generationContext.availableDocumentTypeNames, docType.Name)
		}
		slices.Sort(generationContext.availableDocumentTypeNames)
		generationContext.availableDocumentTypeContext = strings.Join(generationContext.availableDocumentTypeNames, "\n")
	}

	return generationContext, nil
}

func (app *App) getSuggestedCoreMetadata(ctx context.Context, suggestionRequest GenerateSuggestionsRequest, doc Document, generationContext suggestionGenerationContext, logger *logrus.Entry) (coreMetadataSuggestion, error) {
	settingsMutex.RLock()
	titleSchema := settings.TitleSchema
	settingsMutex.RUnlock()
	if titleSchema == "" {
		titleSchema = defaultTitleSchema
	}

	availableTags := removeTagFromList(generationContext.availableTagNames, manualTag)
	availableTags = removeTagFromList(availableTags, autoTag)
	availableTags = removeTagFromList(availableTags, autoOcrTag)

	templateMutex.RLock()
	defer templateMutex.RUnlock()

	templateData := map[string]interface{}{
		"Language":                     getLikelyLanguage(),
		"Title":                        doc.Title,
		"TitleSchema":                  titleSchema,
		"Today":                        getTodayDate(),
		"Content":                      doc.Content,
		"GenerateTitles":               suggestionRequest.GenerateTitles,
		"GenerateTags":                 suggestionRequest.GenerateTags,
		"GenerateCorrespondents":       suggestionRequest.GenerateCorrespondents,
		"GenerateDocumentTypes":        suggestionRequest.GenerateDocumentTypes,
		"GenerateCreatedDate":          suggestionRequest.GenerateCreatedDate,
		"AvailableTags":                availableTags,
		"AvailableTagContext":          generationContext.availableTagContext,
		"AvailableTagParents":          generationContext.availableParentTagPaths,
		"OriginalTags":                 doc.Tags,
		"AvailableCorrespondents":      generationContext.availableCorrespondentNames,
		"BlackList":                    correspondentBlackList,
		"AvailableDocumentTypeContext": generationContext.availableDocumentTypeContext,
	}

	availableTokens, err := getAvailableTokensForContent(metadataTemplate, templateData)
	if err != nil {
		return coreMetadataSuggestion{}, fmt.Errorf("error calculating available tokens for metadata: %v", err)
	}

	truncatedContent, err := truncateContentByTokens(doc.Content, availableTokens)
	if err != nil {
		return coreMetadataSuggestion{}, fmt.Errorf("error truncating content for metadata: %v", err)
	}

	var promptBuffer bytes.Buffer
	templateData["Content"] = truncatedContent
	if err := metadataTemplate.Execute(&promptBuffer, templateData); err != nil {
		return coreMetadataSuggestion{}, fmt.Errorf("error executing metadata template: %v", err)
	}

	prompt := promptBuffer.String()
	logger.WithField("prompt_length", len(prompt)).Debug("Core metadata suggestion prompt prepared")

	completion, err := app.LLM.GenerateContent(ctx, []llms.MessageContent{
		{
			Parts: []llms.ContentPart{
				llms.TextContent{Text: prompt},
			},
			Role: llms.ChatMessageTypeHuman,
		},
	}, llms.WithJSONMode(), llms.WithTemperature(0))
	if err != nil {
		return coreMetadataSuggestion{}, fmt.Errorf("error getting response from LLM: %v", err)
	}

	response := stripMarkdown(stripReasoning(completion.Choices[0].Content))
	var llmResponse coreMetadataLLMResponse
	if err := json.Unmarshal([]byte(response), &llmResponse); err != nil {
		return coreMetadataSuggestion{}, fmt.Errorf("error parsing metadata JSON response: %v", err)
	}

	result := coreMetadataSuggestion{}
	if suggestionRequest.GenerateTitles {
		result.Title = cleanLLMScalar(llmResponse.Title)
		result.GeneratedFields++
	}
	if suggestionRequest.GenerateTags {
		result.Tags, result.AddTagParents = filterSuggestedTagsWithParents(
			llmResponse.Tags,
			doc.Tags,
			availableTags,
			generationContext.availableDetailedTags,
			createNewTags,
		)
		result.RemoveTags = filterSuggestedRemoveTags(llmResponse.RemoveTags, doc.Tags)
		result.GeneratedFields++
	}
	if suggestionRequest.GenerateCorrespondents {
		result.Correspondent = validateSuggestedCorrespondent(cleanLLMScalar(llmResponse.Correspondent), generationContext.availableCorrespondentNames, correspondentBlackList)
		result.GeneratedFields++
	}
	if suggestionRequest.GenerateDocumentTypes {
		result.DocumentType = validateSuggestedDocumentType(cleanLLMScalar(llmResponse.DocumentType), generationContext.availableDocumentTypeNames, createNewDocumentTypes, logger)
		result.GeneratedFields++
	}
	if suggestionRequest.GenerateCreatedDate {
		result.CreatedDate = cleanLLMScalar(llmResponse.CreatedDate)
		result.GeneratedFields++
	}

	return result, nil
}

func (app *App) generateSingleDocumentSuggestion(ctx context.Context, suggestionRequest GenerateSuggestionsRequest, doc Document, generationContext suggestionGenerationContext, logger *logrus.Entry) (DocumentSuggestion, error) {
	documentID := doc.ID
	docLogger := documentLogger(documentID)
	startTime := time.Now()
	docLogger.Printf("Processing Document ID %d...", documentID)

	suggestedTitle := doc.Title
	var suggestedTags []string
	var suggestedRemoveTags []string
	var suggestedTagParents map[string]int
	var suggestedCorrespondent string
	var suggestedDocumentType string
	var suggestedCreatedDate string
	var suggestedCustomFields []CustomFieldSuggestion
	fieldErrors := map[string]string{}
	successfulFields := 0

	if suggestionRequest.GenerateTitles || suggestionRequest.GenerateTags || suggestionRequest.GenerateCorrespondents || suggestionRequest.GenerateDocumentTypes || suggestionRequest.GenerateCreatedDate {
		metadata, err := app.getSuggestedCoreMetadata(ctx, suggestionRequest, doc, generationContext, docLogger)
		if err != nil {
			docLogger.Errorf("Error processing document %d: %v", documentID, err)
			if suggestionRequest.GenerateTitles {
				fieldErrors["title"] = err.Error()
			}
			if suggestionRequest.GenerateTags {
				fieldErrors["tags"] = err.Error()
			}
			if suggestionRequest.GenerateCorrespondents {
				fieldErrors["correspondent"] = err.Error()
			}
			if suggestionRequest.GenerateDocumentTypes {
				fieldErrors["document_type"] = err.Error()
			}
			if suggestionRequest.GenerateCreatedDate {
				fieldErrors["created_date"] = err.Error()
			}
		} else {
			if suggestionRequest.GenerateTitles {
				suggestedTitle = metadata.Title
				successfulFields++
			}
			if suggestionRequest.GenerateTags {
				suggestedTags = metadata.Tags
				suggestedRemoveTags = metadata.RemoveTags
				suggestedTagParents = metadata.AddTagParents
				successfulFields++
			}
			if suggestionRequest.GenerateCorrespondents {
				suggestedCorrespondent = metadata.Correspondent
				successfulFields++
			}
			if suggestionRequest.GenerateDocumentTypes {
				suggestedDocumentType = metadata.DocumentType
				successfulFields++
			}
			if suggestionRequest.GenerateCreatedDate {
				suggestedCreatedDate = metadata.CreatedDate
				successfulFields++
			}
		}
	}

	if suggestionRequest.GenerateCustomFields {
		settingsMutex.RLock()
		selectedIDs := settings.CustomFieldsSelectedIDs
		settingsMutex.RUnlock()

		if len(selectedIDs) == 0 {
			log.Warnf("Custom field generation is enabled, but no custom fields are selected in the settings. Please select at least one custom field for this feature to work.")
		} else {
			var err error
			suggestedCustomFields, err = app.getSuggestedCustomFields(ctx, doc, selectedIDs, docLogger)
			if err != nil {
				log.Errorf("Error generating custom fields for document %d: %v", documentID, err)
				fieldErrors["custom_fields"] = err.Error()
			} else {
				successfulFields++
			}
		}
	}

	if len(fieldErrors) > 0 && successfulFields == 0 {
		return DocumentSuggestion{}, fmt.Errorf("Document %d: all requested suggestion fields failed", documentID)
	}

	suggestion := DocumentSuggestion{
		ID:               documentID,
		OriginalDocument: doc,
	}
	settingsMutex.RLock()
	suggestion.CustomFieldsWriteMode = settings.CustomFieldsWriteMode
	suggestion.CustomFieldsEnable = settings.CustomFieldsEnable
	settingsMutex.RUnlock()

	if suggestionRequest.GenerateTitles {
		docLogger.Printf("Suggested title for document %d: %s", documentID, suggestedTitle)
		suggestion.SuggestedTitle = suggestedTitle
	} else {
		suggestion.SuggestedTitle = doc.Title
	}

	if suggestionRequest.GenerateTags {
		docLogger.Printf("Suggested tags for document %d: %v", documentID, suggestedTags)
		suggestion.SuggestedTags = suggestedTags
		suggestion.AddTagParents = suggestedTagParents
	} else {
		suggestion.SuggestedTags = doc.Tags
	}

	if suggestionRequest.GenerateCorrespondents {
		log.Printf("Suggested correspondent for document %d: %s", documentID, suggestedCorrespondent)
		suggestion.SuggestedCorrespondent = suggestedCorrespondent
	}

	if suggestionRequest.GenerateDocumentTypes {
		log.Printf("Suggested document type for document %d: %s", documentID, suggestedDocumentType)
		suggestion.SuggestedDocumentType = suggestedDocumentType
	}

	if suggestionRequest.GenerateCreatedDate {
		log.Printf("Suggested createdDate for document %d: %s", documentID, suggestedCreatedDate)
		suggestion.SuggestedCreatedDate = suggestedCreatedDate
	}

	if suggestionRequest.GenerateCustomFields {
		log.Printf("Suggested custom fields for document %d: %v", documentID, suggestedCustomFields)
		suggestion.SuggestedCustomFields = suggestedCustomFields
	}
	if len(fieldErrors) > 0 {
		suggestion.FieldErrors = fieldErrors
	}

	suggestion.RemoveTags = appendUniqueStrings(suggestedRemoveTags, manualTag, autoTag)

	elapsed := time.Since(startTime)
	runtime := time.Unix(0, elapsed.Nanoseconds()).UTC()
	docLogger.Printf("Document %d processed successfully. Runtime: %s", documentID, runtime.Format("15:04:05"))

	return suggestion, nil
}

// getTodayDate returns the current date in YYYY-MM-DD format
func getTodayDate() string {
	return time.Now().Format("2006-01-02")
}

// stripReasoning removes the reasoning from the content indicated by <think> and </think> tags.
func stripReasoning(content string) string {
	// Remove reasoning from the content
	reasoningStart := strings.Index(content, "<think>")
	if reasoningStart != -1 {
		reasoningEnd := strings.Index(content, "</think>")
		if reasoningEnd != -1 {
			content = content[:reasoningStart] + content[reasoningEnd+len("</think>"):]
		}
	}

	// Trim whitespace
	content = strings.TrimSpace(content)
	return content
}

// stripMarkdown removes the markdown code block from the content.
func stripMarkdown(content string) string {
	// Remove markdown code block
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
	}
	return strings.TrimSpace(content)
}
