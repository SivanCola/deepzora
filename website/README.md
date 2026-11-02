# Deepzora website

Local-first Chinese / English product website. Static HTML, CSS and JavaScript; no runtime dependencies or credentials. Uses the current Deepzora brand assets and a clearly labeled interactive workflow illustration.

Presentation uses a pure white background, neutral gray typography and borders, and orange for primary actions and selection. The page keeps one workflow illustration and focuses on features, CLI installation, downloads, FAQ, and documentation.

## Preview

Requires Node.js 24 or later. From the repository root:

```sh
npm --prefix website run dev
```

- Chinese: http://127.0.0.1:4321/
- English: http://127.0.0.1:4321/en/

The server binds only to loopback. Source changes rebuild automatically; refresh the browser to view them. Stop with Ctrl-C. `npm --prefix website run preview` builds and serves without watching.

## Build and verify

```sh
npm --prefix website run check
npm --prefix website run build
```

Output: `website/dist/`. The build contains localized metadata, canonical / alternate links, sitemap, robots.txt, and a bilingual 404 page. No application server is needed.

## Production deployment

The production host is Cloudflare Pages project `deepzora-website`, with its native GitHub integration:

| Setting | Value |
| --- | --- |
| GitHub repository | `SivanCola/deepzora` |
| Production branch | `main` |
| Root directory | `website` |
| Framework preset | None |
| Build command | `npm run check` |
| Build output directory | `dist` |
| Environment variable | `NODE_VERSION=24.20.0` |
| Environment variable | `SKIP_DEPENDENCY_INSTALL=true` |
| Build watch include path | `website/*` |
| Custom domain | `deepzora.app` |

Cloudflare pulls the repository, builds the static pages, and publishes after changes reach `main`. The build runs the same checks used locally; a failed build does not replace the previous successful deployment. GitHub Actions `Website` independently validates the source and saves the static output as an artifact. Cloudflare's GitHub App handles deployment. A main-branch deploy hook stored as `CLOUDFLARE_PAGES_DEPLOY_HOOK` in Actions Secrets also allows the verified release mirror to request a rebuild, without repository write access or a Cloudflare administration token.

The download hostname `downloads.deepzora.app` continues to serve versioned release objects from R2. `www.deepzora.app` redirects to the apex while preserving paths and queries.

To roll back website code, select an earlier successful production deployment in Cloudflare Pages, then revert the source change in GitHub. A future production build still uses the latest verified release inventory; recovering an older mirror cannot downgrade downloads.

## Downloads

`src/release.json` is an offline preview and PR-validation snapshot for desktop v0.0.2, obtained from:

https://downloads.deepzora.app/desktop/desktop-v0.0.2/mirror.json

Download buttons point directly to versioned R2 objects on `downloads.deepzora.app`. macOS architectures, Windows installers and portable packages, and Linux x64 packages are explicit choices. Full inventory and GitHub release links provide alternatives. The build validates URLs, sizes, checksums, and required package names; it does not rehash large remote binaries.

Production builds (`CF_PAGES=1`, `CF_PAGES_BRANCH=main`) instead fetch `https://downloads.deepzora.app/website/latest.json`, then fetch its immutable `mirror.json`. They validate the inventory's size, SHA-256, source commit, stable version, canonical URLs, and complete website package matrix before rendering. Missing or invalid remote metadata fails the build; production never falls back to a stale snapshot. Both languages and public `/release.json` are generated from the same inventory.

After a new stable release passes public postflight, the mirror workflow verifies every source and public R2 object, advances the website pointer using a conditional write and version guard, and triggers the main-branch Pages deploy hook. It then waits up to 15 minutes for fresh production metadata and verifies the version and download URLs on both `/` and `/en/`. Hook or deployment failure fails the workflow; rerun the mirror for the same version to safely retry. An older-version backfill preserves the newer pointer and skips website deployment. No manual source update or PR is needed per release. See [the mirror runbook](../docs/R2_DOWNLOAD_MIRROR.md).

To check the current production release locally (network required):

```sh
WEBSITE_RELEASE_SOURCE=r2 npm --prefix website run check
node --test website/*.test.mjs scripts/release-website-deploy.test.mjs scripts/release-r2-mirror.test.mjs
```

Use the default offline build for local development and PR previews. A deliberate snapshot refresh is optional and must still pass validation. The deploy hook URL is a credential: never put it in source, logs, or artifacts.

## Content and assets

- `src/content.mjs`: Chinese / English copy.
- `src/page.mjs`: semantic HTML and release download rendering.
- `public/styles.css`, `public/app.js`: responsive presentation and interactions.
- Brand files copied from the repository's current desktop branding.
- Manrope is bundled locally under the SIL Open Font License; see `public/assets/Manrope-OFL.txt`. No Google Fonts requests at runtime.

No analytics, cookies, provider calls, or API-key collection. The browser detects the operating system locally to select a download tab; architecture remains a user choice. CLI commands are copied only on request.
