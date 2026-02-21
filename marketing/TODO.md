# Gollem & Monty-Go Marketing — Remaining Manual Steps

*Last updated: 2026-02-21*

## CLAs to Sign
- [x] **LiteLLM CLA** — Signed at https://cla-assistant.io/BerriAI/litellm?pullRequest=21747
- [ ] **CNCF CLA** — Required before filing OTEL proposal issue on open-telemetry/opentelemetry-go-contrib

## Issues to File
- [ ] **OTEL Go Contrib proposal** — Copy `marketing/otel-proposal-issue.md` to a new issue on `github.com/open-telemetry/opentelemetry-go-contrib` (needs CNCF CLA first)
- [x] **LangFuse Go SDK announcement** — https://github.com/langfuse/langfuse/issues/12189

## Content to Post (stagger across different days for max exposure)
- [ ] **Show HN** — Post `marketing/show-hn.md` to news.ycombinator.com (best: 9-11am ET weekday)
- [ ] **Go Weekly** — Submit gollem to https://golangweekly.com/submit using `marketing/go-weekly-submission.md`
- [ ] **r/golang** — Post `marketing/reddit-r-golang.md` (best: Tue-Thu)
- [ ] **r/LocalLLaMA** — Post `marketing/reddit-r-localllama.md` (best: Tue-Thu, different day from r/golang)
- [ ] **dev.to blog** — Publish `marketing/blog-go-vs-python-agents.md`, cross-post to personal blog, share in HN/Reddit threads

## Framework Ecosystem PRs
- [x] **chi** — https://github.com/go-chi/chi/issues/1055
- [x] **gin** — https://github.com/gin-gonic/contrib/pull/233
- [x] **echo** — https://github.com/labstack/echo/issues/2904
- [x] **fiber** — https://github.com/gofiber/awesome-fiber/pull/52

## GitHub Action Marketplace
- [ ] **Test gollem-review-action on a real PR** — Use `act` locally or test on a real PR in a test repo
- [ ] **Publish gollem-review-action** — Tag v1 on `fugue-labs/gollem-review-action` to publish to GitHub Marketplace

## MCP Registry
- [ ] **Test gollem-mcp-server with Claude Desktop** — Verify it works as an MCP server in Claude Desktop config
- [ ] **Register gollem-mcp-server** — Follow `github.com/modelcontextprotocol/registry` quickstart to publish

## Deferred (July 2026)
- [ ] **awesome-go** — Submit gollem to `github.com/avelino/awesome-go` Artificial Intelligence section after 5-month repo history requirement is met (~July 2026). Entry prepared in plan.

## Open PRs to Monitor

All PRs are open, waiting on maintainer review. CI is clean on our end (LiteLLM CI failures are all pre-existing on their main branch).

| PR | Target | Status |
|----|--------|--------|
| https://github.com/mbasso/awesome-wasm/pull/252 | awesome-wasm | Open, waiting on maintainer |
| https://github.com/ollama/ollama/pull/14345 | Ollama README | Open, waiting on maintainer |
| https://github.com/BerriAI/litellm/pull/21747 | LiteLLM cookbook | Open, CLA signed, CI failures are theirs |
| https://github.com/e2b-dev/e2b-cookbook/pull/85 | E2B cookbook | Open, waiting on maintainer |
| https://github.com/tetratelabs/wazero/pull/2477 | wazero examples | Open, waiting on maintainer |
| https://github.com/promptfoo/promptfoo/pull/7803 | Promptfoo docs | Open, Prettier fixed |
| https://github.com/gin-gonic/contrib/pull/233 | gin contrib | Open, waiting on maintainer |
| https://github.com/gofiber/awesome-fiber/pull/52 | awesome-fiber | Open, waiting on maintainer |

## Open Issues to Monitor

| Issue | Target | Status |
|-------|--------|--------|
| https://github.com/go-chi/chi/issues/1055 | chi | Open |
| https://github.com/labstack/echo/issues/2904 | echo | Open |
| https://github.com/langfuse/langfuse/issues/12189 | langfuse | Open |

## New Repos Created

| Repo | Description | Status |
|------|-------------|--------|
| https://github.com/fugue-labs/langfuse-go | LangFuse Go SDK with gollem middleware | All tests pass |
| https://github.com/fugue-labs/gollem-review-action | GitHub Action for AI PR review | All tests pass, needs Marketplace publish |
| https://github.com/fugue-labs/webhook-agents-demo | Multi-tenant webhook agents demo | Builds clean |
