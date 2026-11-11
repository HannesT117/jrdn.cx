# Server setup (Uberspace)

The deploy workflow (`.github/workflows/deploy.yml`) sets up everything on the server on every push to `main`: the static site, the API binary, `~/etc/portfolio-api.env`, the content and extras files, the supervisord service and the `/api` web backend. Nothing is edited by hand on the server.

The service renders the board (coffee, music, sport, reading) into the built page at its `<!--board:…-->` markers and writes `~/html/index.html` on start and every 15 minutes. Apache serves that file like any other static file. The deploy uploads the new assets, waits for the service to render the page against them, and only then deletes the old assets.

## 1. GitHub secrets and variables

Settings → Secrets and variables → Actions.

| Secret | Purpose |
|---|---|
| `HOST`, `USER`, `SSH_KEY`, `HOST_KEY` | SSH access for the deploy (see 6) |
| `OPPER_API_KEY` | Query box model calls (see 2) |
| `OPPER_MANAGEMENT_KEY` | Reads the project's spend for the daily budget (see 2) |
| `CONTENT_JSON` | Answers, strengths and learning areas for the query box (see 4) |
| `EXTRAS_JSON` | Coffee, bikes, books, podcasts, repertoire, concerts and gigs (see 5) |
| `GARMIN_API_KEY` | Sport timeline (see 3) |

| Variable (optional) | Purpose |
|---|---|
| `API_ENABLED` | `false` switches the query box off without removing secrets |
| `ALLOWED_ORIGINS` | Comma-separated origins the query box accepts. Defaults to `johannestrepesch.de`, `jrdn.cx` and `trepesch.dev`, each with and without `www` |
| `CLIENT_IP_HEADER` | Header in which Uberspace's proxy passes the visitor's IP. Without it every visitor counts against the same per-client cap. `GET /api/whoami` shows what reaches the service for your own request |
| `DAILY_BUDGET_USD`, `OPPER_PROJECT` | Daily spend limit in USD and the Opper project it applies to (defaults 2 and `Portfolio`) |
| `DAILY_CAP_PER_CLIENT`, `DAILY_CAP_GLOBAL` | Model calls per visitor and in total per day (defaults 20 and 300) |

The query box is on when `CONTENT_JSON`, `OPPER_API_KEY` and `OPPER_MANAGEMENT_KEY` exist and `API_ENABLED` is not `false`. The HMAC secret for the rate limiter is generated on every deploy, because the limiter only keeps hashed client keys in memory and resets on restart anyway. The deploy validates the env file, content and extras before it touches the server and stops on invalid data.

## 2. Opper (once)

1. Create an Opper account, add a card (Clef is a premium model, so the API returns 402 without a card), and create a project `Portfolio` with an API key (`op-…`). Store it with `gh secret set OPPER_API_KEY` (it prompts for the value).
2. Create a management key (`op-mak-…`) with only the scopes `usage:read` and `projects:read`, and store it with `gh secret set OPPER_MANAGEMENT_KEY`. The service checks the project's spend for the current UTC day at most once a minute and rests the query box once it reaches `DAILY_BUDGET_USD`. If Opper's usage figure is older than ten minutes, the box rests too. The call that crosses the limit still completes, so the day can end slightly above it.
3. The service calls `melious/clef`, an EU route (Finland) with zero data retention. That route has no failover: when Melious is down, the API answers `unavailable`.

## 3. Garmin (once a year)

The sport node shows activities from the last four calendar days (Sydney time) from Garmin Connect's Personal API: only the kind (run, ride, swim, other), the start rounded to 15 minutes and the duration. Names, distance and routes never leave the server.

1. Create a personal API key at https://connect.garmin.com/app/settings/personalApiToken (accept the terms, pick the longest lifetime). The key is shown only once.
2. Store it and deploy: `gh secret set GARMIN_API_KEY && gh workflow run deploy.yml` (the first command prompts for the value).

The service trades the key for a 24-hour access token itself and asks Garmin at most every 15 minutes. Without the key the page leaves the timeline out.

The key expires after the lifetime you picked; Garmin emails a reminder about a week before. When it has expired the log shows `garmin rejected the personal API key`. Create a new key and repeat step 2.

## 4. Content

The format is the same as `api/internal/content/testdata/content.example.json`.

```sh
gh secret set CONTENT_JSON < path/to/content.json && gh workflow run deploy.yml
```

## 5. Extras

Everything on the now board comes from one JSON file. Keep it outside this repository, edit it there and publish it:

```sh
gh secret set EXTRAS_JSON < path/to/extras.json && gh workflow run deploy.yml
```

```json
{
  "beans": [{ "name": "…", "origin": "…", "process": "washed", "roaster": "…" }],
  "pastBeans": [],
  "gear": [{ "kind": "grinder", "name": "…" }],
  "brews": ["V60"],
  "bikes": [{ "kind": "road", "name": "…", "details": ["…"] }],
  "sportGear": [{ "kind": "watch", "name": "…" }],
  "music": [{ "kind": "piano", "name": "…" }],
  "books": [{ "title": "…", "author": "…", "started": "2031-01-01" }],
  "pastBooks": [],
  "podcasts": [{ "name": "…", "host": "…" }],
  "gigs": [{ "date": "2031-05-01", "program": "…", "choir": "…", "place": "…" }],
  "repertoire": [{ "title": "…", "composer": "…", "year": 1840, "approx": true, "form": "Motet", "scoring": "SATB", "album": "…" }],
  "concerts": 400,
  "stages": [{ "year": 2005, "title": "…", "place": "…", "with": "…" }],
  "countries": ["…"]
}
```

Limits: 4 open beans, 3 open books, 6 finished beans and books each, 8 gigs, 400 pieces, 80 characters per text. Gigs disappear the day after their date. Optional fields: `process`, `details`, `host`, `choir`, `approx`, `scoring`, `album`, `year`, `place` and `with` on stages. Check a file locally with `go run ./cmd/portfolio-api -check -content internal/content/testdata/content.example.json -extras path/to/extras.json` in `api/`. GitHub secrets hold at most 48 KB.

Everything in this file ends up in the public page.

## 6. Deploy key

The deploy workflow restarts the service after copying the binary (`supervisorctl restart portfolio-api`). The deploy key therefore needs shell access. A forced-command key that only allows rsync is not enough.
