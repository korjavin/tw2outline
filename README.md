# tw2outline

Polls your Twitter/X bookmarks on a schedule and saves each one as a document in [Outline](https://www.getoutline.com/).

Combines the Twitter fetching logic from [tw2dynalist](https://github.com/korjavin/tw2dynalist) with the Outline publishing logic from [tg2outline](https://github.com/korjavin/tg2outline).

## How it works

1. Authenticates with Twitter via OAuth 2.0 PKCE (browser-based, one-time).
2. Polls your bookmarks every `CHECK_INTERVAL` (default: 1 h).
3. For each new bookmark, creates an Outline document:
   - **Title**: first 80 chars of the tweet text
   - **Body**: full tweet text + a `[Source]` link back to the tweet
4. Keeps a local JSON cache so duplicates are never re-sent.
5. Optionally removes the bookmark from Twitter after saving (`REMOVE_BOOKMARKS=true`).
6. Optionally sends a push notification via [ntfy](https://ntfy.sh/).
7. Exposes a status dashboard at `http://localhost:8080/` and JSON metrics at `/api/metrics`.

## Quick start

### 1. Twitter OAuth app

Create a Twitter Developer App at <https://developer.twitter.com/> with:
- **OAuth 2.0** enabled
- **Callback URL**: `http://localhost:8080/callback` (or your public domain)
- **Scopes**: `tweet.read`, `users.read`, `bookmark.read`, `bookmark.write`, `offline.access`

### 2. Outline API token

In Outline: **Settings → API Tokens → New token**.

To find your collection ID, open the collection in Outline and copy the UUID from the URL.

### 3. Configure

```bash
cp .env.example .env
# Edit .env with your values
```

### 4a. Run locally

```bash
go run .
# Visit the auth URL printed to the log, authorize, then the bot starts polling.
```

### 4b. Run with Docker Compose

```bash
docker compose up -d
```

Open `http://localhost:8080` in your browser, follow the auth URL printed in the logs.

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `OUTLINE_URL` | ✅ | — | Base URL of your Outline instance |
| `OUTLINE_API_TOKEN` | ✅ | — | Outline Bearer token |
| `OUTLINE_COLLECTION_ID` | ✅ | — | Target collection UUID |
| `TWITTER_CLIENT_ID` | ✅ | — | Twitter OAuth2 client ID |
| `TWITTER_CLIENT_SECRET` | ✅ | — | Twitter OAuth2 client secret |
| `TW_USER` | ✅ | — | Your Twitter username (for user ID lookup) |
| `TWITTER_REDIRECT_URL` | — | `http://localhost:8080/callback` | OAuth callback URL |
| `CHECK_INTERVAL` | — | `1h` | How often to poll bookmarks (Go duration) |
| `REMOVE_BOOKMARKS` | — | `false` | Delete bookmark after saving to Outline |
| `CLEANUP_PROCESSED_BOOKMARKS` | — | `false` | On startup, remove already-cached bookmarks |
| `CALLBACK_PORT` | — | `8080` | HTTP server port |
| `CACHE_FILE_PATH` | — | `cache.json` | Path to processed tweet ID cache |
| `TOKEN_FILE_PATH` | — | `token.json` | Path to persist OAuth2 token |
| `LOG_LEVEL` | — | `INFO` | `DEBUG` / `INFO` / `WARN` / `ERROR` |
| `NTFY_SERVER` | — | `http://ntfy:80` | ntfy server URL |
| `NTFY_TOPIC` | — | `tw2outline` | ntfy topic |
| `NTFY_USERNAME` | — | — | ntfy basic auth username |
| `NTFY_PASSWORD` | — | — | ntfy basic auth password |

## Building

```bash
go build -o tw2outline .
```

## License

MIT
