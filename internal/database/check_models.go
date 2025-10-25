package database

import (
	"database/sql"
	"time"
)

// K8sPattern represents a Kubernetes pattern (e.g., Health Probe, Managed Lifecycle)
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

// CheckResult represents a check result for a repository
type CheckResult struct {
	ID           int       `json:"id"`
	RepositoryID int       `json:"repositoryId"`
	CheckItemID  int       `json:"checkItemId"`
	Result       bool      `json:"result"`
	Memo         *string   `json:"memo"`
	CheckedAt    time.Time `json:"checkedAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// GetK8sPatterns retrieves all K8s patterns
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

// GetCheckItems retrieves check items, optionally filtered by pattern ID
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
		args = []interface{}{*patternID}
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

// GetCheckResults retrieves check results, optionally filtered by repository ID
func (db *DB) GetCheckResults(repositoryID *int) ([]CheckResult, error) {
	var query string
	var args []interface{}

	if repositoryID != nil {
		query = `
			SELECT id, repository_id, check_item_id, result, memo, checked_at, updated_at
			FROM check_results
			WHERE repository_id = $1
			ORDER BY updated_at DESC
		`
		args = []interface{}{*repositoryID}
	} else {
		query = `
			SELECT id, repository_id, check_item_id, result, memo, checked_at, updated_at
			FROM check_results
			ORDER BY updated_at DESC
			LIMIT 100
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
		ON CONFLICT (repository_id, check_item_id) DO UPDATE SET
			result = EXCLUDED.result,
			memo = EXCLUDED.memo,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`
	var id int
	if err := db.QueryRow(query, repositoryID, checkItemID, result, memo).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateCheckResult updates an existing check result
func (db *DB) UpdateCheckResult(id, repositoryID, checkItemID int, result bool, memo *string) error {
	query := `
		UPDATE check_results
		SET repository_id = $2, check_item_id = $3, result = $4, memo = $5, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`
	_, err := db.Exec(query, id, repositoryID, checkItemID, result, memo)
	return err
}

// DeleteCheckResult deletes a check result
func (db *DB) DeleteCheckResult(id int) error {
	query := `DELETE FROM check_results WHERE id = $1`
	_, err := db.Exec(query, id)
	return err
}
