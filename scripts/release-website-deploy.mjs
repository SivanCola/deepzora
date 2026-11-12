#!/usr/bin/env node
import { readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';
import { loadVerifiedRelease, pointerURL, sha256, validateRelease, versionAtLeast } from '../website/release.mjs';

function validateReport(report) {
  if (report?.schema !== 1 || report.publicBase !== 'https://downloads.deepzora.app' || !/^[a-f0-9]{40}$/.test(report.sourceSHA) || !Number.isFinite(Date.parse(report.verifiedAt)) || !Array.isArray(report.files) || typeof report.website?.deploy !== 'boolean' || !versionAtLeast(report.website.version, report.version)) throw new Error('Missing successful R2 mirror report');
  if (report.website.deploy && (report.website.version !== report.version || !/^[a-f0-9]{64}$/.test(report.website.pointerSHA256))) throw new Error('Invalid verified website promotion');
  return report;
}

export async function deployWebsite(report, hook, { fetcher = fetch, wait = delay, now = Date.now, timeout = 15 * 60_000 } = {}) {
  validateReport(report);
  if (!report.website.deploy) {
    console.log(`Skipped website deployment: recovered ${report.version}, current ${report.website.version}`);
    return { skipped: true, version: report.website.version };
  }
  let url;
  try { url = new URL(hook); } catch { throw new Error('Configure CLOUDFLARE_PAGES_DEPLOY_HOOK'); }
  if (url.origin !== 'https://api.cloudflare.com' || !/^\/client\/v4\/pages\/webhooks\/deploy_hooks\/[a-f0-9-]+$/.test(url.pathname) || url.search || url.hash || url.username || url.password) throw new Error('Invalid Cloudflare Pages deploy hook');
  // Bind the successful report to the public pointer and immutable inventory.
  const pointerResponse = await fetcher(`${pointerURL}?deploy=${now()}`, { redirect: 'error', signal: AbortSignal.timeout(30_000), cache: 'no-store' });
  if (!pointerResponse.ok || sha256(Buffer.from(await pointerResponse.arrayBuffer())) !== report.website.pointerSHA256) throw new Error('Website pointer changed after mirror; rerun the mirror');
  const release = await loadVerifiedRelease({ fetcher, floor: report.version });
  if (release.version !== report.version || release.sourceSHA !== report.sourceSHA || release.files.some(file => !report.files.some(item => item.key === file.key && item.url === file.url && item.size === file.size && item.sha256 === file.sha256))) throw new Error('Website release does not match the verified mirror report');
  const started = now();
  let response;
  // Never log a network error from the secret-bearing URL or its response body.
  try { response = await fetcher(url.href, { method: 'POST', redirect: 'error', signal: AbortSignal.timeout(30_000) }); }
  catch { throw new Error('Cloudflare Pages deploy hook request failed; rerun the mirror to retry'); }
  if (!response.ok) throw new Error(`Cloudflare Pages deploy hook HTTP ${response.status}`);
  const accepted = await response.json().catch(() => null);
  if (accepted?.success !== true) throw new Error('Cloudflare Pages did not accept the deployment');
  console.log(`Cloudflare accepted website rebuild for verified release ${release.version}`);
  while (now() - started < timeout) {
    try {
      const live = await fetcher(`https://deepzora.app/release.json?verify=${now()}`, { cache: 'no-store', redirect: 'error', signal: AbortSignal.timeout(30_000) });
      if (!live.ok) throw new Error('Website deployment not ready');
      const deployed = validateRelease(await live.json());
      if (deployed.version !== release.version || deployed.sourceSHA !== release.sourceSHA || Date.parse(deployed.builtAt) < started || !Number.isFinite(Date.parse(deployed.builtAt))) throw new Error('Waiting for the new production build');
      if (JSON.stringify(deployed.files) !== JSON.stringify(release.files)) throw new Error('Deployed download inventory mismatch');
      for (const path of ['/', '/en/']) {
        const page = await fetcher(`https://deepzora.app${path}?verify=${now()}`, { cache: 'no-store', redirect: 'error', signal: AbortSignal.timeout(30_000) });
        if (!page.ok) throw new Error('Localized website not ready');
        const html = await page.text();
        if (!html.includes(`v${release.version}`)) throw new Error('Localized website version mismatch');
        if (/href=["'][^"']*-Windows-(?:x64|arm64)-Portable\.zip(?:["'?#])/.test(html)) throw new Error('Windows website must offer installers only');
        for (const file of release.files.filter(file => /(?:\.(?:dmg|exe|deb)|-Linux-x64\.tar\.gz)$/.test(file.name))) if (!html.includes(file.url)) throw new Error('Localized website download mismatch');
      }
      console.log(`Production verified: https://deepzora.app/ + /en/ · v${release.version} · ${deployed.commitSHA}`);
      return { version: release.version, sourceSHA: release.sourceSHA, commitSHA: deployed.commitSHA, builtAt: deployed.builtAt };
    } catch { /* A queued or failed build must never be reported as deployed. */ }
    await wait(10_000);
  }
  throw new Error('Website production verification timed out; inspect Cloudflare Pages, then rerun the mirror');
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    const result = await deployWebsite(JSON.parse(await readFile(process.env.R2_MIRROR_REPORT, 'utf8')), process.env.CLOUDFLARE_PAGES_DEPLOY_HOOK);
    if (process.env.GITHUB_STEP_SUMMARY) await writeFile(process.env.GITHUB_STEP_SUMMARY, `\n### Official website\n\n- ${result.skipped ? 'Preserved newer release' : 'Production verified'}: v${result.version}\n- https://deepzora.app/ · https://deepzora.app/en/\n`, { flag: 'a' });
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
