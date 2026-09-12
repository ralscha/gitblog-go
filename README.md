# gitblog-go

Go application that builds and serves [blog.rasc.ch](https://blog.rasc.ch) from a Git-backed Markdown post repository.

## What it does

- Pulls Markdown posts from the configured Git repository.
- Converts posts with Goldmark, GitHub code embeds, Mermaid diagrams, emoji, anchors, and Shiki syntax highlighting.
- Writes HTML plus gzip and Brotli variants next to the source posts.
- Generates RSS, Atom, JSON feed, and sitemap files.
- Indexes published posts in Meilisearch for year, tag, and free-text search.
- Serves the dynamic index, feedback form, and GitHub webhook endpoint.
- Can run a broken-link report for generated posts.

## Requirements

- Go 1.27.1 or newer matching `go.mod`.
- Node.js and npm only when rebuilding the CSS assets.
- Docker for local Meilisearch, Inbucket, and Mermaid rendering.
- Taskfile is optional, but the common commands are defined in `Taskfile.yml`.

Syntax highlighting uses [shiki-go](https://github.com/ralscha/shiki-go) in the Go process, with embedded grammars and the `one-light` / `one-dark-pro` themes. Running the blog does not require Node.js or a separate highlighting CLI.

Install Node dependencies if you need to rebuild the CSS assets:

```sh
cd css_build && npm install
```

## Configuration

The app reads `app.env` from the project root when present and also accepts environment variables with the `GOLB_` prefix, so an environment-only deployment is supported. Dots in config keys become underscores, for example `GOLB_HTTP_PORT=localhost:8080` and `GOLB_BLOG_POSTDIR=./posts`.

Important keys:

```properties
http.port=localhost:8080
smtp.host=localhost
smtp.port=2500
smtp.tlsPolicy=none
smtp.sender=me@example.com

github.url=git@github.com:owner/posts.git
github.webhookSecret=secret
github.privateKey=/path/to/private/key

blog.secret=secret
blog.postDir=./posts
blog.url=http://localhost:8080
blog.title=My Blog
blog.description=My Blog
blog.author=me

meilisearch.host=http://127.0.0.1:7799
meilisearch.key=MASTER_KEY
```

`blog.url` may be configured with or without a trailing slash.

`github.privateKey` is used for SSH URLs. Public HTTPS and local repository URLs do not need an SSH key. The posts directory may be absent or empty for the first clone; an existing nonempty directory must be a Git checkout. For SSH, the service account also needs a trusted host entry in its `known_hosts` file.

Set `github.webhookSecret` to the same nonempty secret configured in GitHub. The callback returns HTTP 503 when this setting is empty.

`smtp.tlsPolicy` defaults to `mandatory` (STARTTLS required). Use `none` for the local Inbucket service; `opportunistic` is also supported for servers where TLS is optional. The checked-in development configuration uses `none`; configure the appropriate policy for your production SMTP server.

## Local Development

Start supporting services:

```sh
docker compose up -d
```

Run the server:

```sh
go run gitblog/cmd/blog
```

Useful commands:

```sh
task tidy              # go fmt and go mod tidy
task test              # run the Go test suite
task audit             # go vet, staticcheck, go mod verify
task build             # build the blog binary
task build-linux-amd64 # cross-compile for Linux amd64
go run gitblog/cmd/blog index  # rebuild the Meilisearch index
go run gitblog/cmd/blog rebuild # force regeneration of all HTML, feeds, sitemap, and the search index
go run gitblog/cmd/blog report # run the broken-link report
```

The server exposes `GET /healthz` for process-level health checks.

The Go server handles dynamic routes. Run Caddy with the supplied `caddyfile` to serve generated posts, images, stylesheets, and feeds as well. Static assets, including `assets/blog-9.css` and its fonts, must be present in the posts checkout.

## Post Format

Posts are Markdown files in `blog.postDir` with YAML front matter:

```md
---
title: Example post
published: 2026-06-21T10:00:00Z
updated: 2026-06-21T12:00:00Z
tags:
  - go
summary: Short summary shown on the index page
draft: false
---

Post body goes here.
```

Draft posts are skipped. `DRAFT.md` files are ignored.

## Deployment Notes

`caddyfile` serves generated files from `posts`, uses precompressed Brotli/gzip assets, hides Markdown and `.git` files, and reverse proxies the dynamic routes to the Go server on `localhost:8080`.

The GitHub webhook endpoint is `POST /githubCallback`. Push events trigger a background refresh of posts, feeds, sitemap, and search index.

Each refresh retries publication even if the Markdown has not changed, and repairs missing or stale compressed pages. Search replacements are built in temporary `posts_build_*` indexes and swapped into `posts` after successful indexing. The Meilisearch API key must permit index creation/deletion, settings, documents, tasks, and index swaps for both names. Allow space for two copies of the index during publication. Run only one publishing process against a posts checkout at a time.

After changing templates or the syntax highlighter, run `rebuild` once to regenerate existing HTML. Restarting alone only converts Markdown that has changed.

