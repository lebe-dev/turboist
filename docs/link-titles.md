# Link titles

Paste a link as the title of a new task and the server names the task after the
page it points to:

| You type | The task becomes |
|---|---|
| `https://habr.com/ru/articles/123456/` | **Статья «Как устроен WAL в SQLite»** |
| `https://www.youtube.com/watch?v=…` (with a `youtube.com` rule) | **Посмотреть «Название ролика»** |

The link moves into the description, followed by the page's own summary
(`og:description`) and whatever the description already said.

Off by default. Turn it on in **Settings → Inbox → Link titles** on the web, or
**Settings → Rules → Link titles** in the native Android app.

## What counts as a link

The title — after trimming — must be one `https://` URL and nothing else:

- `https` only; `http://` links are left alone.
- A public domain name: no IP addresses, no single-label hosts (`router`), no
  reserved names (`localhost`, `*.local`, `*.internal`, `*.lan`, `*.home.arpa`, …).
- No port other than 443, no `user:password@`.

Anything else — a link with words around it, a plain `http://` link — is a
normal task.

## Templates

A template is the task title with `{title}` standing for the page title and,
optionally, `{domain}` for the link's host (without `www.`).

- **Default template** — used for every link no rule matches. Empty means the
  built-in default for your interface language: `Статья «{title}»` (ru),
  `Article “{title}”` (en).
- **Domain rules** — a domain and its own template, e.g. `youtube.com` →
  `Посмотреть «{title}»`. A rule matches the domain and every subdomain
  (`youtube.com` covers `www.youtube.com` and `m.youtube.com`, not
  `notyoutube.com`); the most specific match wins. Paste a URL into the domain
  field and it is reduced to the bare domain.

- **Built-in rules** — `github.com` (and its subdomains) uses the bare `{title}`,
  so a shared repository or issue is named exactly like its page. They apply
  after your own rules and before the default template; add your own rule for
  the same domain to change one.

When no title can be found the template is filled with the host and path,
e.g. `Статья «example.com/blog/post»`.

## How it works

Everything happens on the server, after the task is created — the clients only
store the switch and the templates.

1. `TaskService.Create` sees a bare link and, if the feature is on, puts the task
   into `link_title_queue`.
2. A background worker (`internal/service/linktitle`) fetches the page and reads
   its `<head>`: `og:title`, then `twitter:title`, then a JSON-LD `headline`, then
   `<title>`. The site name is stripped from the end or start (`… | Habr`).
3. If the page carries no title — typically a single-page app that builds it in
   JavaScript — and a renderer is configured, the worker asks the renderer.
4. The task is re-read; if its title is still the bare link, it is rewritten and
   every client is told over SSE. If you renamed it in the meantime, nothing is
   touched.

Network failures, timeouts, `429` and `5xx` are retried twice (after 1 and
5 minutes); after that, or right away for a `404` or a non-HTML page, the task
gets the fallback title.

### Offline

A task created offline — in the web app's outbox or the Android app's local
copy — is a bare link until it syncs. It is named when the server receives it,
and the new title arrives through the normal sync a few seconds later.

### Inbox processing

The LLM Inbox processor skips a task while it waits for its title, so it never
decides where a bare URL belongs; it sees the task once it has been renamed.

## Sharing into Turboist

- **Android app** — *Share → Turboist* from any app opens quick add. With link
  titles on, the shared `https://` link becomes the title (so the server names
  it) and the page name the sharing app sent goes to the description; with link
  titles off the page name stays the title, as before.
- **Installed web app (PWA)** — Chrome on Android and desktop list the installed
  app in the share sheet (Web Share Target). A share opens quick add the same
  way.

## Rendering fallback

Most sites — single-page apps included — put `og:title` into their initial HTML
for link previews, so the plain fetch is enough nearly always. For the rest you
can point the server at an external "URL → Markdown" service:

```bash
LINK_TITLES_RENDERER_URL=https://markdown.new/
```

The link is appended to the URL, or substituted for `{url}` when the template
contains it (`https://render.example/md?url={url}`). The worker understands
YAML front matter (`title:`), Jina-style `Title:` headers and, failing both, the
first `# heading`. Any service with that shape works: markdown.new, Jina Reader
(`https://r.jina.ai/`), or your own headless browser next to Turboist.

> **Privacy.** With a renderer configured, links whose page carries no title are
> sent to that service. Without one, only the sites you link to see a request.

## Security

The server fetches URLs that anyone who can create a task chooses, so the
fetcher is locked down:

- the URL checks above run on the link and on **every redirect** (at most 5);
- the dialer checks the **resolved address** right before connecting and
  refuses loopback, private, link-local (including `169.254.169.254`), CGNAT,
  multicast, documentation and other non-public ranges — so a public name that
  resolves, or re-resolves, into your network is refused too;
- no proxy is used for these fetches, so the address check sees the real
  destination;
- 12 s per fetch, 1 MiB read, and only `text/html` / `application/xhtml+xml`.

The renderer is configured by the operator and is reached through the normal
HTTP client, so it may live on a private network.
