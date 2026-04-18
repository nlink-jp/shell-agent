# エージェント＆ツールアーキテクチャ — shell-agent

> ステータス: v0.7.0
> 日付: 2026-04-19

## 概要

shell-agentのエージェントループは、3つのツール実行チャネル — シェルスクリプト、MCPサーバー、内蔵ツール — を統合フィードバックループで制御する。ローカルLLMとの信頼性を最優先し、ループをシンプルに保ち、テキスト内ツール呼び出しのフォールバックパーサーを提供する。

## 設計判断

### なぜシンプルなフィードバックループか（ReActではなく）

**検討した代替案:** ReAct（Reason + Act）パターン。Plan/Execute/Summarizeの明示的なフェーズ。

**実際に起きたこと:** gemma-4-26b-a4bでReActを実装・テストした結果:
- 中間要約がモデルにテキストレスポンス内でツール呼び出しタグを出力させた
- 計画品質が不安定 — モデルが従えない計画を生成
- 反復ごとに3回のLLM呼び出しがトークンを過剰消費
- 量子化されたローカルモデルが確実に処理できる複雑さを超えた

**採用した方式:** シンプルなツール呼び出しフィードバックループ。

```
for round < maxRounds:
    LLM(messages, tools) →
    │
    ├─ テキストレスポンス → 完了（ユーザーに返却）
    │
    └─ ツール呼び出しあり → 各々実行 → 結果を追加 → ループ継続
```

### なぜGemmaタグパーサーか

gemma-4はAPIが構造化ツール呼び出しを返さない場合、ネイティブタグ（`<|tool_call>call:name{args}<tool_call|>`）を使用する。これは以下の場合に発生:
- APIサーバー（LM Studio）が特定のプロンプトでテキストのみのレスポンスを返す
- toolsパラメータが提供されていてもモデルがテキスト内でツール呼び出しを出力する

パーサーなしでは、これらはチャット内で壊れたテキストとして表示される。

## エージェントループ（react.go）

### ターンごとのライフサイクル

```
1. 初期化: configからmaxRounds取得、agentLog作成

2. ループ (round 0..maxRounds-1):
   a. セッションレコードからメッセージ構築（buildMessages）
   b. LLM呼び出し: ChatWithContext(ctx, messages, toolDefs)
   c. トークン追跡: resp.Usage → tokenStats

   d. レスポンス解析:
      ├─ API tool_callsあり? → そのまま使用
      ├─ API tool_callsなし、テキストにgemmaタグ含む?
      │   → parseGemmaToolCalls(text) → 合成ツール呼び出し
      └─ ツール呼び出しなし?
          ├─ 空テキスト? → continue（リトライ）
          └─ テキストあり? → タイムスタンプ漏出除去 → 返却

   e. 各ツール呼び出しについて:
      ├─ jsonfix.Extract(arguments) → JSON修復
      ├─ handleBuiltinTool(name, args) → 内蔵ツールを先に試行
      ├─ handleMCPTool(name, args) → MCP名前空間を試行
      ├─ handleShellTool(name, args) → シェルスクリプト登録
      └─ fuzzyMatch(name, registry) → フォールバック: contains一致
      
      → 結果をRecord{Role: "tool", Tier: hot}として保存
      → 結果から画像を抽出 → objstore

   f. ループ継続

3. ループ消費 → 最後のレスポンスまたはエラーを返却
```

### Gemmaタグパーサー

**対応フォーマット:**
```
<|tool_call>call:weather{"location":"Tokyo"}<tool_call|>
<tool_call>call:search{"query":"DuckDB"}</tool_call>
```

**解析手順:**
1. タグ境界を検出
2. `call:`以降の内部テキストを抽出
3. 最初の`{`で分割 → ツール名 + JSON引数
4. nlk/jsonfix.Extract()でJSON修復
5. 合成IDを割当: `"gemma-0"`, `"gemma-1"`, ...

**ファジーツール名マッチング:**
```go
// LLMが"weather"の代わりに"weather:get_current_weather"を出力することがある
for _, t := range registry {
    if strings.Contains(llmName, t.Name) {
        return t  // "weather:get_current_weather"は"weather"を含む
    }
}
```

## ツール実行チャネル

### チャネル1: 内蔵ツール

`app.go`の`handleBuiltinTool()`で直接処理:

| ツール | 用途 |
|--------|------|
| `list-images` | 会話内画像をタイムスタンプ付きで一覧 |
| `view-image` | ID指定で過去の画像をリコール |
| `create-report` | 画像付きMarkdownレポート生成 |
| `load-data` | CSV/JSON/JSONLをDuckDBにロード |
| `describe-data` | テーブルスキーマ表示/説明付与 |
| `query-preview` | 自然言語 → SQL → 結果プレビュー |
| `query-sql` | 直接SQL実行 |
| `suggest-analysis` | LLMが分析観点を提案 |
| `quick-summary` | クエリ + LLM要約 |
| `analyze-bg` | バックグラウンド分析プロセス起動 |
| `analysis-status` | バックグラウンドジョブ進捗確認 |
| `analysis-result` | 完了レポート取得 |
| `reset-analysis` | 分析データベースリセット |

### チャネル2: MCPツール（mcp-guardian経由）

**アーキテクチャ:**
```
shell-agent ←stdio→ mcp-guardian ←stdio→ MCPサーバー
                     (認証プロキシ)
```

**Guardianライフサイクル:**
1. `Start()` → バイナリを子プロセスとして起動、stdioパイプ設定、初期化、ツール検出
2. `CallTool(name, args)` → JSON-RPC 2.0リクエスト/レスポンス
3. `Stop()` → stdin閉鎖、プロセスkill

**JSON-RPC 2.0プロトコル:**
```json
→ {"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"q":"test"}}}
← {"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"..."}]}}
```

**マルチサーバー名前空間:**
- ツール名接頭辞: `mcp__<guardianName>__<toolName>`
- 各guardianは独立した子プロセス
- 設定保存時にホットリロード

### チャネル3: シェルスクリプトツール

**検出:**
```bash
#!/bin/bash
# @tool: get-weather
# @description: 指定した場所の現在の天気を取得
# @param: location string "都市名または座標"
# @category: read
```

- `~/Library/Application Support/shell-agent/tools/` に配置
- `Registry.Scan()`が各ファイルの先頭50行を読取り
- ヘッダーコメントを`ToolScript`構造体にパース

**実行:**
1. ジョブワークスペース作成（実行ごとの一時ディレクトリ）
2. 環境変数設定: `SHELL_AGENT_JOB_ID`, `SHELL_AGENT_WORK_DIR`
3. stdin経由でJSON引数渡し
4. 3分タイムアウトで実行
5. stdoutを結果テキストとして収集
6. ファイナライズ: ワークスペース内の生成ファイルをArtifactとして収集
7. 一時ディレクトリ削除

**Artifact収集:**
- 全Artifactをobjstoreに保存
- IDをツール出力に追記: `[Artifacts produced: a1b2c3d4e5f6]`

## MITL（Man-In-The-Loop）

### カテゴリシステム

| カテゴリ | MITL必須 | 例 |
|---------|---------|---|
| `read` | いいえ | ファイル一覧、APIクエリ、データ表示 |
| `write` | はい | ファイル作成、データ変更 |
| `execute` | はい | シェルコマンド、プロセス起動 |

内蔵ツールはMITLをバイパス（信頼された内部コード）。

### 承認フロー

```
1. LLMからツール呼び出し受信
2. カテゴリチェック: tool.Category.NeedsMITL()
3. MITL必要な場合:
   → "chat:toolcall_request"イベントをフロントエンドに発行
   → フロントエンドが承認ダイアログ表示（ツール名、引数、カテゴリ）
   → ユーザーが承認または拒否をクリック
   → mitlChチャネル経由でレスポンス送信
4. 承認 → ツール実行
5. 拒否 → "Tool call '{name}' was rejected by the user."を返却
```

## マルチモーダル画像処理

### 画像フロー

```
入力経路:
  ドラッグ＆ドロップ → data URL → objstore.SaveDataURL() → ImageEntry{ID}
  クリップボード貼付 → 同上
  ファイルピッカー → 同上
  ツールアーティファクト → objstore.Save() → ImageEntry{ID}

LLMコンテキスト:
  最新画像 → フルbase64 data URL + ラベル
  古い画像 → テキスト参照: "[Past image ID: {id}]"
  リコール画像 → __IMAGE_RECALL__マーカー → buildMessages()で展開
```

### スマート画像リコール

LLMが`view-image`ツール経由で過去の画像をリコール可能:
```
ユーザー: 「さっき共有した画像と比較して」
LLM: [list-images] → IDを発見 → [view-image(id)]
→ __IMAGE_RECALL__マーカーを返却
→ buildMessages()がマーカーをdata URLに展開
→ 次のLLM呼び出しでリコールされた画像をビジュアル入力として認識
```

## レポート生成

### フロー

```
LLMがcreate-report(title, content, filename)を呼出
  → 画像参照を抽出: ![desc](image:ID)
  → objstoreに保存
  → コンテンツからmarkdown画像を除去
  → Record{Role: "report"}を作成
  → "chat:report"イベント発行（画像ID付き）

フロントエンド表示:
  → markdownコンテンツをレンダリング
  → 画像はIDでGetImageDataURL()経由ロード
  → ギャラリー表示（インラインではない）
  → フルスクリーンオーバーレイ: 拡大、コピー、保存

ディスクに保存:
  → ネイティブダイアログでパス選択
  → 各画像ID → objstoreから読込 → base64 data URL
  → インラインで追記: ![Image N](data:image/png;base64,...)
  → 自己完結型markdownファイルとして書出
```

### 設計: ギャラリー、インラインではない

画像はレポートテキストの下にギャラリーとして表示。インラインmarkdown埋込ではない。理由:
- ReactMarkdownがdata: URLをデフォルトでサニタイズ
- ギャラリーはmarkdown構造に関わらず一貫したレイアウトを提供
- ライトボックスビューアがギャラリーアイテムと自然に連携
- コピー/保存アクションが画像ごとに適用可能

## ツール数管理

シェルスクリプト + MCP + 内蔵ツールで合計30超になり得る。gemma-4-26b-a4bは26個超のツール定義で精度低下。対策:

1. **無効化トグル**: Settings → Toolsで不要なツールを無効化
2. **除外**: 無効ツールは`buildToolDefs()`でLLMに送信されない
3. **Q8量子化推奨**: Q4_K_Mはツール呼び出し精度が悪化
