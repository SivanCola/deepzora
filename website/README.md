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

The intended production host is Cloudflare Pages with its native GitHub integration:

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
| Build watch include path | `website/**` |
| Custom domain | `deepzora.app` |

Cloudflare pulls the repository, builds the static pages, and publishes after changes reach `main`. The build runs the same checks used locally; a failed build does not replace the previous successful deployment. GitHub Actions `Website` independently validates the source and saves the static output as an artifact. Cloudflare's GitHub App handles deployment without adding Cloudflare API credentials to GitHub Actions.

The download hostname `downloads.deepzora.app` is managed separately and continues to serve release objects from R2. Website publishing must not change that hostname or the release mirror workflow.

To roll back, select an earlier successful production deployment in Cloudflare Pages, then revert the source change in GitHub so subsequent builds preserve the rollback.

## Downloads

`src/release.json` is the public R2 mirror inventory for desktop v0.0.2, obtained from:

https://downloads.deepzora.app/desktop/desktop-v0.0.2/mirror.json

Download buttons point directly to versioned R2 objects on `downloads.deepzora.app`. macOS architectures, Windows installers and portable packages, and Linux x64 packages are explicit choices. Full inventory and GitHub release links provide alternatives. The build validates URLs, sizes, checksums, and required package names; it does not rehash large remote binaries.

For a future release, replace `src/release.json` with that release's published mirror inventory, run `check`, and review the download links before deployment. The website intentionally uses a build-time release snapshot; existing GitHub → R2 automation remains unchanged.

## Content and assets

- `src/content.mjs`: Chinese / English copy.
- `src/page.mjs`: semantic HTML and release download rendering.
- `public/styles.css`, `public/app.js`: responsive presentation and interactions.
- Brand files copied from the repository's current desktop branding.
- Manrope is bundled locally under the SIL Open Font License; see `public/assets/Manrope-OFL.txt`. No Google Fonts requests at runtime.

No analytics, cookies, provider calls, or API-key collection. The browser detects the operating system locally to select a download tab; architecture remains a user choice. CLI commands are copied only on request.
