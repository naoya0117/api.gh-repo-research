-- Migration: Redesign check query tables to separate patterns and check items
-- Date: 2025-10-25

BEGIN;

-- 1. Create k8s_patterns table
CREATE TABLE k8s_patterns (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 2. Create check_items table
CREATE TABLE check_items (
    id SERIAL PRIMARY KEY,
    pattern_id INTEGER NOT NULL REFERENCES k8s_patterns(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(pattern_id, name)
);

-- 3. Create check_results table (replaces my_checked_repositories)
CREATE TABLE check_results (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    check_item_id INTEGER NOT NULL REFERENCES check_items(id) ON DELETE CASCADE,
    result BOOLEAN NOT NULL,
    memo TEXT,
    checked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(repository_id, check_item_id)
);

-- 4. (Data seeding intentionally omitted to keep tables empty)

-- 6. Drop old tables (if they exist and are empty or migration is confirmed)
-- Note: Uncomment these lines after confirming data migration is successful
-- DROP TABLE IF EXISTS my_checked_repositories CASCADE;
-- DROP TABLE IF EXISTS check_queries CASCADE;

COMMIT;
