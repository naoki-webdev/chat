# Orbit Chat

Go + React / TypeScriptで構築した、リアルタイムチャットアプリです。
チャンネル・ダイレクトメッセージ・スレッドを備え、WebSocketによる即時反映だけでなく、切断復帰や未読状態まで一貫して扱える構成にしています。

![Orbit Chatの画面](./test-screenshot-wide.png)

## このアプリで見てほしいこと

- **通信が一時的に切れても会話を復元できる**
  最後に受信したイベント以降の差分を取得するため、メッセージの作成・編集・削除・リアクションを取りこぼしにくい設計です。
- **未読を「数字」ではなく読んだ位置で管理している**
  複数端末で使っても既読位置が共有されるよう、チャンネルごとのread cursorをPostgreSQLへ保存します。
- **AIが会話を読む負担を減らす**
  未読メッセージを優先して、決定事項・依頼・未解決事項を整理します。元メッセージへ戻れるため、必要な部分だけ原文を確認できます。
- **権限のない会話をリアルタイム配信しない**
  チャンネルへの参加権限を履歴取得だけでなく、イベント差分とWebSocket配信でも確認します。

## Architecture

```mermaid
flowchart LR
  Browser[React / TypeScript]
  API[Go HTTP API]
  Hub[WebSocket Hub]
  DB[(PostgreSQL)]
  AI[Orbit AI Service]
  Provider[Gemini / OpenAI-compatible / Ollama / Bedrock]

  Browser -->|REST: auth, history, commands| API
  Browser <-->|WebSocket: realtime events| Hub
  API --> DB
  API --> Hub
  API --> AI
  AI --> Provider
  AI --> DB
```

RESTは認証、履歴取得、メッセージ操作を担当し、WebSocketはリアルタイムイベントの配信を担当します。AIのAPIキーはGoバックエンドだけが保持し、ブラウザへ渡しません。

## 技術的に工夫したこと

### 切断復帰とイベント差分同期

すべての永続イベントに連番を付けています。

```text
101 message.created
102 message.created
103 reaction.added
104 message.updated
```

クライアントが102まで受信して切断した場合、再接続時に`GET /api/events?after=102`を呼び出して103以降を復元します。WebSocketイベントの連番に飛びがあった場合も、同じ差分同期へ切り替えます。送信バッファが満杯になったクライアントは切断し、再接続時の復元に任せることで、イベントを黙って捨てないようにしています。

### read cursorによる未読管理

未読数だけを保存せず、ユーザーごと・チャンネルごとに最後に読んだイベント位置を保存します。

```text
channel_read_states
  user_id
  channel_id
  last_read_sequence
```

そのため、最後に読んだメッセージが削除されても既読位置が巻き戻りません。

### チャンネル単位のアクセス制御

`channel_members`で参加者を管理しています。履歴、スレッド、リアクション、イベント差分、WebSocket購読のすべてでmembershipを確認します。WebSocketのbroadcastでは、イベントごとにmembership snapshotを取得してからHub内で配信対象を絞り込みます。

### メッセージの整合性

- メッセージの編集・削除は所有者だけが実行可能
- thread rootの削除はsoft deleteとして返信を保持
- スレッドは1階層に制限
- リアクションは`UNIQUE(message_id, user_id, emoji)`とトランザクションで二重追加を防止
- 履歴は`before`カーソルによるcursor pagination

## Orbit AI

Orbit AIは、単独のAIチャット画面ではなく、既存のリアルタイムチャットへ参加するAIメンバーとして実装しています。

ユーザーがOrbit AIへ送ったメッセージに対して、Go側でAIのストリーミング応答を受け取り、次のWebSocketイベントへ変換して画面へ流します。

```text
message.ai_started
message.ai_delta
message.ai_completed
message.ai_failed
```

チャンネルの「会話の要点」では、未読メッセージとスレッド返信を優先してAIへ渡し、決定事項・依頼・未解決事項・雑談に分類します。各項目には実際に存在する元メッセージIDだけを付け、クリックで原文へ移動できます。

未読件数は全件を正確に表示しつつ、AIへ渡す履歴は新しい側から最大200件・32,000文字までに制限します。大量の未読がある場合も、画面上で「未読件数」と「要約対象件数」を分けて表示します。

AI Providerは`AI_PROVIDER`で切り替えられます。

| Provider | 用途 |
| --- | --- |
| Gemini / OpenAI | 公開デモ向けのクラウドAI |
| Ollama | データを外部AIへ送らないローカル利用 |
| OpenRouter | 複数モデルを切り替える検証 |
| Amazon Bedrock | AWS環境を想定した運用 |

## 主な機能

- ワークスペース、チャンネル、ダイレクトメッセージ
- メッセージの送信・編集・削除、リアクション、スレッド返信
- Typing / Presence、未読バッジ、既読状態の保存
- 会話内検索、保存済みメッセージ、キーボードショートカット
- HTTP-only Cookieセッション、bcrypt、チャンネルmembership
- Vitest、Playwright、PostgreSQL統合テスト、GitHub Actions

## 技術スタック

- Frontend: React, TypeScript, Vite
- Backend: Go, `net/http`, Gorilla WebSocket
- Database: PostgreSQL, versioned SQL migrations
- Authentication: bcrypt, HTTP-only Cookie session
- AI: OpenAI-compatible streaming API, Gemini, Ollama, OpenRouter, Bedrock
- Testing: Vitest, Playwright, Go test, Go race detector

## 起動

### Frontend

```bash
npm ci
npm run dev
```

### Backend

別ターミナルで実行します。ローカル開発では`DATABASE_URL`未指定時にインメモリストアを使用できます。

```powershell
$env:APP_ENV = "development"
cd backend
go run ./cmd/server
```

PostgreSQLを使う場合は、プロジェクトルートで次のように起動します。

```powershell
docker compose up -d postgres
$env:DATABASE_URL = "postgres://orbit:orbit@127.0.0.1:5432/orbit_chat?sslmode=disable"
$env:APP_ENV = "development"
cd backend
go run ./cmd/server
```

フロントエンドは`http://127.0.0.1:4174`、バックエンドAPIは`http://localhost:8080`で起動します。

デモログインは、`APP_ENV=development`または`test`で利用できます。

```text
メールアドレス: demo@example.com
パスワード: demo-password
```

本番環境では`DATABASE_URL`、`FRONTEND_ORIGIN`、`COOKIE_SECURE=true`が必須で、既知パスワードのデモユーザーは自動作成されません。

### Orbit AIを接続する場合

APIキーはバックエンドの環境変数へ設定します。設定しない場合はMock Providerで動作します。

```powershell
$env:AI_PROVIDER = "gemini"
$env:AI_API_KEY = "your-api-key"
$env:AI_MODEL = "gemini-3.7-flash"
```

利用可能なProviderと設定例は[backend/.env.example](./backend/.env.example)を参照してください。

## テスト

```bash
npm test
npm run build
npm run test:e2e
```

バックエンドのテストは`backend`ディレクトリで実行します。

```powershell
cd backend
go test ./...
go test -race ./...
```

GitHub Actionsでは、PostgreSQL 16を使った認可統合テスト、フロントエンドのテスト・ビルド、Playwright E2E、Go race detectorを実行します。

## ディレクトリ構成

```text
src/
  components/   UIコンポーネント
  hooks/        WebSocket・チャット状態管理
  services/     REST API・WebSocket client
  types/        フロントエンドの型と変換処理
backend/
  cmd/server/   HTTP/WebSocketサーバーとRepository実装
  internal/ai/  AI Provider interfaceとストリーミング処理
  cmd/server/migrations/
                versioned SQL migrations
e2e/            Playwrightシナリオ
```

## 次の候補

- Decision Memory（決定事項・理由・関連メッセージの永続化と検索）
- ワークスペース、ロール、招待の管理モデル
- メンション、通知、添付ファイル
- membership snapshotのキャッシュと負荷テスト
