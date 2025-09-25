package database

import (
	"database/sql"
	"time"
)

type Repository struct {
	ID              int          `json:"id"`
	URL             string       `json:"url"`
	NameWithOwner   string       `json:"nameWithOwner"`
	StargazerCount  int          `json:"stargazerCount"`
	PrimaryLanguage *string      `json:"primaryLanguage"`
	HasDockerfile   bool         `json:"hasDockerfile"`
	CreatedAt       time.Time    `json:"createdAt"`
	UpdatedAt       time.Time    `json:"updatedAt"`
}

type SearchState struct {
	ID            int       `json:"id"`
	SessionID     string    `json:"sessionId"`
	Query         string    `json:"query"`
	CurrentCursor *string   `json:"currentCursor"`
	TotalFetched  int       `json:"totalFetched"`
	IsCompleted   bool      `json:"isCompleted"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
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
		SELECT id, url, name_with_owner, stargazer_count, primary_language, has_dockerfile, created_at, updated_at
		FROM repositories
		ORDER BY stargazer_count DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repositories []Repository
	for rows.Next() {
		var repo Repository
		var primaryLanguage sql.NullString
		
		err := rows.Scan(
			&repo.ID,
			&repo.URL,
			&repo.NameWithOwner,
			&repo.StargazerCount,
			&primaryLanguage,
			&repo.HasDockerfile,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		
		if primaryLanguage.Valid {
			repo.PrimaryLanguage = &primaryLanguage.String
		}
		
		repositories = append(repositories, repo)
	}

	return repositories, rows.Err()
}

func (db *DB) SaveSearchState(state SearchState) error {
	query := `
		INSERT INTO search_states (session_id, query, current_cursor, total_fetched, is_completed)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (session_id) DO UPDATE SET
			current_cursor = EXCLUDED.current_cursor,
			total_fetched = EXCLUDED.total_fetched,
			is_completed = EXCLUDED.is_completed,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, state.SessionID, state.Query, state.CurrentCursor, state.TotalFetched, state.IsCompleted)
	return err
}

func (db *DB) LoadSearchState(sessionID string) (*SearchState, error) {
	query := `
		SELECT id, session_id, query, current_cursor, total_fetched, is_completed, created_at, updated_at
		FROM search_states
		WHERE session_id = $1
	`
	row := db.QueryRow(query, sessionID)

	var state SearchState
	var currentCursor sql.NullString

	err := row.Scan(
		&state.ID,
		&state.SessionID,
		&state.Query,
		&currentCursor,
		&state.TotalFetched,
		&state.IsCompleted,
		&state.CreatedAt,
		&state.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
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
		SELECT id, session_id, query, current_cursor, total_fetched, is_completed, created_at, updated_at
		FROM search_states
		ORDER BY updated_at DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []SearchState
	for rows.Next() {
		var state SearchState
		var currentCursor sql.NullString

		err := rows.Scan(
			&state.ID,
			&state.SessionID,
			&state.Query,
			&currentCursor,
			&state.TotalFetched,
			&state.IsCompleted,
			&state.CreatedAt,
			&state.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if currentCursor.Valid {
			state.CurrentCursor = &currentCursor.String
		}

		states = append(states, state)
	}

	return states, rows.Err()
}