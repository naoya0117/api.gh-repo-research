package database

import (
	"database/sql"
	"time"
)

type CheckQuery struct {
	ID            int       `json:"id"`
	Name          string    `json:"name"`
	Description   *string   `json:"description"`
	QueryTemplate string    `json:"queryTemplate"`
	CreatedAt     time.Time `json:"createdAt"`
}

type EasyCheckedRepository struct {
	ID             int       `json:"id"`
	RepositoryID   int       `json:"repositoryId"`
	CheckQueryID   int       `json:"checkQueryId"`
	GeminiResponse *string   `json:"geminiResponse"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type BatchProgress struct {
	ID                     int        `json:"id"`
	SessionID              string     `json:"sessionId"`
	CurrentRepositoryID    *int       `json:"currentRepositoryId"`
	TotalRepositories      int        `json:"totalRepositories"`
	CompletedRepositories  int        `json:"completedRepositories"`
	Status                 string     `json:"status"`
	RateLimitResetTime     *time.Time `json:"rateLimitResetTime"`
	StartedAt              time.Time  `json:"startedAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

type RateLimitState struct {
	Kind       string     `json:"kind"`
	IsSleeping bool       `json:"isSleeping"`
	ResumeAt   *time.Time `json:"resumeAt"`
	Reason     string     `json:"reason"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type FailedItem struct {
	ID         int       `json:"id"`
	Kind       string    `json:"kind"`
	Payload    string    `json:"payload"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (db *DB) CreateCheckQueriesTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS check_queries (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			query_template TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := db.Exec(query)
	return err
}

func (db *DB) CreateEasyCheckedRepositoriesTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS easy_checked_repositories (
			id SERIAL PRIMARY KEY,
			repository_id INTEGER REFERENCES repositories(id),
			check_query_id INTEGER REFERENCES check_queries(id),
			gemini_response TEXT,
			status VARCHAR(50) DEFAULT 'pending',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(repository_id, check_query_id)
		)
	`
	_, err := db.Exec(query)
	return err
}

func (db *DB) CreateBatchProgressTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS batch_progress (
			id SERIAL PRIMARY KEY,
			session_id VARCHAR(255) UNIQUE NOT NULL,
			current_repository_id INTEGER,
			total_repositories INTEGER,
			completed_repositories INTEGER,
			status VARCHAR(50) DEFAULT 'running',
			rate_limit_reset_time TIMESTAMP,
			started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := db.Exec(query)
	return err
}

func (db *DB) GetCheckQueries() ([]CheckQuery, error) {
	query := `
		SELECT id, name, description, query_template, created_at
		FROM check_queries
		ORDER BY id
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queries []CheckQuery
	for rows.Next() {
		var q CheckQuery
		var description sql.NullString

		err := rows.Scan(
			&q.ID,
			&q.Name,
			&description,
			&q.QueryTemplate,
			&q.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if description.Valid {
			q.Description = &description.String
		}

		queries = append(queries, q)
	}

	return queries, rows.Err()
}

func (db *DB) InsertCheckQuery(name, description, queryTemplate string) error {
	query := `
		INSERT INTO check_queries (name, description, query_template)
		VALUES ($1, $2, $3)
	`
	_, err := db.Exec(query, name, description, queryTemplate)
	return err
}

func (db *DB) GetRepository(id int) (*Repository, error) {
	query := `
		SELECT id, url, name_with_owner, stargazer_count, primary_language, has_dockerfile, created_at, updated_at
		FROM repositories
		WHERE id = $1
	`
	row := db.QueryRow(query, id)

	var repo Repository
	var primaryLanguage sql.NullString

	err := row.Scan(
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
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if primaryLanguage.Valid {
		repo.PrimaryLanguage = &primaryLanguage.String
	}

	return &repo, nil
}

func (db *DB) GetUncheckedRepositories() ([]Repository, error) {
	query := `
		SELECT DISTINCT r.id, r.url, r.name_with_owner, r.stargazer_count, r.primary_language, r.has_dockerfile, r.created_at, r.updated_at
		FROM repositories r
		CROSS JOIN check_queries cq
		LEFT JOIN easy_checked_repositories ecr
			ON r.id = ecr.repository_id
			AND cq.id = ecr.check_query_id
			AND ecr.status = 'completed'
		WHERE ecr.id IS NULL
		ORDER BY r.stargazer_count DESC
	`
	rows, err := db.Query(query)
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

func (db *DB) InsertEasyCheckedRepository(repositoryID, checkQueryID int, geminiResponse, status string) error {
	query := `
		INSERT INTO easy_checked_repositories (repository_id, check_query_id, gemini_response, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (repository_id, check_query_id) DO UPDATE SET
			gemini_response = EXCLUDED.gemini_response,
			status = EXCLUDED.status,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, repositoryID, checkQueryID, geminiResponse, status)
	return err
}

func (db *DB) GetEasyCheckedRepository(repositoryID, checkQueryID int) (*EasyCheckedRepository, error) {
	query := `
		SELECT id, repository_id, check_query_id, gemini_response, status, created_at, updated_at
		FROM easy_checked_repositories
		WHERE repository_id = $1 AND check_query_id = $2
	`
	row := db.QueryRow(query, repositoryID, checkQueryID)

	var ecr EasyCheckedRepository
	var geminiResponse sql.NullString

	err := row.Scan(
		&ecr.ID,
		&ecr.RepositoryID,
		&ecr.CheckQueryID,
		&geminiResponse,
		&ecr.Status,
		&ecr.CreatedAt,
		&ecr.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if geminiResponse.Valid {
		ecr.GeminiResponse = &geminiResponse.String
	}

	return &ecr, nil
}

func (db *DB) SaveBatchProgress(progress BatchProgress) error {
	query := `
		INSERT INTO batch_progress (session_id, current_repository_id, total_repositories, completed_repositories, status, rate_limit_reset_time)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (session_id) DO UPDATE SET
			current_repository_id = EXCLUDED.current_repository_id,
			total_repositories = EXCLUDED.total_repositories,
			completed_repositories = EXCLUDED.completed_repositories,
			status = EXCLUDED.status,
			rate_limit_reset_time = EXCLUDED.rate_limit_reset_time,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, progress.SessionID, progress.CurrentRepositoryID, progress.TotalRepositories, progress.CompletedRepositories, progress.Status, progress.RateLimitResetTime)
	return err
}

func (db *DB) LoadBatchProgress(sessionID string) (*BatchProgress, error) {
	query := `
		SELECT id, session_id, current_repository_id, total_repositories, completed_repositories, status, rate_limit_reset_time, started_at, updated_at
		FROM batch_progress
		WHERE session_id = $1
	`
	row := db.QueryRow(query, sessionID)

	var progress BatchProgress
	var currentRepositoryID sql.NullInt64
	var rateLimitResetTime sql.NullTime

	err := row.Scan(
		&progress.ID,
		&progress.SessionID,
		&currentRepositoryID,
		&progress.TotalRepositories,
		&progress.CompletedRepositories,
		&progress.Status,
		&rateLimitResetTime,
		&progress.StartedAt,
		&progress.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if currentRepositoryID.Valid {
		repoID := int(currentRepositoryID.Int64)
		progress.CurrentRepositoryID = &repoID
	}

	if rateLimitResetTime.Valid {
		progress.RateLimitResetTime = &rateLimitResetTime.Time
	}

	return &progress, nil
}

func (db *DB) ListBatchProgress() ([]BatchProgress, error) {
	query := `
		SELECT id, session_id, current_repository_id, total_repositories, completed_repositories, status, rate_limit_reset_time, started_at, updated_at
		FROM batch_progress
		ORDER BY updated_at DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progressList []BatchProgress
	for rows.Next() {
		var progress BatchProgress
		var currentRepositoryID sql.NullInt64
		var rateLimitResetTime sql.NullTime

		err := rows.Scan(
			&progress.ID,
			&progress.SessionID,
			&currentRepositoryID,
			&progress.TotalRepositories,
			&progress.CompletedRepositories,
			&progress.Status,
			&rateLimitResetTime,
			&progress.StartedAt,
			&progress.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if currentRepositoryID.Valid {
			repoID := int(currentRepositoryID.Int64)
			progress.CurrentRepositoryID = &repoID
		}

		if rateLimitResetTime.Valid {
			progress.RateLimitResetTime = &rateLimitResetTime.Time
		}

		progressList = append(progressList, progress)
	}

	return progressList, rows.Err()
}

func (db *DB) CreateRateLimitStateTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS rate_limit_state (
			kind TEXT PRIMARY KEY,
			is_sleeping BOOLEAN NOT NULL DEFAULT FALSE,
			resume_at TIMESTAMPTZ,
			reason TEXT,
			updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := db.Exec(query)
	return err
}

func (db *DB) CreateFailedQueueTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS failed_queue (
			id SERIAL PRIMARY KEY,
			kind TEXT NOT NULL,
			payload JSONB NOT NULL,
			reason TEXT,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := db.Exec(query)
	return err
}

func (db *DB) GetRateLimitState(kind string) (*RateLimitState, error) {
	query := `
		SELECT kind, is_sleeping, resume_at, reason, updated_at
		FROM rate_limit_state
		WHERE kind = $1
	`
	row := db.QueryRow(query, kind)

	var state RateLimitState
	var resumeAt sql.NullTime
	var reason sql.NullString

	err := row.Scan(
		&state.Kind,
		&state.IsSleeping,
		&resumeAt,
		&reason,
		&state.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if resumeAt.Valid {
		state.ResumeAt = &resumeAt.Time
	}
	if reason.Valid {
		state.Reason = reason.String
	}

	return &state, nil
}

func (db *DB) SetRateLimitState(state RateLimitState) error {
	query := `
		INSERT INTO rate_limit_state (kind, is_sleeping, resume_at, reason, updated_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (kind) DO UPDATE SET
			is_sleeping = EXCLUDED.is_sleeping,
			resume_at = EXCLUDED.resume_at,
			reason = EXCLUDED.reason,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, state.Kind, state.IsSleeping, state.ResumeAt, state.Reason)
	return err
}

func (db *DB) EnqueueFailedItem(item FailedItem) error {
	query := `
		INSERT INTO failed_queue (kind, payload, reason, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err := db.Exec(query, item.Kind, item.Payload, item.Reason, item.CreatedAt)
	return err
}

func (db *DB) DequeueFailedItems(kind string, limit int) ([]FailedItem, error) {
	query := `
		SELECT id, kind, payload, reason, created_at
		FROM failed_queue
		WHERE kind = $1
		ORDER BY created_at ASC
		LIMIT $2
	`
	rows, err := db.Query(query, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []FailedItem
	for rows.Next() {
		var item FailedItem
		err := rows.Scan(
			&item.ID,
			&item.Kind,
			&item.Payload,
			&item.Reason,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (db *DB) DeleteFailedItem(id int) error {
	query := `DELETE FROM failed_queue WHERE id = $1`
	_, err := db.Exec(query, id)
	return err
}

func (db *DB) ClearFailedItems(kind string) error {
	query := `DELETE FROM failed_queue WHERE kind = $1`
	_, err := db.Exec(query, kind)
	return err
}