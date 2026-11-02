import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";
import { newSessionButton } from "./app-page-actions.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
process.env.PLAYWRIGHT_BROWSERS_PATH ??= path.join(root, ".pw-browsers");
const { chromium } = await import("playwright");
const server = await createServer({ root, logLevel: "error", server: { host: "127.0.0.1", port: 0, hmr: false } });
await server.listen();
let browser;
try {
  browser = await chromium.launch({ headless: true, executablePath: process.env.CHROME_EXECUTABLE });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, locale: "en-US" });
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.goto(`http://127.0.0.1:${server.httpServer.address().port}/?mock=demo&bench=1`);
  await page.locator("#composer-input").waitFor();
  await newSessionButton(page).click();
  const picker = page.getByRole("button", { name: "Agent preset: Default", exact: true });
  await picker.focus();
  await picker.press("ArrowDown");
  await page.getByRole("menu", { name: "Agent preset: Default", exact: true }).waitFor();
  await page.waitForFunction(() => document.activeElement?.getAttribute("role") === "menuitemradio");
  await page.keyboard.press("End");
  assert.equal(await page.evaluate(() => document.activeElement.textContent), "New agent preset…");
  await page.keyboard.press("Enter");
  const editor = page.locator(".agent-preset-editor");
  await editor.waitFor();
  assert.equal(await editor.getAttribute("aria-label"), "New preset");
  await page.waitForFunction(() => document.activeElement?.id.endsWith("-name"));
  console.log("PASS picker action opens the creation form and focuses its name");
  await editor.getByLabel("Name", { exact: true }).fill("Custom avatar");
  assert.equal(await editor.getByLabel("ID", { exact: true }).inputValue(), "custom-avatar");
  await editor.getByLabel("ID", { exact: true }).fill("avatar-manual");
  await editor.getByLabel("Name", { exact: true }).fill("Custom avatar renamed");
  assert.equal(await editor.getByLabel("ID", { exact: true }).inputValue(), "avatar-manual", "manual ID survives name changes");
  await editor.getByLabel("Name", { exact: true }).fill("Custom avatar");

  const png = await page.evaluate(() => {
    const canvas = document.createElement("canvas"); canvas.width = 400; canvas.height = 200;
    const context = canvas.getContext("2d"); context.fillStyle = "red"; context.fillRect(0, 0, 200, 200);
    context.fillStyle = "blue"; context.fillRect(200, 0, 200, 200);
    return canvas.toDataURL("image/png").split(",")[1];
  });
  const upload = page.locator('.agent-preset-icon-picker input[type="file"]');
  const file = { name: "avatar.png", mimeType: "image/png", buffer: Buffer.from(png, "base64") };
  await upload.setInputFiles(file);
  await page.getByRole("group", { name: "Crop icon", exact: true }).waitFor();
  assert.equal(await editor.getByRole("button", { name: "Save", exact: true }).isDisabled(), true, "save waits for a crop decision");
  await editor.getByLabel("Zoom", { exact: true }).fill("2");
  await editor.getByLabel("Horizontal position").fill("0");
  await page.waitForFunction(() => document.querySelector('.agent-preset-icon-crop canvas')?.getContext("2d").getImageData(96, 96, 1, 1).data[0] === 255);
  await page.getByRole("button", { name: "Use icon", exact: true }).click();
  console.log("PASS square crop applied");
  await page.waitForFunction(() => document.querySelector('.agent-preset-editor [aria-label="Custom icon"] img')?.naturalWidth === 128);
  await editor.getByRole("button", { name: "Save", exact: true }).click();
  const row = page.getByRole("article", { name: "Custom avatar", exact: true });
  await row.waitFor();
  console.log("PASS image saved and listed");
  const stored = await row.locator("img").getAttribute("src");
  assert.match(stored, /^data:image\/png;base64,/u);
  assert.ok(stored.length < 128 * 1024);
  await row.getByRole("button", { name: "Copy", exact: true }).click();
  assert.equal(await page.locator('[aria-label="Custom icon"] img').getAttribute("src"), stored, "copy retains embedded image");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.locator("#management-back").click();
  await picker.click();
  const option = page.getByRole("menuitemradio", { name: /Custom avatar/u });
  assert.equal(await option.locator("img").getAttribute("src"), stored, "picker and list use the same persisted image");
  await option.click();
  await page.getByRole("button", { name: "Agent preset: Custom avatar", exact: true }).waitFor();
  console.log("PASS saved image renders in the composer picker");
  assert.equal(await page.locator(".agent-preset-choice .composer-choice img").getAttribute("src"), stored);
  await page.getByRole("button", { name: "Agent preset: Custom avatar", exact: true }).click();
  await page.getByRole("menuitem", { name: "New agent preset…", exact: true }).click();
  await editor.waitFor();
  await editor.getByRole("button", { name: "Cancel", exact: true }).click();
  assert.equal(await editor.count(), 0, "consumed create request does not reopen after cancel");
  await row.getByRole("button", { name: "Edit", exact: true }).click();

  for (const [badFile, message] of [
    [{ name: "vector.svg", mimeType: "image/svg+xml", buffer: Buffer.from("<svg/>") }, "Choose a PNG, JPEG or WebP image."],
    [{ name: "huge.png", mimeType: "image/png", buffer: Buffer.alloc(5 * 1024 * 1024 + 1) }, "Choose an image smaller than 5 MB."],
    [{ name: "broken.png", mimeType: "image/png", buffer: Buffer.from("broken") }, "Could not read this image. Try another file."],
  ]) {
    await upload.setInputFiles(badFile);
    await page.getByRole("alert").filter({ hasText: message }).waitFor();
    assert.equal(await page.locator('[aria-label="Custom icon"] img').getAttribute("src"), stored, "invalid upload retains the saved image");
  }
  await upload.setInputFiles(file);
  await page.getByRole("button", { name: "Use icon", exact: true }).waitFor();
  await page.locator(".agent-preset-icon-crop").getByRole("button", { name: "Cancel", exact: true }).click();
  assert.equal(await page.locator('[aria-label="Custom icon"] img').getAttribute("src"), stored, "cancelling crop keeps previous image");
  await upload.setInputFiles(file);
  await editor.getByLabel("Zoom", { exact: true }).fill("2");
  await editor.getByLabel("Horizontal position").fill("100");
  await page.getByRole("button", { name: "Use icon", exact: true }).click();
  assert.notEqual(await page.locator('[aria-label="Custom icon"] img').getAttribute("src"), stored, "replacement applies the new crop");
  const evidence = process.env.DEEPZORA_PRESET_EVIDENCE;
  if (evidence) {
    await mkdir(evidence, { recursive: true });
    await page.screenshot({ path: path.join(evidence, "custom-icon-editor.png"), fullPage: true });
  }
  await page.getByRole("button", { name: "Restore default icon", exact: true }).click();
  assert.equal(await page.locator('[aria-label="Custom icon"]').count(), 0);
  assert.equal(await page.getByRole("radio", { name: "bot", exact: true }).getAttribute("aria-checked"), "true");

  // A late decode must not undo a subsequent built-in icon choice.
  await page.evaluate(() => {
    const decode = window.createImageBitmap.bind(window);
    window.createImageBitmap = file => new Promise(resolve => { window.releasePresetImage = async () => resolve(await decode(file)); });
  });
  await upload.setInputFiles(file);
  await page.getByText("Reading image…", { exact: true }).waitFor();
  await page.getByRole("radio", { name: "search", exact: true }).click();
  await page.evaluate(() => window.releasePresetImage());
  assert.equal(await page.locator(".agent-preset-icon-crop").count(), 0, "late decode cannot reopen crop");
  assert.equal(await page.getByRole("radio", { name: "search", exact: true }).getAttribute("aria-checked"), "true");
  await page.getByRole("form").getByRole("button", { name: "Save", exact: true }).click();
  await row.waitFor();
  assert.equal(await row.locator("img").count(), 0, "reset and built-in choice persist");
  assert.deepEqual(errors, []);
  console.log("PASS preset picker creation, keyboard navigation, image crop/save/copy/readback, upload validation, cancel/reset and stale decode");
} finally {
  await browser?.close();
  await server.close();
}
