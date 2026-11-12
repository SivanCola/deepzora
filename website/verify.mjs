import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFile, stat } from 'node:fs/promises';
import { resolve } from 'node:path';
import { build, root, dist } from './build.mjs';

const release = await build();
const stylesheet = await readFile(resolve(dist, 'styles.css'));
const stylesheetURL = `/styles.css?v=${createHash('sha256').update(stylesheet).digest('hex').slice(0, 12)}`;
for (const [page, lang] of [['index.html', 'zh-CN'], ['en/index.html', 'en']]) {
  const html = await readFile(resolve(dist, page), 'utf8');
  assert(html.includes(`<html lang="${lang}">`));
  assert(html.includes(`rel="stylesheet" href="${stylesheetURL}"`), 'Stylesheet URL must identify the deployed CSS contents');
  assert.equal([...html.matchAll(/<h1>/g)].length, 1);
  assert(!/undefined|TODO|Lorem ipsum/.test(html));
  const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map(match => match[1]);
  assert.equal(ids.length, new Set(ids).size, 'Duplicate IDs');
  for (const match of html.matchAll(/(?:href|src)="([^"]+)"/g)) {
    const value = match[1];
    if (value.startsWith('#') && value.length > 1) assert(ids.includes(value.slice(1)), `Missing anchor ${value}`);
    if (value.startsWith('/')) {
      const pathname = new URL(value, 'https://deepzora.app').pathname;
      const path = resolve(dist, `.${pathname}`, pathname.endsWith('/') ? 'index.html' : '');
      assert((await stat(path)).isFile(), `Missing local asset ${value}`);
    }
    if (value.startsWith('https://downloads.deepzora.app/') && value !== 'https://downloads.deepzora.app/') assert(release.files.some(file => file.url === value), `Unknown R2 asset ${value}`);
    const repoFile = value.match(/^https:\/\/github.com\/SivanCola\/deepzora\/blob\/main\/(.+)$/);
    if (repoFile) assert((await stat(resolve(root, '..', repoFile[1]))).isFile(), `Broken documentation link ${value}`);
  }
  assert.equal([...html.matchAll(/class="button package-button"/g)].length, 7);
  assert(html.includes('aria-live="polite"'));
  console.log(`Verified ${page}: localized content, navigation, assets, docs, seven R2 installers + portable alternatives`);
}
