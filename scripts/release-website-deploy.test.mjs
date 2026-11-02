import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { deployWebsite } from './release-website-deploy.mjs';
import { sha256 } from '../website/release.mjs';

const hook = 'https://api.cloudflare.com/client/v4/pages/webhooks/deploy_hooks/00000000-0000-0000-0000-000000000001';
function fixture() {
  const release = JSON.parse(readFileSync(new URL('../website/src/release.json', import.meta.url)));
  const inventory = JSON.stringify(release);
  const pointer = JSON.stringify({ schema: 1, version: release.version, sourceSHA: release.sourceSHA, inventory: { url: `https://downloads.deepzora.app/desktop/desktop-v${release.version}/mirror.json`, size: Buffer.byteLength(inventory), sha256: sha256(inventory) } });
  const report = { schema: 1, version: release.version, sourceSHA: release.sourceSHA, publicBase: release.publicBase, verifiedAt: new Date().toISOString(), files: release.files, website: { version: release.version, deploy: true, pointerSHA256: sha256(pointer) } };
  let clock = Date.now();
  const deployed = { ...structuredClone(release), builtAt: new Date(clock + 1).toISOString(), commitSHA: 'a'.repeat(40) };
  const calls = [];
  const fetcher = async (url, options) => {
    calls.push([url, options]);
    if (url === hook) return Response.json({ success: true });
    if (url.includes('/website/latest.json?')) return new Response(pointer);
    if (url.endsWith('/mirror.json')) return new Response(inventory);
    if (url.includes('/release.json?')) return Response.json(deployed);
    return new Response(`v${release.version} ${release.files.map(file => file.url).join(' ')}`);
  };
  return { report, deployed, fetcher, calls, options: { fetcher, now: () => clock, wait: async ms => { clock += ms; }, timeout: 20_000 } };
}

test('verified mirror triggers main rebuild and waits for fresh metadata plus both localized pages', async () => {
  const f = fixture();
  const result = await deployWebsite(f.report, hook, f.options);
  assert.equal(result.version, '0.0.2');
  assert.equal(f.calls.filter(([url]) => url === hook).length, 1);
  assert.equal(f.calls.find(([url]) => url === hook)[1].method, 'POST');
  assert(f.calls.some(([url]) => url.startsWith('https://deepzora.app/en/?')));
});

test('missing report, changed pointer or unverified files never trigger deployment; older recovery skips', async () => {
  for (const mutate of [
    r => { delete r.website; },
    r => { r.website.pointerSHA256 = 'f'.repeat(64); },
    r => { r.files = r.files.slice(1); },
    r => { r.sourceSHA = 'b'.repeat(40); },
  ]) {
    const f = fixture(); mutate(f.report);
    await assert.rejects(deployWebsite(f.report, hook, f.options));
    assert(!f.calls.some(([url]) => url === hook));
  }
  const f = fixture(); f.report.website = { deploy: false, version: '0.0.10' };
  assert.equal((await deployWebsite(f.report, undefined, f.options)).skipped, true);
  assert.equal(f.calls.length, 0);
});

test('an old build, wrong downloads or a failed Pages deployment is not reported as success', async () => {
  for (const mutate of [
    f => { f.deployed.builtAt = '2020-01-01T00:00:00Z'; },
    f => { f.deployed.files[0].sha256 = 'b'.repeat(64); },
    f => { const fetcher = f.fetcher; f.options.fetcher = (url, options) => url.startsWith('https://deepzora.app/en/?') ? new Response('old page') : fetcher(url, options); },
  ]) {
    const f = fixture(); mutate(f);
    await assert.rejects(deployWebsite(f.report, hook, f.options), /timed out/);
  }
});

test('hook errors are fatal, retryable, and never expose the secret URL', async () => {
  for (const [fail, message] of [
    [async () => { throw new Error(`request failed ${hook}`); }, 'Cloudflare Pages deploy hook request failed; rerun the mirror to retry'],
    [async () => new Response('', { status: 503 }), 'Cloudflare Pages deploy hook HTTP 503'],
    [async () => Response.json({ success: false }), 'Cloudflare Pages did not accept the deployment'],
  ]) {
    const f = fixture();
    f.options.fetcher = (url, options) => url === hook ? fail() : f.fetcher(url, options);
    await assert.rejects(deployWebsite(f.report, hook, f.options), { message });
  }
  const f = fixture();
  await assert.rejects(deployWebsite(f.report, 'https://example.com/hook', f.options), /Invalid Cloudflare/);
});
