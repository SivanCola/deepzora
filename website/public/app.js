(() => {
  const t = JSON.parse(document.querySelector('#page-content').textContent);
  const esc = value => String(value).replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
  const code = {
    before: [
      ['// src/client.ts', 'comment'], ['', ''],
      ['export async function request(', ''], ['  url: string,', ''], ['  options: RequestInit = {},', ''], [') {', ''],
      ['  const response = await fetch(', ''], ['    url, options', ''], ['  );', ''], ['', ''],
      ['  if (!response.ok) {', ''], ['    throw new Error(', ''], ['      `Request failed: ${response.status}`', ''], ['    );', ''], ['  }', ''], ['', ''],
      ['  return response.json();', ''], ['}', ''],
    ],
    diff: [
      ['// src/client.ts · illustrative diff', 'comment'], ['', ''], ['export async function request(', ''], ['  url: string,', ''], ['  options: RequestInit = {},', ''], [') {', ''],
      ['- const response = await fetch(url, options);', 'removed'], ['+ const response = await withRetry(', 'added'], ['+   () => fetch(url, options),', 'added'], ['+   { attempts: 3 },', 'added'], ['+ );', 'added'], ['', ''],
      ['  if (!response.ok) {', ''], ['    throw new Error(', ''], ['      `Request failed: ${response.status}`', ''], ['    );', ''], ['  }', ''], ['  return response.json();', ''], ['}', ''],
    ],
    tests: [
      ['// client.test.ts · test example', 'comment'], ['', ''], ["describe('request', () => {", ''], ["  it('retries a transient failure',", ''], ['    async () => {', ''],
      ['      const send = vi.fn()', ''], ['        .mockRejectedValueOnce(', ''], ["          new TypeError('Network error')", ''], ['        )', ''], ['        .mockResolvedValueOnce(', ''], ['          new Response(body)', ''], ['        );', ''], ['', ''],
      ['      await request(endpoint);', ''], ['      expect(send)', ''], ['        .toHaveBeenCalledTimes(2);', ''], ['    });', ''], ['});', ''],
    ],
  };
  const renderCode = key => {
    document.querySelector('[data-demo-code]').innerHTML = code[key].map(([line, kind]) => `<span class="code-line${kind === 'added' || kind === 'removed' ? ` ${kind}` : ''}"><span${kind === 'comment' ? ' class="code-comment"' : ''}>${esc(line) || ' '}</span></span>`).join('');
  };
  const switchDemo = button => {
    const index = Number(button.dataset.step), step = t.steps[index];
    document.querySelectorAll('[data-step]').forEach(tab => {
      const active = tab === button;
      tab.classList.toggle('active', active);
      tab.setAttribute('aria-selected', String(active));
      tab.tabIndex = active ? 0 : -1;
    });
    for (const [selector, value] of Object.entries({ user: step.user, title: step.title, body: step.body, tool: step.tool, status: step.status, file: step.file })) document.querySelector(`[data-demo-${selector}]`).textContent = value;
    document.querySelector('[data-demo-files]').innerHTML = step.files.map(file => `<span>↳ ${esc(file)}</span>`).join('');
    document.querySelector('#demo-panel').setAttribute('aria-labelledby', button.id);
    renderCode(step.code);
  };
  const switchOS = button => {
    document.querySelectorAll('[data-os]').forEach(tab => {
      const active = tab === button;
      tab.classList.toggle('active', active);
      tab.setAttribute('aria-selected', String(active));
      tab.tabIndex = active ? 0 : -1;
      document.querySelector(`#packages-${tab.dataset.os}`).hidden = !active;
    });
  };
  function activateTabs(selector, activate) {
    const tabs = [...document.querySelectorAll(selector)];
    for (const tab of tabs) {
      tab.addEventListener('click', () => activate(tab));
      tab.addEventListener('keydown', event => {
        if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
        event.preventDefault();
        const index = tabs.indexOf(tab);
        const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length;
        tabs[next].focus();
        activate(tabs[next]);
      });
    }
  }
  renderCode('before');
  activateTabs('[data-step]', switchDemo);
  activateTabs('[data-os]', switchOS);
  // Make the likely platform convenient, while keeping every architecture an
  // explicit user choice. No analytics, remote inference, or provider calls.
  const platform = navigator.userAgentData?.platform || navigator.platform || '';
  const os = /Win/i.test(platform) ? 'windows' : /Linux/i.test(platform) && !/Android/i.test(navigator.userAgent) ? 'linux' : 'macos';
  switchOS(document.querySelector(`[data-os="${os}"]`));

  const copy = document.querySelector('[data-copy]');
  copy.addEventListener('click', async () => {
    const status = document.querySelector('#copy-status');
    try {
      await navigator.clipboard.writeText(document.querySelector('#copy-command-source').content.textContent);
      copy.querySelector('span').textContent = t.copied;
      copy.setAttribute('aria-label', t.copied);
      status.textContent = t.copied;
      window.setTimeout(() => { copy.querySelector('span').textContent = t.copy; copy.setAttribute('aria-label', t.copy); }, 2200);
    } catch { status.textContent = t.copyFailed; copy.querySelector('span').textContent = t.copyFailed; }
  });

  const menu = document.querySelector('.menu-toggle'), mobileNav = document.querySelector('#mobile-nav');
  const closeMenu = () => { menu.setAttribute('aria-expanded', 'false'); menu.setAttribute('aria-label', t.menu); mobileNav.hidden = true; };
  menu.addEventListener('click', () => {
    const open = menu.getAttribute('aria-expanded') !== 'true';
    menu.setAttribute('aria-expanded', String(open));
    menu.setAttribute('aria-label', open ? t.closeMenu : t.menu);
    mobileNav.hidden = !open;
  });
  mobileNav.querySelectorAll('a').forEach(link => link.addEventListener('click', closeMenu));
  document.addEventListener('keydown', event => { if (event.key === 'Escape' && !mobileNav.hidden) { closeMenu(); menu.focus(); } });
  window.matchMedia('(min-width: 621px)').addEventListener('change', event => { if (event.matches) closeMenu(); });
  document.querySelector('[data-language-switch]').addEventListener('click', event => { if (window.location.hash) event.currentTarget.hash = window.location.hash; });

  const header = document.querySelector('.site-header');
  const updateHeader = () => header.classList.toggle('scrolled', window.scrollY > 12);
  window.addEventListener('scroll', updateHeader, { passive: true }); updateHeader();
  if ('IntersectionObserver' in window) {
    const observer = new IntersectionObserver(entries => {
      for (const entry of entries) if (entry.isIntersecting) {
        document.querySelectorAll('.desktop-nav a').forEach(link => link.classList.toggle('active', link.hash === `#${entry.target.id}`));
      }
    }, { rootMargin: '-15% 0px -65% 0px' });
    ['features', 'workflow', 'download', 'docs'].forEach(id => observer.observe(document.getElementById(id)));
  }
})();
