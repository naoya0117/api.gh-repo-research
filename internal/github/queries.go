package github

import (
	"context"
	"fmt"
	"strings"
)

const (
	SearchStarsRepositoriesQuery = `
		query searchStarsRepositories($query: String!, $first: Int!, $after: String) {
			search(
				first: $first,
				query: $query,
				type: REPOSITORY
				after: $after
			) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					... on Repository {
						url
						nameWithOwner
						stargazerCount
						primaryLanguage {
							name
						}
					}
				}
			}
		}
	`

	GetRepositoryFilesQuery = `
		query getRepositoryFiles($owner: String!, $name: String!) {
			repository(owner: $owner, name: $name) {
				defaultBranchRef {
					target {
						... on Commit {
							tree {
								entries {
									name
									type
									object {
										... on Tree {
											entries {
												name
												type
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	`
)

func (c *Client) GetNextRepositories(ctx context.Context, currentCursor string) (*RepositoriesResponse, error) {
	variables := map[string]interface{}{
		"query": "docker sort:stars-desc in:readme",
		"first": 100,
		"after": currentCursor,
	}

	var response struct {
		Search struct {
			Repositories []Repository `json:"nodes"`
			PageInfo     PageInfo     `json:"pageInfo"`
		} `json:"search"`
	}

	if err := c.Query(ctx, SearchStarsRepositoriesQuery, variables, &response); err != nil {
		return nil, fmt.Errorf("failed to get repositories: %w", err)
	}

	return &RepositoriesResponse{
		Repositories: response.Search.Repositories,
		PageInfo:     response.Search.PageInfo,
	}, nil
}

func isDockerfile(filename string) bool {
	lowerName := strings.ToLower(filename)
	return strings.Contains(lowerName, "dockerfile")
}

func (c *Client) HasDockerfile(ctx context.Context, owner, name string) (bool, error) {
	variables := map[string]interface{}{
		"owner": owner,
		"name":  name,
	}

	var response struct {
		Repository struct {
			DefaultBranchRef struct {
				Target struct {
					Tree struct {
						Entries []struct {
							Name   string `json:"name"`
							Type   string `json:"type"`
							Object struct {
								Entries []struct {
									Name string `json:"name"`
									Type string `json:"type"`
								} `json:"entries"`
							} `json:"object"`
						} `json:"entries"`
					} `json:"tree"`
				} `json:"target"`
			} `json:"defaultBranchRef"`
		} `json:"repository"`
	}

	if err := c.Query(ctx, GetRepositoryFilesQuery, variables, &response); err != nil {
		return false, fmt.Errorf("failed to get repository files: %w", err)
	}

	for _, entry := range response.Repository.DefaultBranchRef.Target.Tree.Entries {
		if isDockerfile(entry.Name) {
			return true, nil
		}

		if entry.Type == "tree" {
			for _, subEntry := range entry.Object.Entries {
				if isDockerfile(subEntry.Name) {
					return true, nil
				}
			}
		}
	}

	return false, nil
}
