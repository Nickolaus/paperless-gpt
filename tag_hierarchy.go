package main

import (
	"os"
	"slices"
	"strings"
)

const (
	tagSelectionModeAll        = "all"
	tagSelectionModeApplicable = "applicable"
)

type DetailedTag struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	ParentID          *int   `json:"parent_id,omitempty"`
	Path              string `json:"path"`
	Depth             int    `json:"depth"`
	HasChildren       bool   `json:"has_children"`
	IsApplicable      bool   `json:"is_applicable"`
	IsWorkflow        bool   `json:"is_workflow"`
	IsSystem          bool   `json:"is_system"`
	IsDerived         bool   `json:"is_derived"`
	IsParentCandidate bool   `json:"is_parent_candidate"`
}

type DetailedTagsResponse struct {
	Tags           []DetailedTag `json:"tags"`
	SelectionMode  string        `json:"selection_mode"`
	DerivedParents bool          `json:"derived_parents"`
	CreateNewTags  bool          `json:"create_new_tags"`
}

type tagHierarchy struct {
	tagsByName       map[string]DetailedTag
	tagsByID         map[int]DetailedTag
	childrenByParent map[int][]DetailedTag
}

func currentTagSelectionMode() string {
	if strings.EqualFold(strings.TrimSpace(envDefault("TAG_SELECTION_MODE", tagSelectionModeAll)), tagSelectionModeApplicable) {
		return tagSelectionModeApplicable
	}
	return tagSelectionModeAll
}

func currentTagDerivedParents() bool {
	return !strings.EqualFold(strings.TrimSpace(envDefault("TAG_DERIVED_PARENTS", "true")), "false")
}

func configuredNonClassificationTagNames() map[string]bool {
	tagNames := map[string]bool{}
	for _, name := range strings.Split(envDefault("TAG_NON_CLASSIFICATION_NAMES", ""), ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			tagNames[strings.ToLower(name)] = true
		}
	}
	return tagNames
}

func configuredTagParentCandidateNames() map[string]bool {
	tagNames := map[string]bool{}
	for _, name := range strings.Split(envDefault("TAG_PARENT_CANDIDATE_NAMES", ""), ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			tagNames[strings.ToLower(name)] = true
		}
	}
	return tagNames
}

func envDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func workflowTagNames() map[string]bool {
	tagNames := map[string]bool{}
	for _, name := range []string{manualTag, autoTag, manualOcrTag, autoOcrTag, pdfOCRCompleteTag} {
		name = strings.TrimSpace(name)
		if name != "" {
			tagNames[strings.ToLower(name)] = true
		}
	}
	return tagNames
}

func buildDetailedTags(tags []Tag, selectionMode string, nonClassificationTags map[string]bool) []DetailedTag {
	return buildDetailedTagsWithParentCandidates(tags, selectionMode, nonClassificationTags, nil)
}

func buildDetailedTagsWithParentCandidates(tags []Tag, selectionMode string, nonClassificationTags map[string]bool, parentCandidateNames map[string]bool) []DetailedTag {
	tagsByID := map[int]Tag{}
	childrenByParentID := map[int][]Tag{}
	for _, tag := range tags {
		if strings.TrimSpace(tag.Name) == "" {
			continue
		}
		tagsByID[tag.ID] = tag
	}
	for _, tag := range tags {
		if tag.ParentID != nil {
			if _, exists := tagsByID[*tag.ParentID]; exists {
				childrenByParentID[*tag.ParentID] = append(childrenByParentID[*tag.ParentID], tag)
			}
		}
	}

	workflowTags := workflowTagNames()
	detailedTags := make([]DetailedTag, 0, len(tagsByID))
	for _, tag := range tagsByID {
		pathParts := tagPathParts(tag, tagsByID, map[int]bool{})
		nameKey := strings.ToLower(tag.Name)
		hasChildren := len(childrenByParentID[tag.ID]) > 0
		isWorkflow := workflowTags[nameKey]
		isSystem := nonClassificationTags[nameKey]
		isParentCandidate := !isWorkflow && !isSystem
		if len(parentCandidateNames) > 0 {
			isParentCandidate = parentCandidateNames[nameKey]
		}
		isApplicable := true
		if selectionMode == tagSelectionModeApplicable {
			isApplicable = !hasChildren && !isWorkflow && !isSystem
		}

		detailedTags = append(detailedTags, DetailedTag{
			ID:                tag.ID,
			Name:              tag.Name,
			ParentID:          tag.ParentID,
			Path:              strings.Join(pathParts, " / "),
			Depth:             len(pathParts) - 1,
			HasChildren:       hasChildren,
			IsApplicable:      isApplicable,
			IsWorkflow:        isWorkflow,
			IsSystem:          isSystem,
			IsDerived:         false,
			IsParentCandidate: isParentCandidate,
		})
	}

	slices.SortFunc(detailedTags, func(left, right DetailedTag) int {
		return strings.Compare(strings.ToLower(left.Path), strings.ToLower(right.Path))
	})
	return detailedTags
}

func tagPathParts(tag Tag, tagsByID map[int]Tag, visiting map[int]bool) []string {
	if visiting[tag.ID] {
		return []string{tag.Name}
	}
	visiting[tag.ID] = true
	defer delete(visiting, tag.ID)

	if tag.ParentID == nil {
		return []string{tag.Name}
	}
	parent, exists := tagsByID[*tag.ParentID]
	if !exists {
		return []string{tag.Name}
	}
	return append(tagPathParts(parent, tagsByID, visiting), tag.Name)
}

func newTagHierarchy(tags []DetailedTag) tagHierarchy {
	hierarchy := tagHierarchy{
		tagsByName:       map[string]DetailedTag{},
		tagsByID:         map[int]DetailedTag{},
		childrenByParent: map[int][]DetailedTag{},
	}
	for _, tag := range tags {
		hierarchy.tagsByName[strings.ToLower(tag.Name)] = tag
		hierarchy.tagsByID[tag.ID] = tag
	}
	for _, tag := range tags {
		if tag.ParentID != nil {
			hierarchy.childrenByParent[*tag.ParentID] = append(hierarchy.childrenByParent[*tag.ParentID], tag)
		}
	}
	return hierarchy
}

func (hierarchy tagHierarchy) parentChain(tag DetailedTag) []DetailedTag {
	if tag.ParentID == nil {
		return nil
	}
	parent, exists := hierarchy.tagsByID[*tag.ParentID]
	if !exists {
		return nil
	}
	parents := hierarchy.parentChain(parent)
	parents = append(parents, parent)
	return parents
}

func (hierarchy tagHierarchy) descendantNames(tag DetailedTag) []string {
	descendants := []string{}
	var walk func(parent DetailedTag)
	walk = func(parent DetailedTag) {
		for _, child := range hierarchy.childrenByParent[parent.ID] {
			descendants = append(descendants, child.Name)
			walk(child)
		}
	}
	walk(tag)
	return descendants
}

func normalizeTagNamesForApply(tagNames []string, removeTags []string, detailedTags []DetailedTag, addTagParents map[string]int, includeDerivedParents bool) []string {
	hierarchy := newTagHierarchy(detailedTags)
	removeSet := tagNameSet(removeTags)
	finalSet := map[string]string{}

	for _, tagName := range tagNames {
		cleanedTag := cleanLLMScalar(tagName)
		if cleanedTag == "" || removeSet[strings.ToLower(cleanedTag)] {
			continue
		}
		finalSet[strings.ToLower(cleanedTag)] = cleanedTag
	}

	if includeDerivedParents {
		for _, tagName := range stringMapValues(finalSet) {
			tag, exists := hierarchy.tagsByName[strings.ToLower(tagName)]
			if !exists {
				if parentID := lookupTagParentID(addTagParents, tagName); parentID != nil {
					if parent, parentExists := hierarchy.tagsByID[*parentID]; parentExists && !removeSet[strings.ToLower(parent.Name)] {
						finalSet[strings.ToLower(parent.Name)] = parent.Name
						for _, ancestor := range hierarchy.parentChain(parent) {
							if !removeSet[strings.ToLower(ancestor.Name)] {
								finalSet[strings.ToLower(ancestor.Name)] = ancestor.Name
							}
						}
					}
				}
				continue
			}
			for _, parent := range hierarchy.parentChain(tag) {
				if !removeSet[strings.ToLower(parent.Name)] {
					finalSet[strings.ToLower(parent.Name)] = parent.Name
				}
			}
		}
	}

	for _, tag := range detailedTags {
		if !tag.HasChildren {
			continue
		}
		if _, exists := finalSet[strings.ToLower(tag.Name)]; !exists {
			continue
		}
		hasSelectedChild := false
		for _, descendantName := range hierarchy.descendantNames(tag) {
			if _, exists := finalSet[strings.ToLower(descendantName)]; exists {
				hasSelectedChild = true
				break
			}
		}
		if !hasSelectedChild && removeSet[strings.ToLower(tag.Name)] {
			delete(finalSet, strings.ToLower(tag.Name))
		}
	}

	normalized := stringMapValues(finalSet)
	slices.Sort(normalized)
	return normalized
}

func tagNameSet(tagNames []string) map[string]bool {
	set := map[string]bool{}
	for _, tagName := range tagNames {
		tagName = cleanLLMScalar(tagName)
		if tagName != "" {
			set[strings.ToLower(tagName)] = true
		}
	}
	return set
}

func stringMapValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}
