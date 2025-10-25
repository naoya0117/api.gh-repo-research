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

-- 4. Insert the 6 Kubernetes patterns
INSERT INTO k8s_patterns (name, description) VALUES
    ('Health Probe', 'Readiness/Liveness Probe専用のエンドポイントを実装'),
    ('Managed Lifecycle', 'グレースフルシャットダウンを実現'),
    ('Self Awareness', 'アプリケーションは自身に関する情報を持たない'),
    ('Sidecar, Adapter, Ambassador', '設定の外部化'),
    ('Process Containment', 'ファイルシステムへの書き込みはせず、実行権限は最小限'),
    ('Stateless Service', 'コンテナはステートレスである');

-- 5. Insert check items for each pattern
-- Health Probe (pattern_id = 1)
INSERT INTO check_items (pattern_id, name, description) VALUES
    (1, 'Readiness Probe実装', 'DB、外部APIなどの依存関係をチェックするエンドポイントがあるか'),
    (1, 'Liveness Probe実装', '軽量なヘルスチェックエンドポイント（外部依存なし）があるか');

-- Managed Lifecycle (pattern_id = 2)
INSERT INTO check_items (pattern_id, name, description) VALUES
    (2, 'SIGTERMハンドリング', 'SIGTERMをハンドリングし正常にシャットダウンできるか（フレームワーク機能含む）');

-- Self Awareness (pattern_id = 3)
INSERT INTO check_items (pattern_id, name, description) VALUES
    (3, 'IP/ホスト名の管理', 'IPアドレス、ホスト名は環境変数経由で提供されているか（ハードコードされていないか）');

-- Sidecar, Adapter, Ambassador (pattern_id = 4)
INSERT INTO check_items (pattern_id, name, description) VALUES
    (4, '設定の外部化', '設定は環境変数または設定ファイルに分離されているか（ハードコードされていないか）');

-- Process Containment (pattern_id = 5)
INSERT INTO check_items (pattern_id, name, description) VALUES
    (5, 'ログ出力方法', 'ログは標準ストリーム（stdout/stderr）に出力しているか'),
    (5, 'ファイルシステム書き込み', 'ファイルシステムへの書き込みを最小化しているか'),
    (5, '非ルートユーザ実行', 'コンテナは非ルートユーザで実行されているか');

-- Stateless Service (pattern_id = 6)
INSERT INTO check_items (pattern_id, name, description) VALUES
    (6, 'データ永続化方法', '永続データは名前付きボリューム、DBなどに保存しているか（コンテナ内保存していないか）');

-- 6. Drop old tables (if they exist and are empty or migration is confirmed)
-- Note: Uncomment these lines after confirming data migration is successful
-- DROP TABLE IF EXISTS my_checked_repositories CASCADE;
-- DROP TABLE IF EXISTS check_queries CASCADE;

COMMIT;
