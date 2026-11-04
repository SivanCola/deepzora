import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import { startPreviewServer } from "./vite-preview-server.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const output =
  process.env.DEEPZORA_BOT_ARTIFACTS ?? "/tmp/deepzora-bot-channels";
await mkdir(output, { recursive: true });
const server = await startPreviewServer(root, 4698);
const browser = await chromium.launch({ headless: true });
const errors = [];
try {
  for (const locale of ["en", "zh"]) {
    const page = await browser.newPage({
      locale: locale === "zh" ? "zh-CN" : "en-US",
      viewport: { width: 1400, height: 1000 },
    });
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto(
      "http://127.0.0.1:4698/?mock=deepseek_upgrade&bench=1&platform=windows",
    );
    await page
      .locator("textarea.composer__input:not([aria-hidden=true])")
      .waitFor();
    await page
      .getByRole("button", {
        name: locale === "zh" ? "机器人渠道" : "Bot Channels",
        exact: true,
      })
      .click();
    const nav = page.locator(".settings-center__navgroups");
    await nav
      .getByRole("button", {
        name: locale === "zh" ? "机器人渠道" : "Bot Channels",
        exact: true,
      })
      .click();
    await page.locator(".bot-provider-card:enabled").first().waitFor();
    assert.equal(await page.locator(".bot-provider-card").count(), 4);
    for (const theme of ["dark", "light"]) {
      await page.evaluate(
        (theme) => (document.documentElement.dataset.theme = theme),
        theme,
      );
      for (const width of [1400, 900, 400]) {
        await page.setViewportSize({ width, height: 1000 });
        await page.screenshot({
          path: path.join(output, `${locale}-${theme}-${width}.png`),
          fullPage: true,
        });
        const overflow = await page
          .locator(".settings-center__content")
          .evaluate((el) => el.scrollWidth - el.clientWidth);
        assert.ok(
          overflow <= 1,
          `${locale} ${theme} ${width} overflow ${overflow}`,
        );
      }
    }
    await page.setViewportSize({ width: 1400, height: 1000 });
    for (let index = 0; index < 4; index++) {
      await page.locator(".bot-provider-card").nth(index).click();
      await page.locator(".bot-channel-editor").waitFor();
      if (index === 0) {
        await page.locator(".bot-login button").click();
        await page.locator(".bot-login [role=alert]").waitFor();
        assert.equal(
          await page.locator(".bot-login img").count(),
          0,
          "failed login must not show a fabricated QR code",
        );
      }
      const name = page.getByRole("textbox", {
        name: locale === "zh" ? "连接名称" : "Connection name",
        exact: true,
      });
      await name.fill(`Test ${index}`);
      await page
        .getByLabel(locale === "zh" ? "允许的聊天 ID" : "Allowed chat ID", {
          exact: true,
        })
        .fill("123");
      await page
        .getByLabel(locale === "zh" ? "允许的发送者 ID" : "Allowed sender ID", {
          exact: true,
        })
        .fill("456");
      await page
        .getByLabel("Bot Token / App Secret", { exact: true })
        .or(page.getByLabel("Bot token / app secret", { exact: true }))
        .fill("test-only-secret");
      if (index === 1 || index === 2)
        await page.getByLabel("App ID", { exact: true }).fill("app-test");
      if (index === 0) {
        await page
          .getByRole("button", {
            name:
              locale === "zh"
                ? "指定模型与专属会话"
                : "Dedicated conversation and model",
            exact: true,
          })
          .click();
        const service = page
          .locator(".bot-channel-editor")
          .getByRole("button", {
            name: locale === "zh" ? "模型服务" : "Model service",
            exact: true,
          });
        await service.click();
        await page
          .getByRole("option", { name: "Missing key", exact: true })
          .click();
        assert.equal(
          await page
            .getByRole("button", {
              name: locale === "zh" ? "仅保存" : "Save only",
              exact: true,
            })
            .isDisabled(),
          true,
          "missing model credential must block save",
        );
        await service.click();
        await page
          .getByRole("option", { name: "Preview service", exact: true })
          .click();
        await page
          .getByRole("button", {
            name: locale === "zh" ? "模型" : "Model",
            exact: true,
          })
          .click();
        await page.getByRole("option", { name: "pro", exact: true }).click();
        await page
          .getByRole("button", {
            name: locale === "zh" ? "测试模型" : "Test model",
            exact: true,
          })
          .click();
        await page.locator(".bot-channel-editor [role=alert]").last().waitFor();
      }
      await page.setViewportSize({ width: 400, height: 1000 });
      assert.ok(
        (await page
          .locator(".settings-center__content")
          .evaluate((el) => el.scrollWidth - el.clientWidth)) <= 1,
        "editor overflow",
      );
      await page.screenshot({
        path: path.join(output, `${locale}-editor-${index}.png`),
        fullPage: true,
      });
      await page.setViewportSize({ width: 1400, height: 1000 });
      if (index === 0) {
        await nav
          .getByRole("button", {
            name: locale === "zh" ? "通用" : "General",
            exact: true,
          })
          .click();
        await page.locator(".settings-leave-dialog").waitFor();
        await page
          .getByRole("button", {
            name: locale === "zh" ? "继续编辑" : "Keep editing",
            exact: true,
          })
          .click();
        assert.equal(await name.inputValue(), "Test 0");
      }
      await page
        .getByRole("button", {
          name: locale === "zh" ? "仅保存" : "Save only",
          exact: true,
        })
        .click();
      await page.locator(".bot-channel-editor").waitFor({ state: "detached" });
      const row = page.locator(".bot-connection").filter({
        has: page.getByRole("heading", {
          name: `Test ${index}`,
          exact: true,
        }),
      });
      await row.waitFor();
      await row
        .getByRole("button", {
          name: locale === "zh" ? "编辑" : "Edit",
          exact: true,
        })
        .click();
      assert.equal(
        await page.locator('input[type="password"]').inputValue(),
        "",
        "saved credential must not return",
      );
      if (index === 0) {
        assert.equal(
          await page
            .getByRole("button", {
              name:
                locale === "zh"
                  ? "指定模型与专属会话"
                  : "Dedicated conversation and model",
              exact: true,
            })
            .getAttribute("aria-pressed"),
          "true",
        );
        assert.equal(
          await page
            .getByRole("button", {
              name: locale === "zh" ? "模型" : "Model",
              exact: true,
            })
            .innerText(),
          "pro",
        );
        await page
          .getByRole("button", {
            name:
              locale === "zh" ? "跟随绑定会话" : "Follow shared conversation",
            exact: true,
          })
          .click();
        await page
          .getByRole("button", {
            name: locale === "zh" ? "共享会话" : "Shared conversation",
            exact: true,
          })
          .waitFor();
        // Mode changes also participate in the normal unsaved-draft guard.
        await page
          .getByRole("button", {
            name: locale === "zh" ? "取消" : "Cancel",
            exact: true,
          })
          .click();
        await page.locator(".settings-leave-dialog").waitFor();
        await page
          .getByRole("button", {
            name: locale === "zh" ? "放弃更改并离开" : "Discard and leave",
            exact: true,
          })
          .click();
        continue;
      }
      await page
        .getByRole("button", {
          name: locale === "zh" ? "取消" : "Cancel",
          exact: true,
        })
        .click();
    }
    assert.equal(await page.locator(".bot-connection").count(), 4);
    await page
      .locator(".bot-connection")
      .first()
      .getByRole("button", {
        name: locale === "zh" ? "连接" : "Connect",
        exact: true,
      })
      .click();
    await page.getByRole("alert").first().waitFor();
    const failureText = await page.getByRole("alert").first().textContent();
    await page.waitForTimeout(3500);
    assert.equal(
      await page.getByRole("alert").first().textContent(),
      failureText,
      "polling must preserve action failure",
    );
    const first = page.locator(".bot-connection").first();
    await first
      .getByRole("button", {
        name: locale === "zh" ? "删除" : "Delete",
        exact: true,
      })
      .click();
    await first
      .getByRole("button", {
        name: locale === "zh" ? "确认删除连接？" : "Delete connection?",
        exact: true,
      })
      .click();
    await page.waitForFunction(
      () => document.querySelectorAll(".bot-connection").length === 3,
    );
    await page.close();
  }
  assert.deepEqual(errors, []);
  console.log(
    "PASS bot channel cards, bilingual themes, responsive editors, draft protection, CRUD, secret omission and connection failure",
  );
  console.log(`Screenshots: ${output}`);
} finally {
  await browser.close();
  await new Promise((resolve, reject) =>
    server.httpServer.close((error) => (error ? reject(error) : resolve())),
  );
}
