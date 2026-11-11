# johannestrepesch.de

[![Build Status](https://github.com/HannesT117/johannestrepesch.de/actions/workflows/deploy.yml/badge.svg)](https://github.com/HannesT117/johannestrepesch.de/actions/workflows/deploy.yml)

Personal homepage: a static page built with Vite (plain HTML, CSS and a little JS) and a small Go service behind `/api` for the query box.

## Development

```sh
pnpm install
pnpm dev                                       # site on http://localhost:5173, proxies /api to :8787
cd api && go run ./cmd/portfolio-api -env dev.env -content internal/content/testdata/content.example.json
cd api && go test -race ./...
```

`dev.env` uses the keys described in [docs/server-setup.md](docs/server-setup.md) (for example `OPPER_API_KEY`, `OPPER_MANAGEMENT_KEY`, `GARMIN_API_KEY`). Never commit it. A missing content file disables the query box.

The board (coffee, music, sport, reading) is rendered by the Go service, so `pnpm dev` shows the page without it. To see the full page:

```sh
pnpm build && cp dist/index.html dist/page.html
cd api && go run ./cmd/portfolio-api -env dev.env -extras path/to/extras.json -page ../dist/page.html -out ../dist/index.html
pnpm preview                                   # in another shell, serves dist/
```

## Deploy

Every push to `main` runs the API tests, builds the site and the API binary, and sets up Uberspace from GitHub secrets: the env file, content, extras, the service and the `/api` backend. The secrets and one-time steps are in [docs/server-setup.md](docs/server-setup.md).
