import assert from 'node:assert/strict';
import { readFile, stat } from 'node:fs/promises';
import { resolve } from 'node:path';
import { build, root, dist, validateRelease } from './build.mjs';

await build();
const release = validateRelease(JSON.parse(await readFile(resolve(root, 'src/release.json'), 'utf8')));
for (const [page, lang] of [['index.html', 'zh-CN'], ['en/index.html', 'en']]) {
  const html = await readFile(resolve(dist, page), 'utf8');
  assert(html.includes(`<html lang="${lang}">`));
  assert.equal([...html.matchAll(/<h1>/g)].length, 1);
  assert(!/undefined|TODO|Lorem ipsum/.test(html));
  const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map(match => match[1]);
  assert.equal(ids.length, new Set(ids).size, 'Duplicate IDs');
  for (const match of html.matchAll(/(?:href|src)="([^"]+)"/g)) {
    const value = match[1];
    if (value.startsWith('#') && value.length > 1) assert(ids.includes(value.slice(1)), `Missing anchor ${value}`);
    if (value.startsWith('/')) {
      const path = resolve(dist, `.${value}`, value.endsWith('/') ? 'index.html' : '');
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
