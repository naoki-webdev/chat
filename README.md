# Orbit Chat

Go + React/TypeScriptで育てるリアルタイムチャットです。Discord風の3ペインUIに、GoのREST APIとWebSocketサーバーを接続しています。

## 現在入っているもの

- ワークスペース、チャンネル、DMの切り替え
- メッセージ一覧と未読バッジ
- メッセージ送信（Enter）、編集、削除
- DB永続化されたリアクション、スレッド返信
- event_idによる切断復帰、read cursor、cursor pagination
- WebSocketのTyping / Presence一時イベント
- 会話内検索
- メンバー詳細パネル
- Orbit AIとの個別メッセージ（接続先を切り替えられるストリーミング回答）
- 会話の要点（決まったこと・依頼・未解決をまとめ、元メッセージへ移動）
- オンライン / 離席ステータスの切り替え
- LIVE / RECONNECTINGの接続状態表示
- デスクトップ向けのダークUI
- GoバックエンドのAPIとWebSocketブロードキャスト

フロントエンドはHTTP-only Cookieセッションでログインし、Go APIからmembershipのあるチャンネルと履歴を取得します。送信・編集・削除はREST、更新通知はWebSocketで処理します。PostgreSQL接続時はユーザー、セッション、チャンネル、membership、メッセージを永続化し、編集・削除にはメッセージ所有者チェックを行います。thread rootの削除はsoft deleteとして返信を保持します。

## 起動

### Frontend

Node.jsをインストールしたあと、このフォルダで実行します。

```bash
npm install
npm run dev
```

フロントの単体テストとE2Eは次で実行できます。

```bash
npm test
npm run test:e2e
```

### Backend

Goをインストールしたあと、別ターミナルで実行します。

```powershell
$env:APP_ENV = "development"
cd backend
go run ./cmd/server
```

PostgreSQLを使う場合は、先にプロジェクトルートで起動します。

```powershell
docker compose up -d postgres
$env:DATABASE_URL = "postgres://orbit:orbit@127.0.0.1:5432/orbit_chat?sslmode=disable"
$env:APP_ENV = "development"
cd backend
go run ./cmd/server
```

`APP_ENV=development` または `test` のときだけ、必要に応じてデモデータを投入できます。`APP_ENV=production`（または未指定の非ローカル環境）では `DATABASE_URL` が必須で、既知パスワードのデモユーザーは自動作成されません。デモ用ログインは `demo@example.com / demo-password` です。

APIは `http://localhost:8080`、WebSocketは `ws://localhost:8080/ws?channel_id=general` で利用できます。

フロントが参照するAPIの接続先は `VITE_API_BASE_URL` で変更できます。未指定の場合は `http://127.0.0.1:8080` です。

### Orbit AI

左サイドバーの `Orbit AI` を開くと、AI参加者との会話を試せます。Go側のProvider interfaceから、クラウドAI、ローカルLLM、マルチモデル基盤を同じWebSocketストリーミング経路へ接続できます。APIキーはブラウザへ渡しません。

#### Gemini

```powershell
$env:AI_PROVIDER = "gemini"
$env:AI_API_KEY = "your-gemini-api-key"
$env:AI_MODEL = "gemini-3.7-flash"
```

#### Ollama

```powershell
ollama pull qwen3:8b
$env:AI_PROVIDER = "ollama"
$env:AI_MODEL = "qwen3:8b"
```

#### OpenRouter

```powershell
$env:AI_PROVIDER = "openrouter"
$env:AI_API_KEY = "your-openrouter-api-key"
$env:AI_MODEL = "openai/gpt-4o-mini"
```

#### Amazon Bedrock Mantle

```powershell
$env:AI_PROVIDER = "bedrock"
$env:AI_API_KEY = "your-bedrock-api-key"
$env:AI_REGION = "ap-northeast-1"
$env:AI_MODEL = "openai.gpt-oss-120b"
```

設定後、バックエンドを再起動します。

```powershell
cd backend
go run ./cmd/server
```

`AI_PROVIDER` 未設定時は従来の `OPENAI_*` 設定を使用し、APIキー未設定時はMockへフォールバックします。

## 現在の実装範囲

- `backend/cmd/server/migrations`のversioned SQL migrationと初期データ投入
- bcryptパスワード、HTTP-only Cookieセッション、ログイン・登録・ログアウト
- `channel_members`によるチャンネル単位のアクセス制御（履歴、thread、reaction、event差分、WebSocket購読）
- WebSocket broadcast時のmembership snapshot（イベント1件につき1回取得し、Hub内で接続ユーザーを照合）
- メッセージ所有者チェック
- REST履歴取得とWebSocketリアルタイムイベント
- read cursor（`channel_read_states.last_read_sequence`を基準に、メッセージIDも保持）と未読件数の永続化
- event_idによる再接続差分同期
- cursor paginationによる履歴取得
- 会話の要点は未読メッセージ（スレッド返信を含む）を優先し、未読がなければ最近の履歴へフォールバック
- `parent_message_id`によるスレッド返信
- `UNIQUE(message_id,user_id,emoji)`を使ったリアクション競合制御
- Typing / Presenceの非永続Realtimeイベント
- App.tsx、WebSocket Hub、AI処理の責務分割
- Orbit AIの `message.ai_started` / `message.ai_delta` / `message.ai_completed` ストリーミングイベント
- `POST /api/channels/:channel_id/summary` による構造化された会話要約
- `AI_PROVIDER`によるOpenAI、Gemini、Ollama、OpenRouter、Amazon Bedrock Mantleの切り替え
- PostgreSQL実環境の認可統合テスト
- Vitest、Playwright、GitHub Actions、Go race detector

## 次の実装順

1. Decision Memory（決定事項・理由・関連メッセージを永続化して検索）
2. ワークスペースエンティティ、ロール、招待などの管理モデル
3. メンション、通知、添付ファイル
4. membership snapshotの短期キャッシュ・変更時invalidateと負荷テスト
