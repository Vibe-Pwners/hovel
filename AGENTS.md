# Agent instructions for Hovel

These instructions apply to any AI agent or automation working in this
repository. (`CLAUDE.md` is a symlink to this file.)

## The one rule for the build system

**Always invoke the build system through [Aspect CLI](https://aspect.build/cli)
(`aspect <command>`). Never call `bazel`, `gofmt`, `uv`, or `lefthook` directly.**

The `.aspect/` command configuration is the single source of truth for how the project is built,
tested, linted, formatted, and run. CI, the git hooks, the docs, and you all go
through it. If something cannot be done with an existing command, add or fix an
AXL command rather than running the underlying tool ad hoc.

Run `aspect help` to see everything available.

## Shell in the build graph

Do not add shell scripts to the build, lint, test, docs, or demo pipeline by
default. Hovel uses Bazel/Starlark for declared build actions and Python for
non-trivial repository tooling. Shell is acceptable only when the shell itself
is the thing being demonstrated or operated, such as VHS terminal recordings,
Docker entrypoints, or one-off lab operator scripts.

When a check can be cached, model it as a Bazel target with explicit inputs and
outputs instead of discovering files from shell at execution time. When a task
must materialize generated artifacts back into the working tree (`_site/`,
`docs/demo/out/`, `modules/examples/bin/`), prefer a `bazel run` Python
materializer with declared data dependencies. Do not call `bazel` from helper
scripts.

Build, test, lint, docs, and demo tools should be Bazel-managed execution
inputs whenever practical: pinned archives, pip wheels, Go/Rust toolchains, or
custom toolchains/rules. Do not add new `PATH` lookups, `go install`, `uv tool
install`, Homebrew, or apt-installed CLIs for tools that can reasonably be
declared in Bazel. Host tools are acceptable only for real host services or
system boundaries that Bazel should not own, such as Docker, Wine, tmux, ttyd,
or ffmpeg until they have a pinned execution toolchain.

## Common tasks

| Command | What it does |
| --- | --- |
| `aspect help` | List all commands. |
| `aspect build` | Build the core Hovel binary workspace. |
| `aspect build @hovel_core//cmd/hovel` | Build the Hovel CLI target. |
| `aspect test` | Run core Hovel binary workspace tests. |
| `aspect test //internal/domain/...` | Run specific tests. |
| `aspect hovel status` | Run a Hovel development command. |
| `aspect hovel-check core` | Core Go formatting + golangci-lint + Gazelle checks (read-only). |
| `aspect hovel-format` | Auto-format wired slices: core Go/Gazelle plus Go SDK sources. |
| `aspect test --coverage` | Run core domain and application coverage ratchets. |
| `python3 repo-tools/tasks/checkout.py status` | Show which repository slices are present. |
| `aspect hovel-check` | Run checks for slices present in the current checkout. |
| `aspect hovel-check` | Require a full checkout, then run the wired full gate. |
| `aspect run //docs/tools/docs:stage_site` | Build and materialize the docs site to `_site/`. |
| `aspect hovel-check docs` | Build and validate the hermetic Astro docs artifact. |
| `aspect run //docs/site:dev` | Run the Bazel-managed Astro development server on port 4321. |
| `aspect run //docs/tools/docs:preview_site` | Serve the materialized `_site/` on port 4322. |
| `aspect hovel-report` | Run report-producing tests and build `_site/` with the latest evidence. |
| `aspect hovel cli` (`cli`) / `aspect hovel daemon` | Launch the interactive CLI / the daemon. |
| `aspect hovel status` / `aspect hovel init` | Dev workspace status and initialization. |

## Definition of done

Before considering a code change complete, run the strongest Aspect-backed gate
available for the checked-out slices. For core changes, run **`aspect hovel-check`** from a
full checkout or **`aspect hovel-check core`** from a core-only checkout. The wired gate
currently covers core lint, build, tests, race, fuzz smoke, and coverage.

If you added, moved, or removed Go files or imports, run **`aspect hovel-format`** so
`gofmt` and Gazelle-generated `BUILD.bazel` files are up to date; otherwise
`aspect hovel-check core` will fail on the Gazelle diff check. When you add a new core test
target, also add it to the `test_suite` in `core/BUILD.bazel`.

## Docs authoring

Agents are expected to edit docs as literal HTML fragments under
`docs/site/src/content/`. Do not edit `_site/`, generated API reference output,
or reintroduce complete HTML documents into content files.

Every content file starts with one JSON metadata comment, followed by ordinary
HTML containing exactly one page-level `h1`:

```html
<!-- hovel-doc: {"title":"Example","group":"Foundations","order":120,"navTitle":"Example"} -->
<article>
  <h1>Example</h1>
  <p>Page content.</p>
</article>
```

- The path below `src/content/` determines the output URL. For example,
  `spec/example.html` builds as `_site/spec/example.html`.
- `group` and `order` place the page in generated contents, sidebar, chapter
  numbering, and previous/next navigation. Book groups are `Foundations`,
  `Runtime Platform`, `Operator Experience`, `Module Development`,
  `Engineering`, and `Reference`; `Contents` is reserved for `spec/index.html`.
  Orders must be unique within a group. Never write chapter or section numbers
  by hand. `navTitle` and `description` are optional.
- Write normal HTML. Inline browser JavaScript is allowed, and shared scripts,
  styles, images, or other static files belong under `docs/site/public/`.
- Global chrome lives in `src/components/` and `src/layouts/`. Change it once
  there; never copy headers, sidebars, footers, or page navigation into content.
- Module overview pages live at `src/content/modules/<module>/index.html` and
  additionally declare `moduleOrder`, `moduleType`, `moduleStatus`, and
  `description`. Use the module id as `group` on every page in its document set;
  Astro generates module/document numbering and all module navigation.
- Search is generated automatically from page metadata and body text. Keep
  titles and optional descriptions concrete so client-side search results stay
  useful; do not hand-maintain a separate search index.
- API landing metadata lives in `src/lib/apiReferences.ts`. Astro owns the API
  landing pages and shared Hovel chrome; generated Sphinx, pkgsite, and rustdoc
  interiors keep their native navigation and must not reimplement that chrome.
- The daemon OpenAPI source lives only at
  `docs/site/spec/reference/daemon-rpc.openapi.json`. The Bazel
  `//docs/site:daemon_rpc_openapi` target publishes it into Astro's
  `public/spec/reference/` path; never check in or edit a second public copy.
- The home page contains exactly one
  `<div data-hovel-component="demo-carousel"></div>` marker. Its reusable Astro
  component is `src/components/DemoCarousel.astro`, while the ordered demo data
  lives in `src/pages/index.astro`.
- Keep the build hermetic: do not add CDN assets, runtime network dependencies,
  or host `node`, `npm`, `pnpm`, or Python package assumptions. Update
  dependencies with the Bazel-managed dependency targets through `aspect run`;
  refresh both the checked-in JavaScript and hashed Python locks.
- After docs changes, run `aspect hovel-check docs`. Use `aspect run //docs/tools/docs:stage_site` when the root
  `_site/` materialization is required.
- Test evidence is generated after Bazel finishes. Use `aspect hovel-report` when
  `_site/reports/tests/latest/` must contain the latest monorepo test report.
- The normal docs staging target is deterministic and does not consume ambient
  `.test-report/` files. Astro owns the report HTML; report builds attach only
  generated JSON, logs, XML, and artifacts.
- CI uploads the evidence-backed `_site/` from `aspect hovel-report` as the
  `docs-site` artifact. The Pages workflow promotes that exact artifact after a
  successful `main` CI run; manual Pages dispatches run the same Aspect contract.

## Architecture guardrails

Hovel uses a hexagonal layering with dependencies pointing inward:

```
adapters -> app -> domain
infra    -> app -> domain
```

- `core/internal/domain` must not import CLI, TUI, REST, MCP, storage, RPC, or
  concrete module/service code. Keep it pure; construct value objects through
  their `New...` constructors so validation runs.
- Front ends call application services (`internal/app`); they do not reach into
  adapters directly.
- Match the surrounding code: error wrapping, and defensive copying of
  maps/slices at boundaries.

## Safety-sensitive code

Hovel is an authorized red-team emulation and defensive validation tool for
scoped, auditable assessments. Changes to the throw planning, confirmation,
guardrail, or audit-event path need extra care. Preserve:

- A throw cannot start without a persisted plan and a recorded confirmation.
- `--now` skips the typed prompt but still records an auditable confirmation
  noting the bypass.
- Modules tagged `dangerous` require `--allow-dangerous` before they can throw.
- Never silently redact or drop operator-controlled configuration values.

See `SECURITY.md` and `CONTRIBUTING.md` for the fuller picture.
