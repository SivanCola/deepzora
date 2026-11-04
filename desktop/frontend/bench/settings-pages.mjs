#!/usr/bin/env node
// Exercise the real settings tree, including editors that the layout matrix
// cannot reach. The browser bridge supplies fixtures; no native settings change.
import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import { startPreviewServer } from "./vite-preview-server.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const port = Number(process.env.DEEPZORA_SETTINGS_PAGES_PORT ?? 4681);
const output = process.env.DEEPZORA_SETTINGS_ARTIFACTS ?? path.join(os.tmpdir(), "deepzora-settings-pages");
await mkdir(output, { recursive: true });
const externalUrl = process.env.DEEPZORA_SETTINGS_BASE_URL;
const preview = externalUrl ? null : await startPreviewServer(root, port);
const baseUrl = externalUrl ?? `http://127.0.0.1:${port}`;
const browser = await chromium.launch({ headless: true });
const errors = [];
const findings = [];
let cases = 0;
const pages = [
  ["general", ".settings-field"], ["models", ".model-assignment-row"], ["providers", ".provider-connections"],
  ["model-stats", ".usage-stats__cards"], ["mcp", ".cap-mcp-add-btn:enabled"], ["bot-channels", ".bot-provider-card:enabled"], ["agent-presets", ".agent-preset-row"],
  ["team", ".settings-section__actions button:enabled"],
  ["skills", ".settings-toolbar"], ["subagents", ".subagents-profile-group"], ["plugins", ".settings-toolbar button:enabled"],
  ["memory", ".memory-tabs-row"], ["hooks", ".hooks-json-panel__textarea"], ["diagnostics", ".diag-page"],
  ["permissions", ".set-rules"], ["sandbox", ".sandbox-settings"], ["network", ".settings-save-bar"],
  ["browser", "[role=status]"], ["appearance", ".appearance-overview"], ["shortcuts", ".settings-section"],
  ["storage", ".settings-path-value"], ["updates", ".settings-field"],
];

async function settle(page) {
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
}

async function verify(page, name, capture = false, scope = ".settings-center__content") {
  if (scope === ".settings-center__content") {
    assert.equal(await page.locator(".settings-page > .settings-page__header h1").count(), 1, `${name}: one primary page title`);
    assert.equal(await page.locator(".settings-page > .settings-page__header h1").isVisible(), true, `${name}: page title stays visible`);
    for (const header of await page.locator(".settings-subpage-header:visible").all()) {
      assert.equal(await header.locator("h2").count(), 1, `${name}: shared subpage title`);
      assert.ok((await header.locator(".settings-back-button").textContent()).trim(), `${name}: named return to parent`);
      assert.ok(await header.getAttribute("data-save-mode"), `${name}: declared save policy`);
    }
  }
  for (const width of [1600, 900, 400]) {
    await page.setViewportSize({ width, height: 1000 });
    await settle(page);
    const overflow = await page.locator(scope).evaluate(main => {
      const page = main.querySelector(".settings-page") ?? main;
      const bounds = main.getBoundingClientRect();
      const offenders = [...page.querySelectorAll("input:not([type=hidden]), textarea, button, select")].filter(el => {
        const r = el.getBoundingClientRect();
        if (!el.getClientRects().length || r.width < 1 || r.height < 1 || getComputedStyle(el).visibility === "hidden") return false;
        // Closed native disclosures and inactive model pages stay mounted.
        if (el.closest("[hidden]")) return false;
        return r.left < bounds.left - 1 || r.right > bounds.right + 1;
      });
      const pageBounds = page.getBoundingClientRect();
      const outlying = [...page.querySelectorAll("*")].filter(el => {
        const r = el.getBoundingClientRect();
        return el.getClientRects().length && r.width > 0 && !el.closest("[hidden]") && r.right > pageBounds.right + 1;
      }).slice(0, 10).map(el => el.getAttribute("class") || el.tagName);
      const surfaceSelector = ".provider-access-card--detail, .provider-model-draft__list, .provider-add-panel, .provider-catalog__detail, .provider-model-dialog, .theme-gallery__editor, .cap-skill-card, .cap-mcp-editor, .prov-card--edit, .cap-plugin-installer, .mem-fact, .mem-doc, .appearance-overview__hero, .typography-settings__detail, .theme-gallery__detail, .diag-section__body, .set-rules";
      const surfaces = [...(page.matches(surfaceSelector) ? [page] : []), ...page.querySelectorAll(surfaceSelector)];
      const gutters = surfaces.flatMap(surface => {
        if (!surface.getClientRects().length || surface.closest("[hidden]")) return [];
        const style = getComputedStyle(surface);
        // Flat sections intentionally align with the page grid. Only enclosed
        // or painted surfaces need their own inset, including nested cards.
        if (style.backgroundColor === "rgba(0, 0, 0, 0)" && parseFloat(style.borderLeftWidth) === 0 && parseFloat(style.borderRightWidth) === 0 && !surface.matches(".typography-settings__detail, .diag-section__body")) return [];
        const rect = surface.getBoundingClientRect();
        const walker = document.createTreeWalker(surface, NodeFilter.SHOW_TEXT);
        let node; const edges = [];
        while ((node = walker.nextNode())) {
          const text = node.textContent.trim();
          const parent = node.parentElement;
          if (!text || text.length > 140 || !parent?.getClientRects().length || parent.closest("[hidden], svg, pre, code, option")) continue;
          const range = document.createRange(); range.selectNodeContents(node);
          const r = range.getBoundingClientRect();
          if (!r.width || !r.height || r.top < rect.top || r.bottom > rect.bottom) continue;
          // A truncated label's raw text range can extend beyond its clip.
          if (r.left < rect.left - 1 || r.right > rect.right + 1) continue;
          if (r.left - rect.left < 8 || rect.right - r.right < 8) edges.push(text.slice(0, 60));
        }
        return edges.length ? [{ surface: surface.className, text: edges.slice(0, 3) }] : [];
      });
      return { page: page.scrollWidth - page.clientWidth, content: main.scrollWidth - main.clientWidth, outlying, gutters,
        controls: offenders.map(el => ({ tag: el.tagName, label: el.getAttribute("aria-label") || el.textContent?.trim().slice(0, 80) || el.className })) };
    });
    if (overflow.page > 1 || overflow.content > 1 || overflow.controls.length || overflow.gutters.length) findings.push({ name, width, ...overflow });
    if (width === 400 && scope === ".settings-center__content") {
      const navigationVisible = await page.locator(".settings-center__compact-navigation button").evaluate(el => {
        const r = el.getBoundingClientRect();
        const main = document.querySelector(".settings-center__content").getBoundingClientRect();
        return r.height >= 30 && r.top >= 0 && r.bottom <= main.top + 1 && el.contains(document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2));
      });
      assert.equal(navigationVisible, true, `${name}: compact navigation remains visible above the page`);
    }
    if (capture && (width === 1600 || width === 400)) await page.screenshot({ path: path.join(output, `${name}-${width}.png`) });
    cases++;
  }
  await page.setViewportSize({ width: 1600, height: 1000 });
  await writeFile(path.join(output, "results.json"), JSON.stringify({ cases, findings, errors }, null, 2));
  console.log(`CHECK ${name}`);
}

async function openSettings(page, locale) {
  await page.goto(`${baseUrl}/?mock=deepseek_upgrade&bench=1&platform=windows`, { waitUntil: "domcontentloaded" });
  await page.locator("textarea.composer__input:not([aria-hidden=true])").waitFor();
  await page.locator("button:has(svg.lucide-settings)").last().click();
  await page.locator(".settings-page--general").waitFor();
  const nav = page.locator(".settings-center__navgroups");
  const labels = await nav.locator(".settings-center__navitem").allTextContents();
  assert.equal(labels.length, pages.length, "every settings entry is covered");
  for (const [index, label] of labels.entries()) {
    await nav.getByRole("button", { name: label.trim(), exact: true }).click();
    const [id, ready] = pages[index];
    await page.locator(`.settings-page--${id} ${ready}`).first().waitFor();
    await page.evaluate(mode => { document.documentElement.dataset.theme = mode; }, locale === "zh" ? "light" : "dark");
    await settle(page);
    await verify(page, `${locale}-${String(index + 1).padStart(2, "0")}`, locale === "zh");
  }
  return nav;
}

try {
  for (const locale of ["en", "zh"]) {
    const page = await browser.newPage({ locale: locale === "zh" ? "zh-CN" : "en-US", viewport: { width: 1600, height: 1000 } });
    await page.emulateMedia({ colorScheme: locale === "zh" ? "light" : "dark" });
    page.on("pageerror", error => errors.push(error.message));
    const nav = await openSettings(page, locale);
    if (locale === "en") {
      const go = name => nav.getByRole("button", { name, exact: true }).click();
      await go("General");
      await page.locator(".settings-sound-editor__toggle").click();
      await verify(page, "sound-editor", true);
      await page.locator(".status-bar-items-editor__toggle").click();
      await page.locator(".status-bar-items-editor__workspace").waitFor();
      await verify(page, "status-bar-editor", true);
      await go("Model preferences");
      await page.locator(".settings-subagent-advanced > summary").click();
      await verify(page, "model-advanced", true);
      await go("Network");
      await page.getByRole("radio", { name: "custom", exact: true }).click();
      await verify(page, "network-custom", true);
      await page.getByRole("textbox", { name: "Server", exact: true }).fill("127.0.0.2");
      assert.equal(await page.getByRole("button", { name: "Save network settings", exact: true }).isEnabled(), true);
      await page.locator(".settings-save-bar").getByRole("button", { name: "Cancel", exact: true }).click();
      assert.equal(await page.getByRole("button", { name: "Save network settings", exact: true }).isEnabled(), false);

      await go("Agent presets");
      await page.locator(".agent-presets .settings-section__actions button").last().click();
      await page.locator(".agent-preset-editor").waitFor();
      await verify(page, "preset-editor", true);
      await page.locator(".agent-preset-editor .settings-save-bar").getByRole("button", { name: "Cancel", exact: true }).click();
      await page.locator(".agent-preset-row").first().getByRole("button", { name: "Copy", exact: true }).click();
      await verify(page, "preset-copy", true);
      await page.locator(".agent-preset-editor").getByRole("button", { name: "Save", exact: true }).click();
      await page.locator(".agent-preset-row").filter({ has: page.getByRole("button", { name: "Edit", exact: true }) }).first().getByRole("button", { name: "Edit", exact: true }).click();
      await verify(page, "preset-edit", true);
      assert.equal(await page.locator(".agent-preset-editor").getByRole("button", { name: "Save", exact: true }).isEnabled(), false);
      await page.locator(".agent-preset-editor .settings-back-button").click();

      await go("Team");
      for(const name of ["Chief","Ada With Spaces"]) {
        await page.getByRole("button",{name:"New member",exact:true}).click();
        await page.getByRole("textbox",{name:"Name",exact:true}).waitFor();
        await verify(page,`team-member-${name.split(" ")[0]}`,true);
        await page.getByRole("textbox",{name:"Name",exact:true}).fill(name);
        await page.locator(".settings-page--team button[type=submit]").click();
        await page.getByRole("heading",{name,exact:true}).waitFor();
      }
      await page.getByRole("button",{name:"New group",exact:true}).click();
      await verify(page,"team-group-editor",true);
      await page.getByRole("textbox",{name:"Name",exact:true}).fill("Project group");
      await page.getByRole("checkbox",{name:"Chief",exact:true}).check();
      await page.getByRole("checkbox",{name:"Ada With Spaces",exact:true}).check();
      await page.locator(".settings-page--team button[type=submit]").click();
      await page.getByRole("heading",{name:"Project group",exact:true}).waitFor();
      await verify(page,"team-saved-list",true);

      await go("MCP & Tools");
      await page.locator(".cap-mcp-add-btn").click();
      await page.locator(".cap-mcp-editor").waitFor();
      await verify(page, "mcp-add", true);
      await page.locator(".cap-mcp-subpage__back").click();
      await page.getByRole("button", { name: "Browse registry", exact: true }).click();
      await page.locator(".cap-mcp-search--action").waitFor();
      await verify(page, "mcp-registry", true);
      await page.locator(".cap-mcp-subpage__back").click();
      await page.locator("button.cap-mcp-list-row__main").first().click();
      await page.locator(".cap-mcp-subpage").waitFor();
      await verify(page, "mcp-detail", true);
      await page.locator(".cap-mcp-subpage").getByRole("button", { name: "Edit config", exact: true }).click();
      await page.locator(".cap-mcp-editor").waitFor();
      await verify(page, "mcp-edit", true);

      await go("Subagents");
      await page.locator(".subagents-toolbar button").first().click();
      await page.locator(".subagents-form-header").waitFor();
      await verify(page, "subagent-editor", true);
      await page.locator(".subagents-form-header .settings-back-button").click();
      await page.locator(".subagents-row-actions").getByRole("button", { name: "Edit", exact: true }).first().click();
      await verify(page, "subagent-edit", true);

      await go("Plugins");
      await page.getByRole("button", { name: "Install plugin", exact: true }).click();
      await page.locator(".cap-plugin-installer").waitFor();
      await verify(page, "plugin-local", true);
      await page.getByRole("button", { name: "Git repository", exact: true }).click();
      await verify(page, "plugin-git", true);

      await go("Memory");
      await page.locator(".memory-tabs-row").waitFor();
      for (const [index, tab] of (await page.locator(".memory-tabs-row [role=tab]").all()).entries()) {
        await tab.click();
        await verify(page, `memory-${index}`, true);
      }

      await go("Model services");
      await page.locator(".provider-connections__item").first().click();
      await page.locator(".provider-editor__body").first().waitFor();
      await verify(page, "provider-editor", true);
      await page.locator(".provider-model-draft__option button[aria-haspopup=dialog]").first().click();
      await page.locator(".provider-model-dialog").waitFor();
      await verify(page, "model-dialog", true, ".provider-model-dialog");
      await page.locator(".provider-model-dialog").press("Escape");
      await page.locator(".provider-editor-advanced > summary").click();
      await verify(page, "provider-advanced", true);
      await page.locator(".provider-connections__nav > .btn").click();
      await page.locator(".provider-add-panel").waitFor();
      await verify(page, "provider-catalog", true);
      await page.locator(".provider-add-panel [role=tab]").last().click();
      await page.locator(".provider-editor--wizard").waitFor();
      await verify(page, "provider-custom", true);
      await page.locator(".provider-add-panel__head .settings-back-button").click();

      await go("Appearance");
      await page.locator(".appearance-overview__row--typography button").click();
      await page.locator(".typography-settings").waitFor();
      await verify(page, "typography", true);
      for (const region of await page.locator(".typography-settings__region").all()) {
        if ((await region.textContent()).includes("Global")) continue;
        await region.click();
        await verify(page, "typography-region");
      }
      await page.locator(".typography-settings__back").click();
      await page.locator(".appearance-overview__hero-actions .btn--primary").click();
      await page.locator(".theme-gallery").waitFor();
      await verify(page, "theme-gallery", true);
      await page.locator(".theme-gallery__open-preview").click();
      await verify(page, "theme-immersive", true);
      await page.locator(".theme-gallery--immersive .settings-back-button").click();
      await page.locator(".theme-gallery__top-actions button").first().click();
      await page.locator(".theme-editor").waitFor();
      await verify(page, "theme-editor", true, ".theme-gallery__editor");
      await page.locator(".theme-gallery__editor").press("Escape");

      const confirmation = page.getByRole("dialog", { name: "Unsaved changes", exact: true });
      const stay = () => confirmation.getByRole("button", { name: "Keep editing", exact: true }).click();
      const discard = () => confirmation.getByRole("button", { name: "Discard and leave", exact: true }).click();
      await go("Network");
      await page.getByRole("radio", { name: "custom", exact: true }).click();
      await page.getByRole("textbox", { name: "Server", exact: true }).fill("127.0.0.2");
      await go("General");
      await confirmation.waitFor(); await stay();
      assert.equal(await page.getByRole("textbox", { name: "Server", exact: true }).inputValue(), "127.0.0.2");
      await go("General"); await discard();
      await page.locator(".settings-page--general").waitFor();
      await go("Network");
      await page.getByRole("radio", { name: "custom", exact: true }).click();
      await page.getByRole("textbox", { name: "Server", exact: true }).fill("127.0.0.3");
      await go("General");
      await confirmation.getByRole("button", { name: "Save and continue", exact: true }).click();
      await page.locator(".settings-page--general").waitFor();
      await go("Network");
      assert.equal(await page.getByRole("textbox", { name: "Server", exact: true }).inputValue(), "127.0.0.3");
      await go("Hooks");
      await page.locator(".hooks-json-panel__textarea").fill("invalid json");
      await go("General");
      await confirmation.getByRole("button", { name: "Save and continue", exact: true }).click();
      await confirmation.getByRole("alert").waitFor();
      assert.equal(await page.locator(".hooks-json-panel__textarea").inputValue(), "invalid json");
      await page.screenshot({ path: path.join(output, "unsaved-save-failure.png") });
      await stay(); await go("General"); await discard();
      await go("Agent presets");
      await page.locator(".agent-presets .settings-section__actions button").last().click();
      await page.locator(".agent-preset-editor input").first().fill("Unsaved preset");
      await page.locator('.agent-preset-editor input[id$="-id"]').fill("INVALID ID");
      await page.locator(".agent-preset-editor .settings-back-button").click();
      await confirmation.waitFor();
      assert.equal(await confirmation.getByRole("button", { name: "Save and continue", exact: true }).isEnabled(), false);
      await stay(); await go("General"); await discard();
      await go("MCP & Tools");
      await page.locator(".cap-mcp-add-btn").click();
      await page.locator(".cap-mcp-quick__input").fill("npx -y example-mcp");
      await page.locator(".cap-mcp-subpage__back").click(); await confirmation.waitFor(); await discard();
      await go("Model services");
      await page.locator(".provider-connections__item").first().click();
      await page.locator(".provider-url-input").first().fill("https://api.example.test/v1/chat/completions");
      await go("General"); await confirmation.waitFor(); await stay();
      await go("General"); await discard();
      await go("Plugins");
      await page.getByRole("button", { name: "Install plugin", exact: true }).click();
      await page.getByRole("button", { name: "Git repository", exact: true }).click();
      await page.locator("#settings-plugin-install input.mem-input").first().fill("/tmp/draft-plugin");
      await go("General"); await confirmation.waitFor();
      assert.equal(await confirmation.getByRole("button", { name: "Save and continue", exact: true }).isEnabled(), false);
      await discard();
      await go("Memory");
      await page.locator(".memory-tabs-row [role=tab]").nth(2).click();
      await page.locator(".mem-section__actions button").click();
      await verify(page, "memory-add", true);
      await page.locator(".mem-add input").fill("Unsaved memory instruction");
      await go("General"); await confirmation.waitFor(); await discard();
      await go("Memory");
      await page.locator(".memory-tabs-row [role=tab]").nth(2).click();
      await page.locator(".mem-doc__head-actions button").first().click();
      const instruction = page.locator(".mem-doc__edit textarea");
      const originalInstruction = await instruction.inputValue();
      await instruction.fill(`${originalInstruction}\nDraft for navigation check`);
      await verify(page, "memory-doc-editor", true);
      await page.locator(".memory-tabs-row [role=tab]").first().click();
      await confirmation.waitFor(); await stay();
      assert.match(await instruction.inputValue(), /Draft for navigation check/);
      await go("General"); await discard();
      await go("Memory");
      await page.locator(".memory-tabs-row [role=tab]").nth(2).click();
      await page.locator(".mem-doc__head-actions button").first().click();
      assert.equal(await instruction.inputValue(), originalInstruction, "discard never persists the instruction draft");
      await go("Appearance");
      await page.locator(".appearance-overview__hero-actions .btn--primary").click();
      await page.locator(".theme-gallery__top-actions button").first().click();
      const themeName = page.locator(".theme-editor__fields input").nth(1);
      await themeName.fill("Unsaved theme name");
      await page.locator(".theme-gallery__editor").press("Escape");
      await confirmation.waitFor(); await stay();
      assert.equal(await themeName.inputValue(), "Unsaved theme name");
      await page.locator(".theme-gallery__editor").press("Escape"); await discard();
      assert.equal(await page.locator(".theme-editor").count(), 0);
      await page.locator(".theme-gallery__back").click();
      await go("General");
      console.log("PASS navigation preserves drafts, discards explicitly, saves before leaving, retains failed/invalid forms and never installs on leave");

      // The compact selector provides every page plus search and keyboard exit.
      await page.setViewportSize({ width: 400, height: 800 });
      const selector = page.locator(".settings-center__compact-navigation button");
      await selector.click();
      const list = page.getByRole("listbox", { name: "Settings", exact: true });
      assert.equal(await list.getByRole("option").count(), pages.length);
      await page.getByRole("combobox", { name: "Search settings", exact: true }).fill("network");
      await list.getByRole("option", { name: "Network", exact: true }).click();
      await page.locator(".settings-page--network").waitFor();
      await selector.click();
      await page.getByRole("combobox", { name: "Search settings", exact: true }).press("Escape");
      assert.equal(await page.getByRole("listbox", { name: "Settings", exact: true }).count(), 0);
    }
    await page.close();
  }
  await writeFile(path.join(output, "results.json"), JSON.stringify({ cases, findings, errors }, null, 2));
  assert.deepEqual(errors, [], "all settings pages render without runtime exceptions");
  assert.deepEqual(findings, [], "all settings pages and editors fit their content pane");
  console.log(`PASS ${cases} settings page/editor viewport cases, both languages, shared titles/back/save policy, compact navigation and draft lifecycle interactions. Screenshots: ${output}`);
} finally {
  await browser.close();
  if (preview) await new Promise((resolve, reject) => preview.httpServer.close(error => error ? reject(error) : resolve()));
}
