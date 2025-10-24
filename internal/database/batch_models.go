package database

import (
	"database/sql"
	"fmt"
	"time"
)

type CheckQuery struct {
	ID            int       `json:"id"`
	Name          string    `json:"name"`
	Description   *string   `json:"description"`
	QueryTemplate string    `json:"queryTemplate"`
	CreatedAt     time.Time `json:"createdAt"`
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
			query_template TEXT NOT NULL,
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
