package database

import (
	"database/sql"
	"fmt"
	"time"
)

type CheckQuery struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

// K8sPattern represents a Kubernetes pattern
type K8sPattern struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

// CheckItem represents a specific check item for a pattern
type CheckItem struct {
	ID          int       `json:"id"`
	PatternID   int       `json:"patternId"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

// CheckResult represents the evaluation result for a repository check item
type CheckResult struct {
	ID           int       `json:"id"`
	RepositoryID int       `json:"repositoryId"`
	CheckItemID  int       `json:"checkItemId"`
	Result       bool      `json:"result"`
	Memo         *string   `json:"memo"`
	CheckedAt    time.Time `json:"checkedAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type MyCheckSummary struct {
	RepositoryID   int       `json:"repositoryId"`
	RepositoryName string    `json:"repositoryName"`
	CheckQueryID   int       `json:"checkQueryId"`
	CheckQueryName string    `json:"checkQueryName"`
	Result         string    `json:"result"`
	Memo           *string   `json:"memo"`
	UpdatedAt      time.Time `json:"updatedAt"`
	IsWebApp       *bool     `json:"isWebApp"`
}

type FailedItem struct {
	ID        int       `json:"id"`
	Kind      string    `json:"kind"`
	Payload   string    `json:"payload"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

var CoreTableNames = []string{
	"repositories",
	"search_states",
	"check_queries",
	"my_checked_repositories",
	"repository_webapp_checks",
	"failed_queue",
}

func (db *DB) CreateCheckQueriesTable() error {
	query := `
                CREATE TABLE IF NOT EXISTS check_queries (
                        id SERIAL PRIMARY KEY,
                        name VARCHAR(255) NOT NULL,
                        description TEXT,
                        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
                )
        `
	_, err := db.Exec(query)
	return err
}

func (db *DB) CreateMyCheckedRepositoriesTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS my_checked_repositories (
			id SERIAL PRIMARY KEY,
			repository_id INTEGER REFERENCES repositories(id),
			check_query_id INTEGER REFERENCES check_queries(id),
			memo TEXT,
			result TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(repository_id, check_query_id)
		)
	`
	_, err := db.Exec(query)
	return err
}

func (db *DB) CreateRepositoryWebAppChecksTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS repository_webapp_checks (
			id INTEGER PRIMARY KEY REFERENCES repositories(id) ON DELETE CASCADE,
			is_web_app BOOLEAN,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := db.Exec(query)
	return err
}

func (db *DB) GetCheckQueries() ([]CheckQuery, error) {
	query := `
                SELECT id, name, description, created_at
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

func (db *DB) InsertCheckQuery(name string, description *string) (int, error) {
	query := `
                INSERT INTO check_queries (name, description)
                VALUES ($1, $2)
                RETURNING id
        `
	var id int
	if err := db.QueryRow(query, name, description).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (db *DB) GetCheckQuery(id int) (*CheckQuery, error) {
	query := `
                SELECT id, name, description, created_at
                FROM check_queries
                WHERE id = $1
        `
	row := db.QueryRow(query, id)

	var q CheckQuery
	var description sql.NullString
	if err := row.Scan(&q.ID, &q.Name, &description, &q.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if description.Valid {
		q.Description = &description.String
	}

	return &q, nil
}

func (db *DB) UpsertMyCheckedRepository(repositoryID, checkQueryID int, result string, memo *string) error {
	query := `
                INSERT INTO my_checked_repositories (repository_id, check_query_id, result, memo)
                VALUES ($1, $2, $3, $4)
                ON CONFLICT (repository_id, check_query_id) DO UPDATE SET
                        result = EXCLUDED.result,
                        memo = EXCLUDED.memo,
                        updated_at = CURRENT_TIMESTAMP
        `
	_, err := db.Exec(query, repositoryID, checkQueryID, result, memo)
	return err
}

func (db *DB) UpsertRepositoryWebAppCheck(repositoryID int, isWebApp bool) error {
	query := `
                INSERT INTO repository_webapp_checks (id, is_web_app)
                VALUES ($1, $2)
                ON CONFLICT (id) DO UPDATE SET
                        is_web_app = EXCLUDED.is_web_app,
                        updated_at = CURRENT_TIMESTAMP
        `
	_, err := db.Exec(query, repositoryID, isWebApp)
	return err
}

func (db *DB) ListMyCheckSummaries(limit int) ([]MyCheckSummary, error) {
	query := `
                SELECT
                        m.repository_id,
                        r.name_with_owner,
                        m.check_query_id,
                        q.name,
                        m.result,
                        m.memo,
                        m.updated_at,
                        w.is_web_app
                FROM my_checked_repositories m
                JOIN repositories r ON r.id = m.repository_id
                JOIN check_queries q ON q.id = m.check_query_id
                LEFT JOIN repository_webapp_checks w ON w.id = m.repository_id
                ORDER BY m.updated_at DESC
                LIMIT $1
        `
	rows, err := db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []MyCheckSummary
	for rows.Next() {
		var summary MyCheckSummary
		var memo sql.NullString
		var isWebApp sql.NullBool

		if err := rows.Scan(
			&summary.RepositoryID,
			&summary.RepositoryName,
			&summary.CheckQueryID,
			&summary.CheckQueryName,
			&summary.Result,
			&memo,
			&summary.UpdatedAt,
			&isWebApp,
		); err != nil {
			return nil, err
		}

		if memo.Valid {
			summary.Memo = &memo.String
		}
		if isWebApp.Valid {
			summary.IsWebApp = &isWebApp.Bool
		}

		summaries = append(summaries, summary)
	}

	return summaries, rows.Err()
}

func (db *DB) GetRepository(id int) (*Repository, error) {
	query := `
		SELECT
			r.id, r.url, r.name_with_owner, r.stargazer_count, r.primary_language,
			r.has_dockerfile, r.created_at, r.updated_at,
			w.is_web_app, w.updated_at as web_app_checked_at
		FROM repositories r
		LEFT JOIN repository_webapp_checks w ON r.id = w.id
		WHERE r.id = $1
	`
	row := db.QueryRow(query, id)

	var repo Repository
	var primaryLanguage sql.NullString
	var isWebApp sql.NullBool
	var webAppCheckedAt sql.NullTime

	err := row.Scan(
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
		if err == sql.ErrNoRows {
			return nil, nil
		}
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

	return &repo, nil
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

func (db *DB) EnsureCoreTables() error {
	creators := []struct {
		name string
		fn   func() error
	}{
		{"repositories", db.CreateRepositoriesTable},
		{"search_states", db.CreateSearchStatesTable},
		{"check_queries", db.CreateCheckQueriesTable},
		{"my_checked_repositories", db.CreateMyCheckedRepositoriesTable},
		{"repository_webapp_checks", db.CreateRepositoryWebAppChecksTable},
		{"failed_queue", db.CreateFailedQueueTable},
	}

	for _, creator := range creators {
		if err := creator.fn(); err != nil {
			return fmt.Errorf("create %s table: %w", creator.name, err)
		}
	}
	return nil
}

// ===== K8s Patterns CRUD =====

// GetK8sPatterns retrieves all patterns
func (db *DB) GetK8sPatterns() ([]K8sPattern, error) {
	query := `
		SELECT id, name, description, created_at
		FROM k8s_patterns
		ORDER BY id
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []K8sPattern
	for rows.Next() {
		var p K8sPattern
		var description sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &description, &p.CreatedAt); err != nil {
			return nil, err
		}
		if description.Valid {
			p.Description = &description.String
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}

// GetK8sPattern retrieves a single pattern by ID
func (db *DB) GetK8sPattern(id int) (*K8sPattern, error) {
	query := `
		SELECT id, name, description, created_at
		FROM k8s_patterns
		WHERE id = $1
	`
	var p K8sPattern
	var description sql.NullString
	err := db.QueryRow(query, id).Scan(&p.ID, &p.Name, &description, &p.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if description.Valid {
		p.Description = &description.String
	}
	return &p, nil
}

// CreateK8sPattern creates a new pattern
func (db *DB) CreateK8sPattern(name string, description *string) (int, error) {
	query := `
		INSERT INTO k8s_patterns (name, description)
		VALUES ($1, $2)
		RETURNING id
	`
	var id int
	err := db.QueryRow(query, name, description).Scan(&id)
	return id, err
}

// UpdateK8sPattern updates an existing pattern
func (db *DB) UpdateK8sPattern(id int, name string, description *string) error {
	query := `
		UPDATE k8s_patterns
		SET name = $2, description = $3
		WHERE id = $1
	`
	_, err := db.Exec(query, id, name, description)
	return err
}

// DeleteK8sPattern deletes a pattern (cascade deletes check items)
func (db *DB) DeleteK8sPattern(id int) error {
	query := `DELETE FROM k8s_patterns WHERE id = $1`
	_, err := db.Exec(query, id)
	return err
}

// ===== Check Items CRUD =====

// GetCheckItems retrieves check items, optionally filtered by pattern_id
func (db *DB) GetCheckItems(patternID *int) ([]CheckItem, error) {
	var query string
	var args []interface{}

	if patternID != nil {
		query = `
			SELECT id, pattern_id, name, description, created_at
			FROM check_items
			WHERE pattern_id = $1
			ORDER BY id
		`
		args = append(args, *patternID)
	} else {
		query = `
			SELECT id, pattern_id, name, description, created_at
			FROM check_items
			ORDER BY pattern_id, id
		`
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CheckItem
	for rows.Next() {
		var item CheckItem
		var description sql.NullString
		if err := rows.Scan(&item.ID, &item.PatternID, &item.Name, &description, &item.CreatedAt); err != nil {
			return nil, err
		}
		if description.Valid {
			item.Description = &description.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// CreateCheckItem creates a new check item
func (db *DB) CreateCheckItem(patternID int, name string, description *string) (int, error) {
	query := `
		INSERT INTO check_items (pattern_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id
	`
	var id int
	err := db.QueryRow(query, patternID, name, description).Scan(&id)
	return id, err
}

// UpdateCheckItem updates an existing check item
func (db *DB) UpdateCheckItem(id int, patternID int, name string, description *string) error {
	query := `
		UPDATE check_items
		SET pattern_id = $2, name = $3, description = $4
		WHERE id = $1
	`
	_, err := db.Exec(query, id, patternID, name, description)
	return err
}

// DeleteCheckItem deletes a check item
func (db *DB) DeleteCheckItem(id int) error {
	query := `DELETE FROM check_items WHERE id = $1`
	_, err := db.Exec(query, id)
	return err
}

// ===== Check Results CRUD =====

// GetCheckResults retrieves check results, optionally filtered by repository_id
func (db *DB) GetCheckResults(repositoryID *int) ([]CheckResult, error) {
	var query string
	var args []interface{}

	if repositoryID != nil {
		query = `
			SELECT id, repository_id, check_item_id, result, memo, checked_at, updated_at
			FROM check_results
			WHERE repository_id = $1
			ORDER BY id
		`
		args = append(args, *repositoryID)
	} else {
		query = `
			SELECT id, repository_id, check_item_id, result, memo, checked_at, updated_at
			FROM check_results
			ORDER BY id
		`
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CheckResult
	for rows.Next() {
		var r CheckResult
		var memo sql.NullString
		if err := rows.Scan(&r.ID, &r.RepositoryID, &r.CheckItemID, &r.Result, &memo, &r.CheckedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if memo.Valid {
			r.Memo = &memo.String
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// CreateCheckResult creates a new check result
func (db *DB) CreateCheckResult(repositoryID, checkItemID int, result bool, memo *string) (int, error) {
	query := `
		INSERT INTO check_results (repository_id, check_item_id, result, memo)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (repository_id, check_item_id) DO UPDATE
		SET result = EXCLUDED.result, memo = EXCLUDED.memo, updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`
	var id int
	err := db.QueryRow(query, repositoryID, checkItemID, result, memo).Scan(&id)
	return id, err
}

// UpdateCheckResult updates an existing check result
func (db *DB) UpdateCheckResult(id int, result bool, memo *string) error {
	query := `
		UPDATE check_results
		SET result = $2, memo = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`
	_, err := db.Exec(query, id, result, memo)
	return err
}

// DeleteCheckResult deletes a check result
func (db *DB) DeleteCheckResult(id int) error {
	query := `DELETE FROM check_results WHERE id = $1`
	_, err := db.Exec(query, id)
	return err
}
