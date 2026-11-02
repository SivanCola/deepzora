import { createHash, randomUUID } from 'node:crypto';
import { readFile } from 'node:fs/promises';

export const publicBase = 'https://downloads.deepzora.app';
export const pointerURL = `${publicBase}/website/latest.json`;
const versionPattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
export const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');

export function versionAtLeast(candidate, floor) {
  if (!versionPattern.test(candidate) || !versionPattern.test(floor)) throw new Error('Invalid stable version');
  const a = candidate.split('.').map(BigInt), b = floor.split('.').map(BigInt);
  for (let i = 0; i < 3; i++) if (a[i] !== b[i]) return a[i] > b[i];
  return true;
}

export function validateRelease(release) {
  if (release?.schema !== 1 || release.repository !== 'SivanCola/deepzora' || release.publicBase !== publicBase || !versionPattern.test(release.version) || !/^[a-f0-9]{40}$/.test(release.sourceSHA)) throw new Error('Invalid release identity');
  if (!Array.isArray(release.files) || !release.files.length) throw new Error('Missing release assets');
  const names = new Set();
  for (const file of release.files) {
    if (!/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(file.name)) throw new Error('Invalid asset name');
    const kind = file.tag === `desktop-v${release.version}` ? 'desktop' : file.tag === `v${release.version}` ? 'cli' : null;
    const key = `${kind}/${file.tag}/${file.name}`;
    if (!kind || file.key !== key || file.url !== `${publicBase}/${key}` || file.source !== `https://github.com/SivanCola/deepzora/releases/download/${file.tag}/${file.name}`) throw new Error(`Invalid R2 URL: ${file.name}`);
    if (!Number.isSafeInteger(file.size) || file.size <= 0 || !/^[a-f0-9]{64}$/.test(file.sha256) || names.has(file.name)) throw new Error(`Invalid asset metadata: ${file.name}`);
    names.add(file.name);
  }
  for (const suffix of ['macOS-arm64.dmg', 'macOS-x64.dmg', 'macOS-universal.dmg', 'Windows-x64-Setup.exe', 'Windows-arm64-Setup.exe', 'Windows-x64-Portable.zip', 'Windows-arm64-Portable.zip', 'Linux-x64.deb', 'Linux-x64.tar.gz']) {
    const name = `Deepzora-${release.version}-${suffix}`;
    if (!release.files.some(file => file.name === name && file.tag === `desktop-v${release.version}`)) throw new Error(`Missing download: ${suffix}`);
  }
  return release;
}

export function validatePointer(pointer) {
  if (pointer?.schema !== 1 || !versionPattern.test(pointer.version) || !/^[a-f0-9]{40}$/.test(pointer.sourceSHA) || pointer.inventory?.url !== `${publicBase}/desktop/desktop-v${pointer.version}/mirror.json` || !Number.isSafeInteger(pointer.inventory.size) || pointer.inventory.size <= 0 || pointer.inventory.size > 1024 * 1024 || !/^[a-f0-9]{64}$/.test(pointer.inventory.sha256)) throw new Error('Invalid verified website pointer');
  return pointer;
}

async function getBytes(url, fetcher) {
  const response = await fetcher(url, { redirect: 'error', cache: 'no-store', signal: AbortSignal.timeout(30_000) });
  if (!response.ok) throw new Error(`Release metadata HTTP ${response.status}`);
  const bytes = Buffer.from(await response.arrayBuffer());
  if (bytes.length > 1024 * 1024) throw new Error('Oversized release metadata');
  return bytes;
}

export async function loadVerifiedRelease({ fetcher = fetch, floor = '0.0.2' } = {}) {
  // Unique query avoids a previously cached pointer; immutable inventories can
  // use the CDN normally. The pointer is published only after all readbacks.
  const pointer = validatePointer(JSON.parse(await getBytes(`${pointerURL}?build=${randomUUID()}`, fetcher)));
  if (!versionAtLeast(pointer.version, floor)) throw new Error('Verified website release would downgrade the snapshot');
  const bytes = await getBytes(pointer.inventory.url, fetcher);
  if (bytes.length !== pointer.inventory.size || sha256(bytes) !== pointer.inventory.sha256) throw new Error('Verified inventory digest mismatch');
  const release = validateRelease(JSON.parse(bytes));
  if (release.version !== pointer.version || release.sourceSHA !== pointer.sourceSHA) throw new Error('Verified inventory identity mismatch');
  return release;
}

export async function loadRelease(env = process.env, fetcher = fetch) {
  const snapshot = validateRelease(JSON.parse(await readFile(new URL('./src/release.json', import.meta.url), 'utf8')));
  const production = env.CF_PAGES === '1' && env.CF_PAGES_BRANCH === 'main';
  if (production && env.WEBSITE_RELEASE_SOURCE === 'snapshot') throw new Error('Production must use the verified R2 release');
  if (env.WEBSITE_RELEASE_SOURCE && !['snapshot', 'r2'].includes(env.WEBSITE_RELEASE_SOURCE)) throw new Error('Unknown website release source');
  return production || env.WEBSITE_RELEASE_SOURCE === 'r2' ? loadVerifiedRelease({ fetcher, floor: snapshot.version }) : snapshot;
}
