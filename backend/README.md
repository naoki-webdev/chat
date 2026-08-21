# Orbit Chat Backend

Goで実装したリアルタイムチャットのAPIサーバーです。

## API

```text
GET    /api/health
POST   /api/auth/register
POST   /api/auth/login
POST   /api/auth/logout
GET    /api/auth/me
PATCH  /api/auth/me                 (要ログイン・表示名変更)
GET    /api/channels                 (要ログイン)
POST   /api/channels                 (要ログイン)
GET    /api/channels/:channel_id/messages (要ログイン)
POST   /api/channels/:channel_id/messages (要ログイン)
POST   /api/channels/:channel_id/read (要ログイン)
POST   /api/channels/:channel_id/summary (要ログイン・会話の要点)
GET    /api/events?after=:cursor    (要ログイン・差分同期)
GET    /api/messages/:message_id/replies (要ログイン・スレッド取得)
PATCH  /api/messages/:message_id     (要ログイン・所有者のみ)
DELETE /api/messages/:message_id     (要ログイン・所有者のみ)
POST   /api/messages/:message_id/reactions (要ログイン)
DELETE /api/messages/:message_id/reactions?emoji=:emoji (要ログイン)
GET    /ws?channel_id=:channel_id    (要ログイン)
GET    /ws?channel_id=all            (要ログイン)
```

Orbit AIとの個別メッセージでは、ユーザーのメッセージを受けて次の一時的なWebSocketイベントを配信します。

```text
message.ai_started
message.ai_delta
message.ai_completed
message.ai_failed
```

最終回答は `messages` と `chat_events` に通常のメッセージとして保存します。Provider未設定またはAPIキー未設定ならモックプロバイダで動作し、設定時はGoサーバーからOpenAI互換のストリーミングAPIを呼び出します。APIキーはフロントエンドへ返しません。

`POST /api/channels/:channel_id/summary` は未読メッセージ（スレッド返信を含む）を優先してAIへ渡し、未読がなければ最近のメッセージへフォールバックします。`summary`、`decisions`、`action_items`、`unresolved`、`chatter_count`、`scope`を構造化して返します。分類項目には現在のコンテキストに存在する元メッセージIDだけを付けるため、フロントエンドから原文へ戻れます。Decision Memoryは次の段階です。

`AI_PROVIDER`には `openai`、`gemini`、`ollama`、`openrouter`、`bedrock`、`mock`を指定できます。`AI_API_KEY`、`AI_MODEL`、`AI_BASE_URL`で共通設定でき、従来の`OPENAI_*`環境変数も後方互換で利用できます。Bedrockは`bedrock-mantle.<region>.api.aws`のChat Completions互換エンドポイントを使用します。

WebSocketは、同じチャンネルに接続しているクライアントへ次のイベントをJSONで配信します。

チャンネル作成・rename・参加者変更も`channel.created`、`channel.updated`、`channel.member_added`、`channel.member_removed`として永続化し、開いているクライアントのサイドバーへ反映します。除外されたユーザーには`member_removed`だけを届け、差分取得後にチャンネルを一覧から外します。

```json
{
  "type": "message.created",
  "channel_id": "general",
  "event_id": 101,
  "message": { "id": "m-1", "body": "hello" }
}
```

`DATABASE_URL`を指定すると、`cmd/server/migrations`のversioned SQL migrationを未適用分だけ実行し、ユーザー、HTTP-only Cookieセッション、チャンネル、メッセージを保存します。開発環境では未指定時にインメモリストアへフォールバックしますが、`APP_ENV=production`（または `prod`）では`DATABASE_URL`と`COOKIE_SECURE=true`が必須です。ログイン・登録にはIPアドレスとメールアドレスを組み合わせたレート制限があります。AI呼び出しには同時実行・最短間隔に加えて、ユーザー単位の日次上限（`AI_DAILY_REQUEST_LIMIT`、既定100）があります。

リバースプロキシ配下で信頼できる`X-Forwarded-For` / `X-Real-IP`を使う場合だけ、`TRUST_PROXY_HEADERS=true`を設定します。既定ではこれらのヘッダーを信用せず、TCP接続元をレート制限のIPとして使います。

チャンネルは`channel_members`でアクセス制御します。既定の公開チャンネル（`general`、`frontend`、`design-system`、`roadmap`、`research`）には登録時に参加しますが、ユーザーが作成したチャンネルは作成者とOrbit AIだけが初期メンバーです。新規ユーザーを既存の全チャンネルへ自動参加させることはありません。メッセージ履歴、スレッド、リアクション、イベント差分、チャンネル別WebSocket購読はmembershipを確認してから返します。

WebSocket broadcastでは、イベントごとにチャンネルのmembership snapshotを1回取得し、Hub内で接続ユーザーを照合します。接続クライアント数に比例したDB問い合わせは発生しません。

WebSocketの送信バッファが満杯になったslow clientは切断します。フロントエンドは再接続時のイベント差分再生に加え、受信した`event_id`の飛び（gap）も検知して`/api/events?after=`から回復します。イベントを黙って捨ててカーソルだけ進めない設計です。

メッセージ履歴は `?limit=50&before=:cursor` で古いページを取得できます。全チャンネルのリアルタイム差分は `GET /api/events?after=:event_id` で取得し、`POST /api/channels/:channel_id/read` で`channel_read_states.last_read_sequence`を更新します。`last_read_message_id`は参照用に保持します。

Typing / PresenceはDBへ保存しない一時イベントです。WebSocketへ次のJSONを送ると、他の接続へ配信されます。

```json
{"type":"typing.started","channel_id":"general"}
{"type":"typing.stopped","channel_id":"general"}
{"type":"presence.changed","presence":"away"}
```

メッセージは`parent_message_id`を指定すると1階層のスレッド返信になります。thread rootを削除するとrootはsoft deleteされ、「このメッセージは削除されました。」として返信を保持します。返信自体を削除した場合だけ返信行を削除し、root一覧へ昇格させません。リアクションは`message_reactions`の主キー（`message_id, user_id, emoji`）で同一ユーザーの二重追加を防ぎ、メッセージ行をロックしたトランザクション内で集計を更新します。

## 起動

```powershell
$env:APP_ENV = "development"
docker compose up -d postgres
$env:DATABASE_URL = "postgres://orbit:orbit@127.0.0.1:5432/orbit_chat?sslmode=disable"
go run ./cmd/server
```

既定ポートは `8080` です。変更する場合は `PORT=8081` を指定します。

## テスト

```powershell
go test ./...
```

race detectorはCGOが有効な環境で実行します。

```powershell
go test -race ./...
```

PostgreSQLの認可統合テストは、`POSTGRES_TEST_DATABASE_URL`を指定した場合に実行されます。CIではPostgreSQL 16を起動して実行します。

デモログインは`APP_ENV=development`、`APP_ENV=test`、または`SEED_DEMO_DATA=true`でデモseedを有効にした場合だけ利用できます。本番環境ではmigrationのみ実行され、既知パスワードのユーザーは自動作成されません。パスワードはbcryptでハッシュ化され、Cookieにはパスワードやユーザー情報を保存しません。

Orbit AIの環境変数は `.env.example` を参照してください。Gemini、OpenRouter、BedrockのAPIキーはバックエンドの環境変数だけに置き、OllamaではローカルのHTTP APIを利用します。
