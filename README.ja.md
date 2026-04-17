# shell-agent

ローカルLLMを活用したmacOS GUIチャット＆エージェントツール。

## 機能

- OpenAI互換API（LM Studio）を使用したマルチターンチャット
- mcp-guardian経由のMCP対応（stdioプロキシ）
- MITL（Man-In-The-Loop）承認付きシェルスクリプトTool Calling
- タイムスタンプ対応Hot/Warm/Coldスライディングウィンドウ記憶
- マルチモーダル画像入力
- ショートカットキー対応メニューバーランチャー

## アーキテクチャ

```
shell-agent/
├── app/          # Wails v2 + React 本体アプリ（Goバックエンド）
├── launcher/     # SwiftUI メニューバーランチャー（macOSネイティブ）
└── docs/         # ドキュメントおよびRFP
```

## 動作要件

- macOS 10.15以上
- LM Studio（またはOpenAI互換APIサーバー）
- Apple Silicon M1/M2 Pro以上推奨（gemma-4-26b-a4b使用時）

## ビルド

```bash
cd app
make build
```

## 開発

```bash
cd app
make dev
```

## デフォルトモデル

google/gemma-4-26b-a4b

## 設定

設定は `~/Library/Application Support/shell-agent/config.json` に保存されます。

## ライセンス

MIT License - 詳細は [LICENSE](LICENSE) を参照してください。
