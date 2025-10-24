# Shuron CLI Tool

このCLIツールは、修論プロジェクトのGitHubリポジトリ収集や関連テーブル管理機能を統合したコマンドラインツールです。

## ビルド

```bash
go build -o shuron-cli ./cmd/cli
```

## 使用方法

### 1. GitHub リポジトリ収集

#### 新しい収集セッションを開始
```bash
./shuron-cli collect start --query="docker sort:stars-desc in:readme" --max=100
```

#### 既存セッションを再開
```bash
./shuron-cli collect resume --session-id=<SESSION_ID>
```

#### セッション一覧表示
```bash
./shuron-cli collect list
```

#### セッション削除
```bash
./shuron-cli collect delete --session-id=<SESSION_ID>
```

### 2. セットアップ

#### テーブルの初期化
```bash
./shuron-cli setup tables --db-url=$DATABASE_URL
```

#### チェッククエリの初期化
```bash
./shuron-cli setup queries --db-url=$DATABASE_URL
```

`setup tables` で作成されるテーブル一覧:

- `repositories`
- `search_states`
- `check_queries`
- `my_checked_repositories`
- `repository_webapp_checks`
- `failed_queue`

`shuron-cli collect` や GraphQL サーバーなど、既存のコマンドは実行時にこれらのテーブルを自動で確認し、存在しなければ作成します。

## 環境変数

- `GITHUB_TOKEN`: GitHub API アクセストークン（collect コマンドで必要）
- `DATABASE_URL`: データベース接続URL（オプション、`--db-url`フラグで指定可能）

以下の個別のDB設定環境変数も利用可能：
- `DB_HOST`: データベースホスト（デフォルト: localhost）
- `DB_PORT`: データベースポート（デフォルト: 5432）
- `DB_USER`: データベースユーザー名（デフォルト: user）
- `DB_PASSWORD`: データベースパスワード（デフォルト: password）
- `DB_NAME`: データベース名（デフォルト: gh-repo-research）

従来の単体コマンド（`collect_github_repos` や `setup_check_queries` など）は廃止され、現在は `shuron-cli` に統合されています。
