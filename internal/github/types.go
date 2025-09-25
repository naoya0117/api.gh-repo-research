package github

type Repository struct {
	URL            string                 `json:"url"`
	FullName       string                 `json:"nameWithOwner"`
	StargazerCount int                    `json:"stargazerCount"`
	PrimaryLanguage *PrimaryLanguage      `json:"primaryLanguage"`
}

type PrimaryLanguage struct {
	Name string `json:"name"`
}

type PageInfo struct {
	HasNextPage bool    `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor"`
}

type RepositoriesResponse struct {
	Repositories []Repository `json:"repositories"`
	PageInfo     PageInfo     `json:"pageInfo"`
}
