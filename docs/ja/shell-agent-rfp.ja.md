# RFP: shell-agent

> Generated: 2026-04-18
> Status: Draft

## 1. 課題定義

ローカルLLMを活用したmacOS GUIチャット＆エージェントツール。メニューバー常駐のSwiftUIランチャーからショートカットキーで本体アプリ（Wails v2 + React）を起動する構成をとる。

MCP対応はmcp-guardian経由のstdioプロキシで実現し、独自機能としてシェルスクリプトをTool Callingのfunctionとして登録・実行できる仕組みを持つ。書き込み・実行系の操作にはMITL（Man-In-The-Loop）承認を必須とし、安全性を確保する。

マルチターンチャットでは各メッセージにタイムスタンプを含むJSON構造の記憶を持ち、Hot/Warm/Coldの3階層スライディングウィンドウで管理する。これによりLLMが会話中の時間経過を正しく認識し、「30分前に言った」のような相対時間の指示を適切に処理できる。

マルチモーダル対応（画像入力）により、画像を含むチャットが可能。

バックエンドはOpenAI互換API（LM Studio想定）、デフォルトモデルはgoogle/gemma-4-26b-a4b。ターゲットユーザーは特に限定しない。

## 2. 機能仕様

### コマンド / API サーフェス

GUIアプリケーションのため、CLIコマンドは持たない。

**本体アプリ（Wails v2 + React）:**
- チャットウィンドウ（メッセージ一覧＋入力欄）
- 画像入力対応（ドラッグ＆ドロップ、ペースト、ファイル選択）
- サイドバー: Tool一覧、会話セッション一覧、LLM状態モニタリングパネル（認知時刻、記憶状態など）
- Tool Calling実行時のMITL承認UI

**ランチャーアプリ（SwiftUI）:**
- メニューバー常駐
- ショートカットキーで本体アプリ起動
- 新規会話、既存会話選択、設定、終了

### 入出力

**LLM通信:**
- OpenAI互換 `/v1/chat/completions` エンドポイント（ストリーミング対応）
- リクエスト: システムプロンプト＋記憶コンテキスト＋ユーザーメッセージ（テキスト＋画像）
- 画像: base64エンコードでcontent配列に含める（OpenAI Vision API形式、llm-cliパターン流用）
- レスポンス: SSEストリーミングでトークン逐次表示

**シェルスクリプトTool:**
- 入力: stdin経由でJSONを渡す
- 出力: stdout経由でテキストまたはJSON構造を返却
- エラー: 呼び出し側でエラーキャッチし、LLMに実行失敗を伝達

**MCP:**
- mcp-guardianをstdio子プロセスとして起動
- JSON-RPC 2.0 over stdio

### コンフィギュレーション

**設定項目（JSON file）:**
- APIエンドポイントURL
- デフォルトモデル名
- APIキー（オプション）
- ツールスクリプトディレクトリパス
- mcp-guardian設定（バイナリパス、設定ファイルパス）
- メモリ設定（Hotトークン上限、Warm/Cold保持期間）
- ショートカットキー設定

**保存場所:** `~/Library/Application Support/shell-agent/`

### 外部依存

| 依存 | 種別 | 必須 |
|------|------|------|
| LM Studio（OpenAI互換APIサーバー） | ローカルサービス | Yes |
| mcp-guardian | バイナリ（stdio子プロセス） | Yes（MCP利用時） |
| nlk | Goライブラリ（直接import） | Yes |

## 3. 設計判断

**言語・フレームワーク:**
- 本体: Go + Wails v2 + React — Goバックエンドによりnlkライブラリおよびllm-cliのAPIクライアントパターンを直接流用可能。Wails v2は安定版でドキュメントが豊富。Reactはエコシステムが最も成熟しており、Wails v2でのサポートが最も手厚い。
- ランチャー: SwiftUI — macOSネイティブのMenuBarExtra APIを利用するためSwiftUIが最適。quick-translateの実装パターンを流用。

**既存ツールとの関係:**
- `llm-cli`（cli-series）: APIクライアント実装パターン（ストリーミング、リトライ、フォーマットフォールバック、マルチモーダル画像入力）を流用
- `nlk`（lib-series）: guard（プロンプトインジェクション防御）、jsonfix（JSON修復）、strip（思考タグ除去）、backoff、validateを直接利用
- `sai`（lab-series）: Hot/Warm/Cold記憶階層の概念を継承。ただしRAGではなくスライディングウィンドウ方式に変更し、タイムスタンプによる時間認識を追加
- `mcp-guardian`（util-series）: MCPクライアント機能を委譲。認証・ガバナンスはguardianが担当
- `quick-translate`（util-series）: ランチャーアプリのMenuBarExtra + AppDelegate + PanelManagerパターンを流用

**スコープ外:**
- クラウド同期
- マルチユーザー対応
- サーバーモード
- データベース（将来的に必要になった場合に再検討）

## 4. 開発計画

### Phase 1: Core — チャット基盤

- Wails v2 + Reactプロジェクトスキャフォールド
- OpenAI互換APIクライアント（llm-cliパターン流用、ストリーミング対応）
- 基本チャットUI（メッセージ一覧＋入力欄）
- タイムスタンプ付きHot記憶（トークンベーススライディングウィンドウ）
- 会話の永続化（JSON file）
- nlk統合（guard, jsonfix, strip, backoff）
- 基本テスト

**独立レビュー可能**

### Phase 2: Features — エージェント機能

- シェルスクリプトTool Calling
  - ディレクトリスキャン・ヘッダーコメント解析による自動登録
  - stdin/stdout経由のJSON I/O
  - MITL承認UI（Read系は承認不要、Write/Execute系は承認必須）
- mcp-guardian連携（stdio子プロセス管理）
- Warm/Cold記憶階層（LLM要約＋時間範囲保持）
- サイドバー（Tool一覧、会話セッション一覧、LLM状態モニタリング）
- SwiftUIランチャーアプリ（メニューバー常駐、ショートカットキー）

**独立レビュー可能**

### Phase 3: Release — ドキュメント・品質

- テスト拡充
- README.md / README.ja.md
- CHANGELOG.md
- AGENTS.md
- リリースビルド・配布

## 5. 必要なAPIスコープ / 権限

None — 外部サービスへの認証はすべてmcp-guardianに委譲。ローカルLLMサーバーは認証不要（オプションでAPIキー対応）。

## 6. シリーズ配置

Series: **util-series**
Reason: quick-translateの前例があり、macOS GUIアプリケーションのutil-seriesへの配置は確立済み。パイプフレンドリーなCLIではないが、ローカルデータ処理・変換という本質は共通する。

## 7. 外部プラットフォーム制約

| 制約 | 詳細 |
|------|------|
| LM Studio | ローカルサーバーが起動している前提。未起動時のエラーハンドリングが必要 |
| Wails v2 | macOS 10.15以上が必要 |
| gemma-4-26b-a4b | 約16GB VRAM必要（Apple Silicon M1/M2 Pro以上で動作可能） |

---

## ツールスクリプト ヘッダー形式

```bash
#!/bin/bash
# @tool: list-files
# @description: List files in a directory
# @param: path string "Directory path to list"
# @category: read
```

- `@tool`: ツール名（LLMに公開される関数名）
- `@description`: ツールの説明（LLMが呼び出し判断に使用）
- `@param`: パラメータ定義（名前 型 説明）、複数行可
- `@category`: `read`（MITL不要）または `write`/`execute`（MITL必須）

---

## 記憶構造（JSON）

```json
{
  "timestamp": "2026-04-18T15:30:00+09:00",
  "role": "user",
  "content": "...",
  "tier": "hot",
  "summary_range": null
}
```

Warm/Coldの場合:
```json
{
  "timestamp": "2026-04-18T16:00:00+09:00",
  "role": "summary",
  "content": "15:00-15:55の会話要約: ...",
  "tier": "warm",
  "summary_range": {
    "from": "2026-04-18T15:00:00+09:00",
    "to": "2026-04-18T15:55:00+09:00"
  }
}
```

---

## Discussion Log

1. **ツール名決定**: `shell-agent` — シェルスクリプトTool Callingを特徴とする名称
2. **アーキテクチャ選択**: 当初macOS GUI（SwiftUI）を想定していたが、nlk（Go）の直接利用を可能にするためWails v2（Go + React）に変更。ランチャーのみSwiftUI
3. **MCP方針**: mcp-guardianにインターフェースを委譲し、すべてstdio型で動作させる。shell-agent側で認証管理は不要
4. **記憶モデル**: saiのHot/Warm/Cold 3階層を継承しつつ、RAGからスライディングウィンドウ方式に変更。saiで欠けていた時間認識をJSON構造内のタイムスタンプで解決
5. **MITL方針**: Read系ファンクションはMITL不要、シェル実行・書き込み・更新・削除系はMITL必須
6. **セキュリティ**: nlk/guardでプロンプトインジェクション防御、nlk/jsonfixでJSON構造崩れ対策
7. **永続化**: JSON fileで十分。DB化が必要になった段階で再検討
8. **フロントエンド**: React選択 — Wails v2での成熟度が最も高くトラブルが少ない
9. **スコープ外**: クラウド同期、マルチユーザー、サーバーモード
10. **シリーズ配置**: util-series（quick-translateの前例に準ずる）
11. **マルチモーダル**: 画像入力対応。llm-cliの既存VLM実装（base64エンコード、OpenAI Vision API形式）を流用
