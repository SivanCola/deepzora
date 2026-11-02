import { cp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';
import { content } from './src/content.mjs';
import { renderPage } from './src/page.mjs';

export const root = fileURLToPath(new URL('.', import.meta.url));
export const dist = resolve(root, 'dist');

export function validateRelease(release) {
  if (release.repository !== 'SivanCola/deepzora' || !/^\d+\.\d+\.\d+$/.test(release.version)) throw new Error('Invalid release identity');
  if (!Array.isArray(release.files) || !release.files.length) throw new Error('Missing release assets');
  const names = new Set();
  for (const file of release.files) {
    const url = new URL(file.url);
    if (url.origin !== 'https://downloads.deepzora.app' || file.url !== `${url.origin}/${file.key}` || url.search || url.hash || url.username || url.password) throw new Error(`Invalid R2 URL: ${file.name}`);
    if (!Number.isSafeInteger(file.size) || file.size <= 0 || !/^[a-f0-9]{64}$/.test(file.sha256) || names.has(file.name)) throw new Error(`Invalid asset metadata: ${file.name}`);
    names.add(file.name);
  }
  for (const suffix of ['macOS-arm64.dmg', 'macOS-x64.dmg', 'macOS-universal.dmg', 'Windows-x64-Setup.exe', 'Windows-arm64-Setup.exe', 'Windows-x64-Portable.zip', 'Windows-arm64-Portable.zip', 'Linux-x64.deb', 'Linux-x64.tar.gz']) {
    if (!names.has(`Deepzora-${release.version}-${suffix}`)) throw new Error(`Missing download: ${suffix}`);
  }
  return release;
}

export async function build() {
  const release = validateRelease(JSON.parse(await readFile(resolve(root, 'src/release.json'), 'utf8')));
  await rm(dist, { recursive: true, force: true });
  await mkdir(resolve(dist, 'en'), { recursive: true });
  await cp(resolve(root, 'public'), dist, { recursive: true });
  for (const t of Object.values(content)) await writeFile(resolve(dist, t.path === '/' ? 'index.html' : 'en/index.html'), renderPage(t, release));
  await writeFile(resolve(dist, 'robots.txt'), 'User-agent: *\nAllow: /\nSitemap: https://deepzora.app/sitemap.xml\n');
  await writeFile(resolve(dist, 'sitemap.xml'), '<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://deepzora.app/</loc></url><url><loc>https://deepzora.app/en/</loc></url></urlset>\n');
  console.log(`Built Chinese + English website · Deepzora v${release.version}`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await build();
