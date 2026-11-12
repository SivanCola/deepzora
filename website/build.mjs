import { cp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';
import { content } from './src/content.mjs';
import { renderPage } from './src/page.mjs';
import { loadRelease } from './release.mjs';
export { validateRelease } from './release.mjs';

export const root = fileURLToPath(new URL('.', import.meta.url));
export const dist = resolve(root, 'dist');

export async function build({ env = process.env, fetcher = fetch } = {}) {
  const release = await loadRelease(env, fetcher);
  await rm(dist, { recursive: true, force: true });
  await mkdir(resolve(dist, 'en'), { recursive: true });
  await cp(resolve(root, 'public'), dist, { recursive: true });
  const stylesheet = await readFile(resolve(dist, 'styles.css'));
  const stylesheetURL = `/styles.css?v=${createHash('sha256').update(stylesheet).digest('hex').slice(0, 12)}`;
  for (const t of Object.values(content)) await writeFile(resolve(dist, t.path === '/' ? 'index.html' : 'en/index.html'), renderPage(t, release, stylesheetURL));
  await writeFile(resolve(dist, 'robots.txt'), 'User-agent: *\nAllow: /\nSitemap: https://deepzora.app/sitemap.xml\n');
  await writeFile(resolve(dist, 'sitemap.xml'), '<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://deepzora.app/</loc></url><url><loc>https://deepzora.app/en/</loc></url></urlset>\n');
  await writeFile(resolve(dist, 'release.json'), `${JSON.stringify({ ...release, builtAt: new Date().toISOString(), commitSHA: env.CF_PAGES_COMMIT_SHA || null }, null, 2)}\n`);
  console.log(`Built Chinese + English website · Deepzora v${release.version}`);
  return release;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await build();
