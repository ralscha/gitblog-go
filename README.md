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
