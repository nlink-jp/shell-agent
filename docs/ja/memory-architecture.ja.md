# 記憶アーキテクチャ — shell-agent

> ステータス: v0.7.0
> 日付: 2026-04-19

## 概要

shell-agentの記憶システムは、ローカルLLMの制約に合わせた**時間認識型マルチ階層会話永続化**を提供する。クラウドベースのアシスタントが大規模コンテキストウィンドウを持つのに対し、ローカルモデルは通常8K-128Kトークンで動作する。本記憶システムは、古いコンテキストを段階的に要約することで、時間的認識を保持しながら長い会話の一貫性を確保する。

## 設計判断

### なぜ3階層か（RAGでも固定ウィンドウでもなく）

**検討した代替案:**
1. **固定スライディングウィンドウ** — 却下: 突然のコンテキスト喪失、議論内容の要約なし
2. **RAG（検索拡張生成）** — 却下: 埋め込みモデル、ベクトルストア、検索パイプラインが必要 — ローカル専用ツールには過剰
3. **無制限コンテキスト** — 却下: ローカルLLMにはハード制限あり、128Kモデルでもフルコンテキストでは劣化

**採用した方式:** LLMコンパクションによるHot/Warm/Cold 3階層。

**根拠:**
- Hot階層は即時コンテキストのための正確な会話を保持
- Warm要約は最近の履歴から重要な決定やトピックを保持
- Cold要約は長期セッションの連続性を提供
- LLMが要約を生成 — 同一モデルを圧縮に活用
- すべてのレコードにタイムスタンプを埋め込み、時間的推論を可能にする

### なぜPinned Memory（要約だけでなく）

Warm/Cold要約は時間的 — 「何が議論されたか」を捉える。しかし一部の事実は**時間不変**: ユーザーの好み、重要な決定、学習したコンテキスト。Pinned Memoryはこれらの事実を自律的に抽出し、セッション横断で永続化する。

**なぜバイリンガル:** システムプロンプトは英語（LLMの推論精度向上のため）。しかし「ユーザーはVimを好む」のような事実はネイティブ言語でのコンテキストが必要。両方の形式を保存: 英語はシステムプロンプト注入用、ネイティブは表示用。

## アーキテクチャ

### レコード構造

システム内のすべてのメッセージは`Record`である:

```go
type Record struct {
    Timestamp    time.Time      // 実時刻（システムが注入）
    Role         string         // "user", "assistant", "tool", "system", "report"
    Content      string         // 全文テキスト
    Tier         Tier           // hot | warm | cold
    SummaryRange *TimeRange     // warm/cold用: 要約されたコンテンツの時間範囲
    Images       []ImageEntry   // objstore ID参照
    InTokens     int            // 消費したプロンプトトークン数
    OutTokens    int            // 生成した完了トークン数
    Report       *ReportData    // role == "report"の場合のメタデータ
}
```

重要な設計: **タイムスタンプはシステムが注入し、LLMが生成するのではない。** LLMは各メッセージに`[15:04:05]`が前置されるのを見るが、時計を制御しない。これにより正確な時間的推論が可能になる（「10分前に何を言った？」）。

### 階層ライフサイクル

```
ユーザーがメッセージ送信
  ↓
Hot階層 ← 新規 Record{Tier: hot}
  │
  │ HotTokenCount() > HotTokenLimit (65536)?
  │ はい ↓
  │
  PromoteOldestHotToWarm()
  │ → 最も古いhotレコードを選択（最新のuser+assistantペアは保持）
  │ → 目標: 超過トークン分を削減
  ↓
  LLMが選択されたレコードを要約
  ↓
Warm階層 ← 新規 Record{Tier: warm, SummaryRange: {from, to}}
  │
  │ WarmRetentionMins（60分）より古い?
  │ はい ↓
  │
Cold階層 ← 再分類 Record{Tier: cold}
  │
  │ ColdRetentionMins（1440分）より古い?
  │ はい ↓
  │
  破棄（要約はセッションJSONに残る）
```

### Hot階層 — 逐語的会話

- **内容**: 完全なメッセージテキスト（無修正）
- **トークン予算**: `cfg.Memory.HotTokenLimit`（デフォルト65,536）
- **トークンカウント**: `EstimateTokens()` — 二重戦略:
  - `len(text) / 2`（文字ベース、CJKに保守的）
  - `len(text) / 4`（単語ベース、英語向き）
  - 過小カウントを防ぐため最大値を採用
- **自動補正**: HotTokenLimitが8192未満の場合、8192に引き上げ（ユーザーコンテキストを削除する過剰なコンパクションを防止）

### Warm階層 — LLM要約

- **内容**: コンパクトされたhotメッセージのLLM生成要約
- **TimeRange**: `{From, To}` — 元のメッセージのタイムスタンプ
- **保持期間**: `cfg.Memory.WarmRetentionMins`（デフォルト60分）
- **LLMコンテキストへの注入**: システムメッセージとして:
  ```
  [前回の会話要約 (15:00-15:30)]:
  プロジェクトアーキテクチャについて議論。分析にDuckDBを使用することを決定。
  主なトピック: データロード、SQL生成、バックグラウンド処理。
  ```

### Cold階層 — アーカイブ要約

- **内容**: warmと同形式、セッション初期のもの
- **保持期間**: `cfg.Memory.ColdRetentionMins`（デフォルト1440 = 24時間）
- **目的**: 非常に長いセッションへの深いコンテキスト提供

### LLM向けメッセージ構築

`buildMessages()`は以下の順序でLLMコンテキストを組み立てる:

```
1. システムプロンプト
   ├── 現在時刻 + タイムゾーン ("2026-04-19 15:04:05 JST UTC+09:00")
   ├── 位置情報（利用可能な場合）
   ├── Pinned Memory（英語の事実）
   └── Guardタグ指示

2. Cold要約（時系列順）

3. Warm要約（時系列順）

4. Hotメッセージ（時系列順）
   ├── ユーザーメッセージ（[HH:MM:SS]接頭辞付き）
   ├── アシスタントレスポンス
   ├── ツール結果（API上はrole:"user"だがtoolとしてタグ付き）
   └── レポート（200文字に切り詰め）

5. 最新のユーザーメッセージ
   └── 現在の画像をdata URLとして（マルチモーダルの場合）
```

## Pinned Memory

### 抽出プロセス

各アシスタントターン後に重要な事実を抽出:

1. 最新4件のhotメッセージを取得
2. LLMにピン留め可能な事実の特定を依頼
3. LLMがJSON返却: `[{fact, native_fact, category}]`
4. 既存のピン留めと重複排除
5. 新規事実を追加

### 構造

```go
type PinnedMemory struct {
    Fact       string    // 英語: "User prefers Vim over VS Code"
    NativeFact string    // ネイティブ: "ユーザーはVimを好む"
    Category   string    // "preference" | "decision" | "fact" | "context"
    SourceTime time.Time // 事実が言及された時刻
    CreatedAt  time.Time // ピン留めされた時刻
}
```

### カテゴリ

| カテゴリ | 用途 | 例 |
|---------|------|---|
| `preference` | ユーザーの好みや習慣 | 「ダークテーマを好む」 |
| `decision` | アーキテクチャや設計の決定 | 「SQLiteではなくDuckDBを選択」 |
| `fact` | ドメイン知識やコンテキスト | 「プロジェクトはWails v2を使用」 |
| `context` | 状況認識 | 「ユーザーはセキュリティエンジニア」 |

## 時間・空間認識

### 時間注入

すべてのシステムプロンプトに含む:
```
Current time: 2026-04-19 15:04:05 (timezone: JST, UTC+09:00)
```

メモリ内のユーザーメッセージに接頭辞:
```
[15:04:05] ユーザーの実際のメッセージ
```

これによりLLMは:
- メッセージ間の経過時間を理解
- 相対時間参照を解決（「30分前に何を言った？」）
- 時間に適した応答を生成（「おはようございます」vs「こんばんは」）

**タイムスタンプ漏出防止**: LLMが`[HH:MM:SS]`形式をレスポンスに模倣することがある。`stripLeakedTimestamps()`が表示前にこれを除去する。

### 空間注入

位置情報（get-locationツール経由、利用可能な場合）:
```
Location: Tokyo, Japan（タイムゾーンベースの推論）
```

外部GPS APIなしで位置認識レスポンスを実現。

## トークンバジェット管理

### デフォルト

| パラメータ | デフォルト | 目的 |
|-----------|-----------|------|
| `HotTokenLimit` | 65,536 | Hot階層の最大トークン数 |
| `WarmRetentionMins` | 60 | warm → coldまでの分数 |
| `ColdRetentionMins` | 1440 | cold → 破棄までの分数 |
| `MaxToolRounds` | 10 | ターン当たりの最大ツール呼び出し反復 |

### トークン推定

```go
func EstimateTokens(text string) int {
    charBased := len(text) / 2  // CJKに保守的
    wordBased := len(text) / 4  // 英語に標準的
    return max(charBased, wordBased)
}
```

二重戦略がJSON重視コンテンツの過小推定を防止（data-analyzerプロジェクトからの教訓）。
