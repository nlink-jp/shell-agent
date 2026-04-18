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

## デフォルトモデル

google/gemma-4-26b-a4b

## 設定

設定は `~/Library/Application Support/shell-agent/config.json` に保存されます。
アプリ内の設定パネルから変更可能です。

## ライセンス

MIT License - 詳細は [LICENSE](LICENSE) を参照してください。
