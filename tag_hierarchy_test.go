package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDetailedTagsApplicableMode(t *testing.T) {
	parentID := 1
	tags := []Tag{
		{ID: 1, Name: "Finanzen"},
		{ID: 2, Name: "Kontoauszug", ParentID: &parentID},
		{ID: 3, Name: "Posteingang"},
	}

	detailedTags := buildDetailedTags(tags, tagSelectionModeApplicable, map[string]bool{"posteingang": true})

	byName := map[string]DetailedTag{}
	for _, tag := range detailedTags {
		byName[tag.Name] = tag
	}

	require.Len(t, detailedTags, 3)
	assert.False(t, byName["Finanzen"].IsApplicable)
	assert.True(t, byName["Finanzen"].HasChildren)
	assert.True(t, byName["Kontoauszug"].IsApplicable)
	assert.Equal(t, "Finanzen / Kontoauszug", byName["Kontoauszug"].Path)
	assert.False(t, byName["Posteingang"].IsApplicable)
	assert.True(t, byName["Posteingang"].IsSystem)
}

func TestBuildDetailedTagsParentCandidatesCanBeRestricted(t *testing.T) {
	detailedTags := buildDetailedTagsWithParentCandidates([]Tag{
		{ID: 1, Name: "Finanzen"},
		{ID: 2, Name: "Kontoauszug"},
	}, tagSelectionModeApplicable, nil, map[string]bool{"finanzen": true})

	byName := map[string]DetailedTag{}
	for _, tag := range detailedTags {
		byName[tag.Name] = tag
	}

	assert.True(t, byName["Finanzen"].IsParentCandidate)
	assert.False(t, byName["Kontoauszug"].IsParentCandidate)
}

func TestNormalizeTagNamesForApplyAddsDerivedParents(t *testing.T) {
	parentID := 1
	detailedTags := buildDetailedTags([]Tag{
		{ID: 1, Name: "Finanzen"},
		{ID: 2, Name: "Kontoauszug", ParentID: &parentID},
	}, tagSelectionModeApplicable, nil)

	normalized := normalizeTagNamesForApply([]string{"", "Kontoauszug"}, nil, detailedTags, nil, true)

	assert.Equal(t, []string{"Finanzen", "Kontoauszug"}, normalized)
}

func TestNormalizeTagNamesForApplyRemovesExplicitlyRemovedParent(t *testing.T) {
	detailedTags := buildDetailedTags([]Tag{
		{ID: 1, Name: "Finanzen"},
	}, tagSelectionModeApplicable, nil)

	normalized := normalizeTagNamesForApply([]string{"Finanzen"}, []string{"Finanzen"}, detailedTags, nil, true)

	assert.Empty(t, normalized)
}

func TestNormalizeTagNamesForApplyAddsParentForNewChildTag(t *testing.T) {
	detailedTags := buildDetailedTags([]Tag{
		{ID: 1, Name: "Finanzen"},
	}, tagSelectionModeApplicable, nil)

	normalized := normalizeTagNamesForApply([]string{"Bausparen"}, nil, detailedTags, map[string]int{"bausparen": 1}, true)

	assert.Equal(t, []string{"Bausparen", "Finanzen"}, normalized)
}
