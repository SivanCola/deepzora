import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { build, dist } from './build.mjs';
import { loadRelease, loadVerifiedRelease, sha256, validateRelease, versionAtLeast } from './release.mjs';

const snapshot = JSON.parse(await readFile(new URL('./src/release.json', import.meta.url), 'utf8'));
function fixture(version = '0.0.10') {
  const release = JSON.parse(JSON.stringify(snapshot).replaceAll('0.0.2', version));
  const bytes = JSON.stringify(release);
  const pointer = { schema: 1, version, sourceSHA: release.sourceSHA, inventory: { url: `https://downloads.deepzora.app/desktop/desktop-v${version}/mirror.json`, size: Buffer.byteLength(bytes), sha256: sha256(bytes) } };
  const calls = [];
  const fetcher = async (url, options) => {
    calls.push([url, options]);
    return new Response(url.includes('/website/latest.json?') ? JSON.stringify(pointer) : bytes);
  };
  return { release, pointer, bytes, fetcher, calls };
}

test('a verified newer release builds both languages and every download from the same inventory', async () => {
  const f = fixture();
  const release = await build({ env: { CF_PAGES: '1', CF_PAGES_BRANCH: 'main', CF_PAGES_COMMIT_SHA: 'a'.repeat(40) }, fetcher: f.fetcher });
  assert.equal(release.version, '0.0.10');
  assert.equal(f.calls.length, 2);
  for (const page of ['index.html', 'en/index.html']) {
    const html = await readFile(`${dist}/${page}`, 'utf8');
    assert.match(html, /v0\.0\.10/);
    assert.doesNotMatch(html, /desktop-v0\.0\.2/);
    for (const file of release.files.filter(file => /\.(dmg|exe|deb)$/.test(file.name))) assert(html.includes(file.url));
  }
  const metadata = JSON.parse(await readFile(`${dist}/release.json`, 'utf8'));
  assert.equal(metadata.version, release.version);
  assert.equal(metadata.commitSHA, 'a'.repeat(40));
  assert.deepEqual(metadata.files, release.files);
});

test('local and PR previews stay offline; production fails closed instead of falling back', async () => {
  const unavailable = async () => { throw new Error('network unavailable'); };
  assert.equal((await loadRelease({}, unavailable)).version, snapshot.version);
  assert.equal((await loadRelease({ CF_PAGES: '1', CF_PAGES_BRANCH: 'feature' }, unavailable)).version, snapshot.version);
  await assert.rejects(loadRelease({ CF_PAGES: '1', CF_PAGES_BRANCH: 'main' }, unavailable), /network unavailable/);
  await assert.rejects(loadRelease({ CF_PAGES: '1', CF_PAGES_BRANCH: 'main', WEBSITE_RELEASE_SOURCE: 'snapshot' }, unavailable), /Production must/);
  await assert.rejects(loadRelease({ WEBSITE_RELEASE_SOURCE: 'unknown' }, unavailable), /Unknown/);
});

test('production rejects missing, corrupted, redirected, mismatched and older inventories', async () => {
  await assert.rejects(loadVerifiedRelease({ fetcher: async () => new Response('', { status: 404 }) }), /HTTP 404/);
  for (const mutate of [
    p => { p.inventory.sha256 = 'a'.repeat(64); },
    p => { p.inventory.size += 1; },
    p => { p.sourceSHA = 'b'.repeat(40); },
    p => { p.inventory.url = 'https://example.com/mirror.json'; },
    p => { p.version = '0.0.10-rc.1'; },
  ]) {
    const f = fixture(); mutate(f.pointer);
    await assert.rejects(loadVerifiedRelease({ fetcher: f.fetcher }));
  }
  const older = fixture('0.0.1');
  await assert.rejects(loadVerifiedRelease({ fetcher: older.fetcher }), /downgrade/);
  const f = fixture();
  await loadVerifiedRelease({ fetcher: f.fetcher });
  assert(f.calls.every(([, options]) => options.redirect === 'error'));
  assert.equal(versionAtLeast('0.0.10', '0.0.2'), true);
  assert.equal(versionAtLeast('0.0.2', '0.0.10'), false);
});

test('installer identities must match the release version and canonical R2 and GitHub paths', () => {
  for (const mutate of [
    r => { r.files[0].url = 'https://example.com/installer'; },
    r => { r.files[0].key = 'desktop/desktop-v0.0.1/installer'; },
    r => { r.files[0].source = 'https://example.com/installer'; },
    r => { r.files[0].name = '../installer'; },
    r => { r.files[0].size = 0; },
    r => { r.files = r.files.slice(1); },
    r => { r.files.push(r.files[0]); },
  ]) {
    const copy = structuredClone(snapshot); mutate(copy);
    assert.throws(() => validateRelease(copy));
  }
});
