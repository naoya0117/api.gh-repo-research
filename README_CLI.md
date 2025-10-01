# Shuron CLI Tool

このCLIツールは、修論プロジェクトのGitHubリポジトリ収集・分析機能を統合したコマンドラインツールです。

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

### 2. バッチ分析

#### 新しいバッチ分析を開始
```bash
./shuron-cli batch start --work-dir=tmp_repositories --db-url=$DATABASE_URL
```

#### バッチ分析を再開
```bash
./shuron-cli batch resume --session-id=<SESSION_ID>
```

#### バッチ分析状況を確認
```bash
./shuron-cli batch status
```

### 3. セットアップ

#### チェッククエリの初期化
```bash
./shuron-cli setup queries --db-url=$DATABASE_URL
```

## 環境変数

- `GITHUB_TOKEN`: GitHub API アクセストークン（collect コマンドで必要）
- `DATABASE_URL`: データベース接続URL（オプション、`--db-url`フラグで指定可能）

以下の個別のDB設定環境変数も利用可能：
- `DB_HOST`: データベースホスト（デフォルト: localhost）
- `DB_PORT`: データベースポート（デフォルト: 5432）
- `DB_USER`: データベースユーザー名（デフォルト: user）
- `DB_PASSWORD`: データベースパスワード（デフォルト: password）
- `DB_NAME`: データベース名（デフォルト: gh-repo-research）

## 従来のコマンドとの対応

| 従来のコマンド | 新しいコマンド |
|---|---|
| `./collect_github_repos` | `./shuron-cli collect start` |
| `./collect_github_repos --session=<ID>` | `./shuron-cli collect resume --session-id=<ID>` |
| `./collect_github_repos --list` | `./shuron-cli collect list` |
| `./collect_github_repos --delete=<ID>` | `./shuron-cli collect delete --session-id=<ID>` |
| `./batch_analyzer --start` | `./shuron-cli batch start` |
| `./batch_analyzer --resume --session-id=<ID>` | `./shuron-cli batch resume --session-id=<ID>` |
| `./batch_analyzer --status` | `./shuron-cli batch status` |
| `./setup_check_queries` | `./shuron-cli setup queries` |