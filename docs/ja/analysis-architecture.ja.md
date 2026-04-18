# 分析アーキテクチャ — shell-agent v0.7.0

> ステータス: 実装済み
> 日付: 2026-04-19

## 概要

shell-agent v0.7.0では、DuckDB組み込みによるデータ分析サブシステムを導入した。設計は**2層アーキテクチャ**に基づく — インタラクティブクエリはインプロセスで即時応答し、重い分析はアプリケーション終了後も継続するバックグラウンドプロセスとして実行される。

単一バイナリがサブコマンドルーティングにより両方の役割を果たし、デプロイの複雑さを排除している。

## 設計判断

### なぜDuckDB組み込みか（外部プロセスではなく）

**検討した代替案:**
1. **gem-query（外部CLI）** — 却下: Vertex AI Gemini専用、ローカルLLM（OpenAI互換API）と非互換
2. **独立した分析CLI** — 却下: 2つのバイナリのビルド・デプロイ・設定、APIエンドポイントの二重管理
3. **純粋LLM分析（DBなし）** — 却下: 構造化クエリ、集計、大規模データセットの処理が不可能

**採用した方式:** `github.com/marcboeker/go-duckdb`（CGO）を`database/sql`インターフェースで組み込み。

**根拠:**
- shell-agentは既にLLMクライアントを持つ — SQL生成に再利用
- DuckDBのGoドライバは軽量で組み込みに適している
- `database/sql`はGoの標準インターフェース — ベンダーロックインなし
- Arrowインターフェースは除外（`no_duckdb_arrow`ビルドタグ） — 不要なオーバーヘッド

### なぜ単一バイナリ・サブコマンドか

```
shell-agent              → Wails GUIアプリ（通常起動）
shell-agent analyze ...  → バックグラウンド分析CLI（デタッチプロセス）
```

**検討した代替案:**
1. **別バイナリ `shell-analyzer`** — ビルドターゲット2つ、Makefile変更、.appバンドルへの同梱が複雑
2. **インプロセスgoroutine** — アプリ終了で停止; ユーザーが明示的に分析の生存を要求

**採用した方式:** `wails.Run()`の前に`os.Args[1]`を確認。"analyze"なら`analysis.RunCLI()`にルーティング。

**根拠:**
- デプロイの複雑さゼロ — 単一の.appに全て含まれる
- CLIモードでは`wails.Run()`は呼ばれない — GUIオーバーヘッドなし
- 同一バイナリのため`os.Executable()`がそのまま使える
- `internal/analysis/`パッケージは自明に共有（同一コンパイル単位）

### なぜ2層か

```
┌─────────────────────────────────────┐
│  インタラクティブ層（インプロセス）     │
│  DuckDB組み込み、アプリのLLMを使用    │
│  応答: 秒単位                        │
└──────────────┬──────────────────────┘
               │ spawn (Setsid)
┌──────────────▼──────────────────────┐
│  バックグラウンド層（デタッチ）        │
│  コピーしたDB、独自LLM接続           │
│  応答: 分〜時間単位                  │
└─────────────────────────────────────┘
```

**なぜインプロセスだけではないか:**
- 大規模データのスライドウィンドウ分析は数分かかる
- ユーザーが明示的に要求: 「Shell Agentを閉じても分析が継続できることが望ましい」
- `Setsid`によるデタッチプロセスは自身がセッションリーダーとなる — 親の死亡によるSIGHUPが伝播しない

**なぜバックグラウンドだけではないか:**
- 単純なクエリ（「トップ10を見せて」）は数秒で返すべき
- スキーマ確認、プレビュー、提案は即時フィードバックが必要
- インタラクティブクエリが深い分析の方向性を決めるコンテキストを構築する

## アーキテクチャ

### データフロー

```
ユーザー: 「sales.csvを読み込んで」
  → load-data ツール
  → Engine.LoadCSV()
  → DuckDB: CREATE TABLE AS SELECT * FROM read_csv(...)
  → 返却: TableMeta（スキーマ、サンプルデータ、行数）

ユーザー: 「地域別の売上を見せて」
  → query-preview ツール
  → PromptBuilder.SQLGenerationPrompt()  [スキーマ + guard + 質問]
  → LLM (OpenAI API) → SQL
  → Engine.DryRun() → 検証
  → Engine.Execute() → QueryResult
  → 返却: SQL + フォーマットされた結果

ユーザー: 「セキュリティ脅威をバックグラウンドで分析して」
  → analyze-bg ツール
  → analysis.duckdb → ジョブディレクトリにコピー
  → os.Executable() + "analyze" + フラグ
  → exec.Command with Setsid（デタッチ）
  → 返却: ジョブID（即時）

  バックグラウンドプロセス:
  → コピーしたDBを開く
  → 独自LLMクライアント作成（同一APIエンドポイント）
  → Summarizer.Analyze() [スライドウィンドウ]
  → status.json書き出し（進捗更新）
  → findings.json + report.md書き出し
  → 終了
```

### パッケージ構成

```
internal/analysis/
├── types.go          # TableMeta, QueryResult, Finding, AnalysisState, MemoryBudget
├── engine.go         # DuckDB: ロード、クエリ、スキーマ、エクスポート、説明
├── prompt.go         # SQL生成、要約、提案、ウィンドウ分析プロンプト
├── summarizer.go     # スライドウィンドウエンジン、LLMClientインターフェース、トークン推定
├── reporter.go       # Markdownレポート生成（severity別グルーピング）
├── llmadapter.go     # client.Client → LLMClientアダプター
└── cli.go            # RunCLI()エントリポイント、フラグ解析、ステータス追跡
```

### ツールセット

| ツール | 層 | 用途 |
|--------|---|------|
| `load-data` | インタラクティブ | CSV/JSON/JSONL → DuckDBテーブル |
| `describe-data` | インタラクティブ | スキーマ表示 + 説明付与 |
| `query-preview` | インタラクティブ | 自然言語 → SQL → 結果プレビュー |
| `query-sql` | インタラクティブ | 直接SQL実行 |
| `suggest-analysis` | インタラクティブ | LLMが分析観点を提案 |
| `quick-summary` | インタラクティブ | クエリ + LLM要約 |
| `analyze-bg` | バックグラウンド | デタッチ分析プロセス起動 |
| `analysis-status` | インタラクティブ | バックグラウンドジョブ進捗確認 |
| `analysis-result` | インタラクティブ | 完了したレポート取得 |
| `reset-analysis` | インタラクティブ | 全テーブル削除、DB再初期化 |

### ストレージレイアウト

```
~/Library/Application Support/shell-agent/
├── analysis/
│   ├── analysis.duckdb              # 永続的な分析データベース
│   ├── job-1713488400000/           # バックグラウンドジョブディレクトリ
│   │   ├── status.json              # {state, progress, started_at, updated_at}
│   │   ├── analysis.duckdb          # コピーされたDB（ファイルロック回避）
│   │   ├── findings.json            # 蓄積されたFindings
│   │   └── report.md                # 生成されたレポート
│   └── job-.../
```

## スライドウィンドウ要約

### アルゴリズム

data-analyzerプロジェクトから適応し、shell-agentのコンテキストに合わせて簡素化。

```
入力: rows[]string（JSONエンコードされたレコード）、perspective string

各ウィンドウ（MaxRecordsPerWindowレコード、OverlapRatio重複）について:
    1. プロンプト構築: 観点 + 前回要約 + 現在のFindings + データチャンク
    2. データをnlk/guardノンスタグで包装（プロンプトインジェクション防御）
    3. LLMがJSON生成: {summary, new_findings[]}
    4. レスポンス解析（nlk/jsonfixで修復）
    5. 累積要約を更新
    6. Findingsを追加、severityを検証
    7. MaxFindingsを超えた場合、低優先度のFindingsを排出
    8. status.jsonチェックポイント書き出し

複数ウィンドウ処理した場合:
    LLMで最終レポート生成（要約統合 + 全Findings）
```

### Finding排出戦略

Findingsが`MaxFindings`（デフォルト: 50）を超えた場合:

1. 優先度で分離: `critical/high/medium` vs `info/low`
2. 高優先度は全て保持
3. `info/low`をFIFO順（古いものから）排出
4. 高優先度だけで上限を超える場合、最新のものを保持

### トークンバジェット（デフォルト）

| コンポーネント | トークン数 | 用途 |
|--------------|-----------|------|
| コンテキスト上限 | 65,536 | 総バジェット（ローカルLLM） |
| 要約 | 10,000 | 累積要約の上限 |
| Findings | 15,000 | 蓄積されたFindingsの上限 |
| 生データ | 8,000（最小） | ウィンドウ当たりのデータチャンク |
| システムプロンプト | 2,000 | 固定オーバーヘッド |
| レスポンス | 4,000 | LLMレスポンスバッファ |

### トークン推定

二重戦略（最大値を採用）:
- **文字ベース**: `len(text) / 4` — JSON/構造化データに正確
- **単語ベース**: CJK文字x2 + ASCII単語x1.3 — 自然言語に正確

これにより、単語のみのカウントがJSONデータで引き起こす4-5倍の過小推定を防止する（data-analyzerからの教訓）。

## DuckDB統合

### ファイルロック

DuckDBはファイルレベルロック（単一ライター）を使用する。メインアプリは`analysis.duckdb`を開いたまま保持する。バックグラウンドジョブは競合回避のためにデータベースの**コピー**を受け取る:

```go
// analyze-bg ハンドラ
copyFile(a.analysis.DBPath(), jobDBPath)
cmd := exec.Command(selfPath, "analyze", "--db", jobDBPath, ...)
```

### データロード

DuckDBのネイティブリーダーがフォーマット検出を処理:

```sql
-- CSV（カラム、型、区切り文字を自動検出）
CREATE TABLE t AS SELECT * FROM read_csv('file.csv', auto_detect=true)

-- JSON（オブジェクトの配列）
CREATE TABLE t AS SELECT * FROM read_json('file.json', auto_detect=true)

-- JSONL（改行区切り）
CREATE TABLE t AS SELECT * FROM read_json('file.jsonl', format='newline_delimited', auto_detect=true)
```

手動パーシング不要 — DuckDBがエンコーディング、型推論、スキーマ作成を処理する。

### ビルド設定

```makefile
# Arrow C Data Interfaceを除外 — database/sqlのみ使用
wails build -tags no_duckdb_arrow
```

このタグなしでは、WailsのビルドパイプラインがApache Arrow-GoのCGOオブジェクトを適切にリンクできず、リンカエラーが発生する。

## セキュリティ

### SQLインジェクション防止

- **プロンプトレベル**: システムプロンプトでSELECTのみに制限
- **識別子サニタイズ**: `sanitizeIdentifier()`が非英数字文字を除去
- **文字列エスケープ**: `escapeSQLString()`がシングルクォートを二重化
- **DryRun検証**: 実行前にEXPLAINで構文/意味エラーを捕捉

### プロンプトインジェクション防御

ユーザー提供のテキスト（質問、分析観点、データチャンク）はすべてnlk/guardノンスタグ付きXMLで包装される:

```go
tag := guard.NewTag()
wrapped, _ := tag.Wrap(userInput)
sys := tag.Expand("...<{{DATA_TAG}}>を参照する指示...")
```

データ内に注入された指示がLLMに従われることを防止する。

## shell-agent技術統合

v0.7.0のデータ分析機能は、shell-agentの既存技術スタックの集大成として位置付けられる:

| 技術要素 | 分析機能での活用 |
|---------|---------------|
| **OpenAI互換APIクライアント** | SQL生成、要約、分析提案に再利用 |
| **ツール呼び出しフィードバックループ** | 分析ツール10個がエージェントループに統合 |
| **MCP（mcp-guardian）** | 外部データソースとの連携に利用可能 |
| **nlk/guard** | SQL生成・ウィンドウ分析のプロンプトインジェクション防御 |
| **nlk/jsonfix** | LLMレスポンスのJSON修復（ウィンドウ分析パース） |
| **objstore** | 分析レポートのアーティファクト管理に利用可能 |
| **Hot/Warm/Cold記憶** | 分析コンテキストはチャット記憶から独立して管理 |
| **時間認識** | SQL生成プロンプトに現在時刻+TZを注入 |
| **Gemmaタグパーサー** | 分析ツール呼び出しのフォールバック解析 |
| **MITL** | バックグラウンド分析起動の承認制御に対応 |

## パフォーマンス特性

Apple Silicon（Mシリーズ）上のgemma-4-26b-a4bでの実測値:

| 操作 | 時間 | 備考 |
|------|------|------|
| CSVロード（20行） | <50ms | DuckDBネイティブリーダー |
| 直接SQLクエリ | <10ms | インプロセスDuckDB |
| 自然言語 → SQL | ~5秒 | LLM 1往復 |
| クイック要約 | ~10秒 | クエリ + LLM要約 |
| スライドウィンドウ（15行, 4ウィンドウ） | ~28秒 | LLM 4回 + 最終レポート |
| 分析提案 | ~16秒 | スキーマコンテキスト付きLLM 1回 |

バイナリサイズ増加: 約15MB（DuckDB組み込みライブラリ）。
