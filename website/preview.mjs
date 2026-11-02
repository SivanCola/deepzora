import { createServer } from 'node:http';
import { readFile, stat } from 'node:fs/promises';
import { watch } from 'node:fs';
import { spawn } from 'node:child_process';
import { extname, resolve, sep } from 'node:path';
import { build, root, dist } from './build.mjs';

await build();
const mime = { '.html': 'text/html; charset=utf-8', '.css': 'text/css; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2', '.txt': 'text/plain; charset=utf-8', '.xml': 'application/xml; charset=utf-8' };
const server = createServer(async (req, res) => {
  try {
    if (!['GET', 'HEAD'].includes(req.method)) { res.writeHead(405, { Allow: 'GET, HEAD' }); res.end(); return; }
    const pathname = decodeURIComponent(new URL(req.url, 'http://127.0.0.1').pathname);
    let file = resolve(dist, `.${pathname}`);
    if (file !== dist && !file.startsWith(dist + sep)) { res.writeHead(403); res.end(); return; }
    const info = await stat(file);
    if (info.isDirectory()) {
      if (!pathname.endsWith('/')) { res.writeHead(308, { Location: pathname + '/' }); res.end(); return; }
      file = resolve(file, 'index.html');
    }
    const body = await readFile(file);
    res.writeHead(200, { 'Content-Type': mime[extname(file)] || 'application/octet-stream', 'Content-Length': body.length, 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff' });
    res.end(req.method === 'HEAD' ? undefined : body);
  } catch { res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' }); res.end('Page not found'); }
});
server.listen(4321, '127.0.0.1', () => console.log('Local preview: http://127.0.0.1:4321/ · English: http://127.0.0.1:4321/en/'));
server.on('error', error => { console.error(error.message); process.exitCode = 1; });

if (process.argv.includes('--watch')) {
  let timer, running = false, pending = false;
  const rebuild = () => {
    if (running) { pending = true; return; }
    running = true;
    const child = spawn(process.execPath, [resolve(root, 'build.mjs')], { stdio: 'inherit' });
    child.on('exit', () => { running = false; if (pending) { pending = false; rebuild(); } });
  };
  for (const folder of ['src', 'public']) watch(resolve(root, folder), { recursive: true }, () => { clearTimeout(timer); timer = setTimeout(rebuild, 150); });
  console.log('Watching source files. Refresh the browser after changes.');
}
