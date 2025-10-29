package database

import (
	"database/sql"
	"errors"
	"time"
)

type Repository struct {
	ID              int        `json:"id"`
	URL             string     `json:"url"`
	NameWithOwner   string     `json:"nameWithOwner"`
	StargazerCount  int        `json:"stargazerCount"`
	PrimaryLanguage *string    `json:"primaryLanguage"`
	HasDockerfile   bool       `json:"hasDockerfile"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	IsWebApp        *bool      `json:"isWebApp"`
	WebAppCheckedAt *time.Time `json:"webAppCheckedAt"`
}

type SearchState struct {
	ID              int       `json:"id"`
	SessionID       string    `json:"sessionId"`
	Query           string    `json:"query"`
	CurrentLanguage *string   `json:"currentLanguage"`
	CurrentCursor   *string   `json:"currentCursor"`
	TotalFetched    int       `json:"totalFetched"`
	IsCompleted     bool      `json:"isCompleted"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (db *DB) CreateRepositoriesTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS repositories (
			id SERIAL PRIMARY KEY,
			url VARCHAR(255) UNIQUE NOT NULL,
			name_with_owner VARCHAR(255) NOT NULL,
			stargazer_count INTEGER NOT NULL,
			primary_language VARCHAR(100),
			has_dockerfile BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := db.Exec(query)
	return err
}

func (db *DB) CreateSearchStatesTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS search_states (
			id SERIAL PRIMARY KEY,
			session_id VARCHAR(255) UNIQUE NOT NULL,
			query VARCHAR(500) NOT NULL,
			current_language VARCHAR(100),
			current_cursor TEXT,
			total_fetched INTEGER DEFAULT 0,
			is_completed BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := db.Exec(query)
	return err
}

func (db *DB) InsertRepository(repo Repository) error {
	query := `
		INSERT INTO repositories (url, name_with_owner, stargazer_count, primary_language, has_dockerfile)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (url) DO UPDATE SET
			name_with_owner = EXCLUDED.name_with_owner,
			stargazer_count = EXCLUDED.stargazer_count,
			primary_language = EXCLUDED.primary_language,
			has_dockerfile = EXCLUDED.has_dockerfile,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, repo.URL, repo.NameWithOwner, repo.StargazerCount, repo.PrimaryLanguage, repo.HasDockerfile)
	return err
}

func (db *DB) GetRepositories(limit, offset int) ([]Repository, error) {
	query := `
		SELECT
			r.id, r.url, r.name_with_owner, r.stargazer_count, r.primary_language,
			r.has_dockerfile, r.created_at, r.updated_at,
			w.is_web_app, w.updated_at as web_app_checked_at
		FROM repositories r
		LEFT JOIN repository_webapp_checks w ON r.id = w.id
		ORDER BY r.stargazer_count DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Log the error or handle it appropriately
			// For now, we'll ignore it as it's typically not critical
		}
	}()

	var repositories []Repository
	for rows.Next() {
		var repo Repository
		var primaryLanguage sql.NullString
		var isWebApp sql.NullBool
		var webAppCheckedAt sql.NullTime

		err := rows.Scan(
			&repo.ID,
			&repo.URL,
			&repo.NameWithOwner,
			&repo.StargazerCount,
			&primaryLanguage,
			&repo.HasDockerfile,
			&repo.CreatedAt,
			&repo.UpdatedAt,
			&isWebApp,
			&webAppCheckedAt,
		)
		if err != nil {
			return nil, err
		}

		if primaryLanguage.Valid {
			repo.PrimaryLanguage = &primaryLanguage.String
		}
		if isWebApp.Valid {
			repo.IsWebApp = &isWebApp.Bool
		}
		if webAppCheckedAt.Valid {
			repo.WebAppCheckedAt = &webAppCheckedAt.Time
		}

		repositories = append(repositories, repo)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return repositories, nil
}

func (db *DB) GetUnevaluatedRepositoriesWithDockerfile(limit, offset int) ([]Repository, error) {
	query := `
		SELECT
			r.id, r.url, r.name_with_owner, r.stargazer_count, r.primary_language,
			r.has_dockerfile, r.created_at, r.updated_at,
			w.is_web_app, w.updated_at as web_app_checked_at
		FROM repositories r
		LEFT JOIN repository_webapp_checks w ON r.id = w.id
		WHERE r.has_dockerfile = true AND (w.is_web_app IS NULL)
		ORDER BY r.stargazer_count DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Log the error or handle it appropriately
			// For now, we'll ignore it as it's typically not critical
		}
	}()

	var repositories []Repository
	for rows.Next() {
		var repo Repository
		var primaryLanguage sql.NullString
		var isWebApp sql.NullBool
		var webAppCheckedAt sql.NullTime

		err := rows.Scan(
			&repo.ID,
			&repo.URL,
			&repo.NameWithOwner,
			&repo.StargazerCount,
			&primaryLanguage,
			&repo.HasDockerfile,
			&repo.CreatedAt,
			&repo.UpdatedAt,
			&isWebApp,
			&webAppCheckedAt,
		)
		if err != nil {
			return nil, err
		}

		if primaryLanguage.Valid {
			repo.PrimaryLanguage = &primaryLanguage.String
		}
		if isWebApp.Valid {
			repo.IsWebApp = &isWebApp.Bool
		}
		if webAppCheckedAt.Valid {
			repo.WebAppCheckedAt = &webAppCheckedAt.Time
		}

		repositories = append(repositories, repo)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return repositories, nil
}

func (db *DB) SearchEvaluatedRepositories(searchQuery string, limit, offset int) ([]Repository, error) {
	query := `
		SELECT
			r.id, r.url, r.name_with_owner, r.stargazer_count, r.primary_language,
			r.has_dockerfile, r.created_at, r.updated_at,
			w.is_web_app, w.updated_at as web_app_checked_at
		FROM repositories r
		LEFT JOIN repository_webapp_checks w ON r.id = w.id
		WHERE r.has_dockerfile = true 
			AND w.is_web_app IS NOT NULL
			AND r.name_with_owner ILIKE $1
		ORDER BY w.updated_at DESC
		LIMIT $2 OFFSET $3
	`
	searchPattern := "%" + searchQuery + "%"
	rows, err := db.Query(query, searchPattern, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Log the error or handle it appropriately
			// For now, we'll ignore it as it's typically not critical
		}
	}()

	var repositories []Repository
	for rows.Next() {
		var repo Repository
		var primaryLanguage sql.NullString
		var isWebApp sql.NullBool
		var webAppCheckedAt sql.NullTime

		err := rows.Scan(
			&repo.ID,
			&repo.URL,
			&repo.NameWithOwner,
			&repo.StargazerCount,
			&primaryLanguage,
			&repo.HasDockerfile,
			&repo.CreatedAt,
			&repo.UpdatedAt,
			&isWebApp,
			&webAppCheckedAt,
		)
		if err != nil {
			return nil, err
		}

		if primaryLanguage.Valid {
			repo.PrimaryLanguage = &primaryLanguage.String
		}
		if isWebApp.Valid {
			repo.IsWebApp = &isWebApp.Bool
		}
		if webAppCheckedAt.Valid {
			repo.WebAppCheckedAt = &webAppCheckedAt.Time
		}

		repositories = append(repositories, repo)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return repositories, nil
}

// GetEvaluatedRepositoriesStats returns statistics for evaluated repositories
func (db *DB) GetEvaluatedRepositoriesStats() (totalCount, webAppCount, nonWebAppCount int, err error) {
	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE w.is_web_app = true) as web_apps,
			COUNT(*) FILTER (WHERE w.is_web_app = false) as non_web_apps
		FROM repositories r
		INNER JOIN repository_webapp_checks w ON r.id = w.id
		WHERE r.has_dockerfile = true AND w.is_web_app IS NOT NULL
	`
	err = db.QueryRow(query).Scan(&totalCount, &webAppCount, &nonWebAppCount)
	return
}

// GetRecentlyEvaluatedRepositories returns recently evaluated repositories ordered by evaluation date
func (db *DB) GetRecentlyEvaluatedRepositories(limit, offset int) ([]Repository, error) {
	query := `
		SELECT
			r.id, r.url, r.name_with_owner, r.stargazer_count, r.primary_language,
			r.has_dockerfile, r.created_at, r.updated_at,
			w.is_web_app, w.updated_at as web_app_checked_at
		FROM repositories r
		INNER JOIN repository_webapp_checks w ON r.id = w.id
		WHERE r.has_dockerfile = true AND w.is_web_app IS NOT NULL
		ORDER BY w.updated_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Log the error or handle it appropriately
			// For now, we'll ignore it as it's typically not critical
		}
	}()

	var repositories []Repository
	for rows.Next() {
		var repo Repository
		var primaryLanguage sql.NullString
		var isWebApp sql.NullBool
		var webAppCheckedAt sql.NullTime

		err := rows.Scan(
			&repo.ID,
			&repo.URL,
			&repo.NameWithOwner,
			&repo.StargazerCount,
			&primaryLanguage,
			&repo.HasDockerfile,
			&repo.CreatedAt,
			&repo.UpdatedAt,
			&isWebApp,
			&webAppCheckedAt,
		)
		if err != nil {
			return nil, err
		}

		if primaryLanguage.Valid {
			repo.PrimaryLanguage = &primaryLanguage.String
		}
		if isWebApp.Valid {
			repo.IsWebApp = &isWebApp.Bool
		}
		if webAppCheckedAt.Valid {
			repo.WebAppCheckedAt = &webAppCheckedAt.Time
		}

		repositories = append(repositories, repo)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return repositories, nil
}

func (db *DB) SaveSearchState(state SearchState) error {
	query := `
		INSERT INTO search_states (session_id, query, current_language, current_cursor, total_fetched, is_completed)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (session_id) DO UPDATE SET
			current_language = EXCLUDED.current_language,
			current_cursor = EXCLUDED.current_cursor,
			total_fetched = EXCLUDED.total_fetched,
			is_completed = EXCLUDED.is_completed,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, state.SessionID, state.Query, state.CurrentLanguage, state.CurrentCursor, state.TotalFetched, state.IsCompleted)
	return err
}

func (db *DB) LoadSearchState(sessionID string) (*SearchState, error) {
	query := `
		SELECT id, session_id, query, current_language, current_cursor, total_fetched, is_completed, created_at, updated_at
		FROM search_states
		WHERE session_id = $1
	`
	row := db.QueryRow(query, sessionID)

	var state SearchState
	var currentLanguage sql.NullString
	var currentCursor sql.NullString

	err := row.Scan(
		&state.ID,
		&state.SessionID,
		&state.Query,
		&currentLanguage,
		&currentCursor,
		&state.TotalFetched,
		&state.IsCompleted,
		&state.CreatedAt,
		&state.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if currentLanguage.Valid {
		state.CurrentLanguage = &currentLanguage.String
	}
	if currentCursor.Valid {
		state.CurrentCursor = &currentCursor.String
	}

	return &state, nil
}

func (db *DB) DeleteSearchState(sessionID string) error {
	query := `DELETE FROM search_states WHERE session_id = $1`
	_, err := db.Exec(query, sessionID)
	return err
}

func (db *DB) ListSearchStates() ([]SearchState, error) {
	query := `
		SELECT id, session_id, query, current_language, current_cursor, total_fetched, is_completed, created_at, updated_at
		FROM search_states
		ORDER BY updated_at DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Log the error or handle it appropriately
			// For now, we'll ignore it as it's typically not critical
		}
	}()

	var states []SearchState
	for rows.Next() {
		var state SearchState
		var currentLanguage sql.NullString
		var currentCursor sql.NullString

		err := rows.Scan(
			&state.ID,
			&state.SessionID,
			&state.Query,
			&currentLanguage,
			&currentCursor,
			&state.TotalFetched,
			&state.IsCompleted,
			&state.CreatedAt,
			&state.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if currentLanguage.Valid {
			state.CurrentLanguage = &currentLanguage.String
		}
		if currentCursor.Valid {
			state.CurrentCursor = &currentCursor.String
		}

		states = append(states, state)
	}

	return states, rows.Err()
}
