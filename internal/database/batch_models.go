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
