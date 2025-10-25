package graph

import (
	"strings"

	"github.com/naoya0117/shuron2025/api/graph/model"
	"github.com/naoya0117/shuron2025/api/internal/database"
)

// Helper functions for converting database models to GraphQL models

// Helper function to convert string to *string
func strPtr(s string) *string {
	return &s
}

func convertK8sPatterns(patterns []database.K8sPattern) []*model.K8sPattern {
	result := make([]*model.K8sPattern, 0, len(patterns))
	for _, p := range patterns {
		var description *string
		if p.Description != nil && strings.TrimSpace(*p.Description) != "" {
			desc := strings.TrimSpace(*p.Description)
			description = &desc
		}
		result = append(result, &model.K8sPattern{
			ID:          int32(p.ID),
			Name:        p.Name,
			Description: description,
			CreatedAt:   p.CreatedAt,
		})
	}
	return result
}

func convertCheckItems(items []database.CheckItem) []*model.CheckItem {
	result := make([]*model.CheckItem, 0, len(items))
	for _, item := range items {
		var description *string
		if item.Description != nil && strings.TrimSpace(*item.Description) != "" {
			desc := strings.TrimSpace(*item.Description)
			description = &desc
		}
		result = append(result, &model.CheckItem{
			ID:          int32(item.ID),
			PatternID:   int32(item.PatternID),
			Name:        item.Name,
			Description: description,
			CreatedAt:   item.CreatedAt,
		})
	}
	return result
}

func convertCheckResults(results []database.CheckResult) []*model.CheckResult {
	result := make([]*model.CheckResult, 0, len(results))
	for _, r := range results {
		var memo *string
		if r.Memo != nil && strings.TrimSpace(*r.Memo) != "" {
			m := strings.TrimSpace(*r.Memo)
			memo = &m
		}
		result = append(result, &model.CheckResult{
			ID:           int32(r.ID),
			RepositoryID: int32(r.RepositoryID),
			CheckItemID:  int32(r.CheckItemID),
			Result:       r.Result,
			Memo:         memo,
			CheckedAt:    r.CheckedAt,
			UpdatedAt:    r.UpdatedAt,
		})
	}
	return result
}

func convertRepositories(repos []database.Repository) []*model.Repository {
	result := make([]*model.Repository, 0, len(repos))
	for _, repo := range repos {
		var primaryLanguage *string
		if repo.PrimaryLanguage != nil && strings.TrimSpace(*repo.PrimaryLanguage) != "" {
			lang := strings.TrimSpace(*repo.PrimaryLanguage)
			primaryLanguage = &lang
		}
		result = append(result, &model.Repository{
			ID:              int32(repo.ID),
			NameWithOwner:   repo.NameWithOwner,
			StargazerCount:  int32(repo.StargazerCount),
			PrimaryLanguage: primaryLanguage,
			HasDockerfile:   repo.HasDockerfile,
			CreatedAt:       repo.CreatedAt,
			IsWebApp:        repo.IsWebApp,
			WebAppCheckedAt: repo.WebAppCheckedAt,
		})
	}
	return result
}
