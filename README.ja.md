# shell-agent

ローカルLLMを活用したmacOS GUIチャット＆エージェントツール。

## 機能

- **マルチターンチャット** — OpenAI互換API（LM Studio）によるストリーミング対話
- **MCP対応** — mcp-guardian経由（複数サーバー対応、stdioプロキシ）
- **シェルスクリプトTool Calling** — 書き込み・実行系はMITL（Man-In-The-Loop）承認必須
- **タイムスタンプ対応記憶** — Hot/Warm/Cold 3階層スライディングウィンドウ、LLMによる自動要約
- **Pinned Memory** — 重要な事実をLLMが自律的に抽出し、セッション横断で永続保持
- **マルチモーダル** — ドラッグ＆ドロップ、ペースト、ファイル選択による画像入力＋スマート画像リコール
- **Markdownレンダリング** — GFM、コードブロックのシンタックスハイライト、テーブル対応
- **メニューバーランチャー**（SwiftUI）— グローバルホットキー（Ctrl+Shift+Space）
- **セキュリティ** — nlk/guard（プロンプトインジェクション防御）、nlk/jsonfix（JSON修復）、nlk/strip（思考タグ除去）
- **データ分析** — DuckDB内蔵によるCSV/JSON/JSONL分析、自然言語SQLクエリ、スライドウィンドウ要約、バックグラウンド分析
- **カラーテーマ** — Dark、Light（クリーム＋ブルー）、Warm（ブラウン）、Midnight（ネイビー）、ライブプレビュー対応
- **設定UI** — API、メモリ、ツール、MCPガーディアン、テーマ、起動モードのアプリ内設定
- **セッション管理** — タイトル自動生成、リネーム、確認付き削除
- **起動モード** — 新規チャットまたは前回のセッション復元を選択可能
- **ウィンドウ状態記憶** — 位置とサイズを起動間で保持

## アーキテクチャ

```
shell-agent/
├── app/          # Wails v2 + React 本体アプリ（Goバックエンド）
├── launcher/     # SwiftUI メニューバーランチャー（macOSネイティブ）
└── docs/         # ドキュメントおよびRFP
```

### Goバックエンドパッケージ

| パッケージ | 用途 |
|-----------|------|
| `internal/chat` | チャットエンジン、時間注入、メッセージ構築 |
| `internal/client` | OpenAI互換APIクライアント（ストリーミング＋非ストリーミング、マルチモーダル） |
| `internal/config` | JSON設定管理（~、$ENV展開対応） |
| `internal/mcp` | mcp-guardian stdio子プロセス管理 |
| `internal/memory` | Hot/Warm/Cold階層、Pinned Memory、画像ストア、セッション永続化 |
| `internal/objstore` | 画像・Blob・レポートの中央オブジェクトリポジトリ |
| `internal/analysis` | DuckDB分析エンジン、SQL生成、スライドウィンドウ要約 |
| `internal/toolcall` | シェルスクリプトツール登録、ヘッダー解析、MITLカテゴリ |

## 動作要件

- macOS 14以上（ランチャー）、macOS 10.15以上（本体アプリ）
- [LM Studio](https://lmstudio.ai/)（またはOpenAI互換APIサーバー）
- Apple Silicon M1/M2 Pro以上推奨（gemma-4-26b-a4b使用時）

## ビルド

```bash
# 本体アプリ
cd app
make build

# ランチャー
cd launcher/ShellAgentLauncher
swift build
```

## 開発

```bash
cd app
make dev    # Wails devサーバーによるホットリロード
```

## ツールスクリプト

`~/Library/Application Support/shell-agent/tools/` にヘッダー注釈付きのシェルスクリプトを配置：

```bash
#!/bin/bash
# @tool: list-files
# @description: List files in a directory
# @param: path string "Directory path to list"
# @category: read
```

カテゴリ: `read`（自動実行）、`write` / `execute`（MITL承認必須）

**セキュリティ**: 引数はシェルインジェクション防止のため、コマンドライン引数ではなくstdin経由のJSONで渡されます。ツールスクリプトはJSONを安全にパース（例: `jq`）し、パースした値を`eval`やクォートなしのシェル展開に渡してはいけません。

## MCP設定

設定UIまたは `config.json` でMCPサーバーを追加：

```json
{
  "guardians": [
    {
      "name": "filesystem",
      "binary_path": "~/.local/bin/mcp-guardian",
      "profile_path": "~/.config/mcp-guardian/profiles/filesystem.json"
    }
  ]
}
```

## データ分析

データファイルをロードし、自然言語またはSQLで分析：

```
ユーザー: /path/to/sales.csv を読み込んで地域別の売上合計を見せて
エージェント: [load-data] → [query-preview] → 東京: ¥2,024,500, 大阪: ¥918,000, ...
```

### 分析ツール

| ツール | 説明 |
|--------|------|
| `load-data` | CSV/JSON/JSONLをDuckDBにロード |
| `describe-data` | テーブルスキーマの表示・説明付与 |
| `query-preview` | 自然言語 → SQL → 結果プレビュー |
| `query-sql` | SQLを直接実行 |
| `suggest-analysis` | LLMが分析観点を提案 |
| `quick-summary` | クエリ実行 + LLM要約 |
| `analyze-bg` | バックグラウンド分析（アプリ終了後も継続） |
| `analysis-status` | バックグラウンドジョブの進捗確認 |
| `analysis-result` | 完了したレポートの取得 |
| `reset-analysis` | 全テーブルをクリア |

### バックグラウンド分析

大規模データセットの場合、`analyze-bg`はShell Agent終了後も継続するプロセスを起動します：

```
ユーザー: アクセスログのセキュリティ脅威を分析して
エージェント: [analyze-bg] → ジョブ開始: job-1713488400000
              ...しばらく後...
ユーザー: 分析の進捗は？
エージェント: [analysis-status] → 完了 (4ウィンドウ, 5件のFinding)
ユーザー: レポートを見せて
エージェント: [analysis-result] → セキュリティ脅威分析レポート
```

## デフォルトモデル

google/gemma-4-26b-a4b

## 設定

設定は `~/Library/Application Support/shell-agent/config.json` に保存されます。
アプリ内の設定パネルから変更可能です。

## ライセンス

MIT License - 詳細は [LICENSE](LICENSE) を参照してください。
