import assert from "node:assert/strict";
import { JSDOM } from "jsdom";
import React, { act } from "react";
import type {
  BotLoginView,
  BotConfig,
  BotConnection,
} from "../lib/botChannels";

const dom = new JSDOM('<div id="root"></div>', {
  url: "http://localhost/",
  pretendToBeVisual: true,
});
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  Node: dom.window.Node,
  HTMLElement: dom.window.HTMLElement,
  HTMLInputElement: dom.window.HTMLInputElement,
  Event: dom.window.Event,
  MouseEvent: dom.window.MouseEvent,
  localStorage: dom.window.localStorage,
  requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window),
  cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window),
  IS_REACT_ACT_ENVIRONMENT: true,
});
Object.defineProperty(globalThis, "navigator", {
  configurable: true,
  value: dom.window.navigator,
});
window.matchMedia = () =>
  ({
    matches: true,
    addEventListener() {},
    removeEventListener() {},
  }) as unknown as MediaQueryList;
const { createRoot } = await import("react-dom/client");
const { BotChannelsSettingsPage } =
  await import("../components/BotChannelsSettingsPage");
const { SettingsExperienceProvider } =
  await import("../components/SettingsExperience");
const { LocaleProvider } = await import("../lib/i18n");
const { installDesktopHostStub } = await import("./desktopHostStub");
let nextID = 0;
let resolvePoll: (view: BotLoginView) => void;
let notifyPoll: () => void;
let pollStarted: Promise<void>;
const saved: (BotConfig & {
  secret: string;
  loginId?: string;
  connect?: boolean;
})[] = [];
let existing: BotConnection[] = [];
const testedModels: string[] = [];
installDesktopHostStub({
  ListBotChannels: async () => ({
    connections: existing,
    members: [
      {
        id: "a-0000000000000001",
        name: "Fixture member",
        model: "fixture/model",
        permission: "read-only",
        folder: "/workspace",
      },
    ],
    sessions: [
      { id: "session", title: "Local session", workspace: "/workspace" },
    ],
    models: [
      {
        ref: "fixture/model",
        provider: "fixture",
        providerLabel: "Fixture",
        model: "model",
        configured: true,
      },
    ],
    workspaces: [{ id: "global", title: "Global", root: "" }],
  }),
  TestBotModel: async (ref) => {
    testedModels.push(ref);
  },
  BeginWeixinBotLogin: async () => ({
    id: `login-${++nextID}`,
    image: "data:image/png;base64,AA==",
    status: "pending",
  }),
  PollWeixinBotLogin: async () =>
    new Promise<BotLoginView>((resolve) => {
      resolvePoll = resolve;
      notifyPoll();
    }),
  BeginFeishuBotLogin: async (provider) => ({
    id: `login-${++nextID}`,
    image: "data:image/png;base64,AA==",
    status: "pending",
    provider: provider as "feishu" | "lark",
    interval: 1,
  }),
  PollFeishuBotLogin: async () =>
    new Promise<BotLoginView>((resolve) => {
      resolvePoll = resolve;
      notifyPoll();
    }),
  SaveBotConnection: async (input) => {
    saved.push(input);
  },
});
const rootEl = document.getElementById("root")!;
const root = createRoot(rootEl);
const flush = () => new Promise<void>((resolve) => setTimeout(resolve, 0));
const button = (text: string) => {
  const found = [...document.querySelectorAll("button")].find(
    (b) => b.textContent?.trim() === text,
  );
  assert.ok(found, text);
  return found;
};
const input = (label: string) => {
  const field = [...rootEl.querySelectorAll(".settings-field")].find(
    (e) => e.querySelector(".settings-field__label")?.textContent === label,
  );
  assert.ok(field, label);
  return field.querySelector("input")!;
};
async function click(element: HTMLElement) {
  await act(async () => {
    element.click();
    await flush();
  });
}
async function fill(label: string, value: string) {
  await act(async () => {
    const element = input(label);
    Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      "value",
    )!.set!.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
async function mount(index = 0) {
  await act(async () => {
    root.render(
      <LocaleProvider>
        <SettingsExperienceProvider key={nextID}>
          <BotChannelsSettingsPage />
        </SettingsExperienceProvider>
      </LocaleProvider>,
    );
    await flush();
  });
  await click(
    rootEl.querySelectorAll<HTMLButtonElement>(".bot-provider-card")[index],
  );
}
async function begin(feishu = false) {
  pollStarted = new Promise<void>((resolve) => {
    notifyPoll = resolve;
  });
  await click(
    button(
      feishu ? "Scan to create and configure bot" : "Sign in with Weixin QR",
    ),
  );
  await act(async () => {
    await pollStarted;
  });
  return `login-${nextID}`;
}
async function complete(id: string, userId?: string) {
  await act(async () => {
    resolvePoll({ id, image: "", status: "success", userId });
    await flush();
  });
}

try {
  await mount();
  let id = await begin();
  await complete(id, "scanner@im.wechat");
  assert.equal(input("Allowed chat ID").value, "scanner@im.wechat");
  assert.equal(input("Allowed sender ID").value, "scanner@im.wechat");
  assert.equal(input("Bot token / app secret").value, "");
  assert.match(rootEl.textContent!, /Both access IDs have been filled/);
  assert.equal(button("Save only").disabled, false);
  await click(button("Save only"));
  assert.equal(saved[0].chatId, "scanner@im.wechat");
  assert.equal(saved[0].senderId, "scanner@im.wechat");
  assert.equal(saved[0].loginId, id);
  assert.equal(saved[0].secret, "");
  assert.equal(saved[0].connect, false, "save-only must not connect");

  await mount();
  id = await begin();
  await fill("Allowed chat ID", "custom-chat");
  await fill("Allowed sender ID", "custom-sender");
  await complete(id, "scanner@im.wechat");
  assert.equal(input("Allowed chat ID").value, "custom-chat");
  assert.equal(input("Allowed sender ID").value, "custom-sender");

  await mount();
  id = await begin();
  await fill("Bot token / app secret", "manual-token");
  await complete(id, "stale@im.wechat");
  assert.equal(input("Allowed chat ID").value, "");
  assert.equal(input("Bot token / app secret").value, "manual-token");

  await mount();
  id = await begin();
  await complete(id);
  assert.equal(input("Allowed chat ID").value, "");
  assert.equal(input("Allowed sender ID").value, "");
  assert.match(rootEl.textContent!, /did not return a user ID/);
  assert.equal(button("Save only").disabled, true);
  for (const index of [1, 2]) {
    await mount(index);
    id = await begin(true);
    await act(async () => {
      resolvePoll({
        id,
        image: "",
        status: "success",
        appId: "cli_fixture",
        provider: "lark",
        userId: "ou_scanner",
        chatId: "openid:ou_scanner",
      });
      await flush();
    });
    assert.equal(input("App ID").value, "cli_fixture");
    assert.equal(input("Allowed sender ID").value, "ou_scanner");
    assert.equal(input("Allowed chat ID").value, "openid:ou_scanner");
    assert.equal(input("Bot token / app secret").value, "");
    assert.equal(button("Save only").disabled, false);
    await click(button("Save only"));
    assert.equal(saved[saved.length - 1]?.provider, "lark");
    assert.equal(saved[saved.length - 1]?.loginId, id);
  }
  // Model testing is an explicit action separate from Save and Connect.
  nextID++;
  await mount(3);
  await fill("Allowed chat ID", "chat");
  await fill("Allowed sender ID", "sender");
  await fill("Bot token / app secret", "bot-fixture");
  assert.equal(rootEl.textContent?.includes("Fixture member"), true);
  assert.equal(rootEl.textContent?.includes("Replies go only"), true);
  await click(button("Save and connect"));
  assert.equal(saved.at(-1)?.mode, "member");
  assert.equal(saved.at(-1)?.memberId, "a-0000000000000001");
  assert.equal(saved.at(-1)?.connect, true);
  existing = [
    {
      ...saved.at(-1)!,
      enabled: true,
      hasSecret: true,
      status: "connected",
      error: "",
    },
  ];
  await click(
    rootEl.querySelector<HTMLButtonElement>('button[aria-label="Refresh"]')!,
  );
  await click(button("Edit"));
  assert.equal(button("Save and reconnect").disabled, true);
  await fill("Connection name", "Edited connected bot");
  await click(button("Save and reconnect"));
  assert.equal(saved.at(-1)?.connect, true);
  assert.equal(saved.at(-1)?.secret, "");
  existing = [];
  nextID++;
  await mount(3);
  await fill("Allowed chat ID", "chat");
  await fill("Allowed sender ID", "sender");
  await fill("Bot token / app secret", "fixture");
  await click(button("Cancel"));
  await click(button("Save and continue"));
  assert.equal(
    saved.at(-1)?.connect,
    false,
    "navigation save must not enable a new bot",
  );
  console.log(
    "PASS Weixin/Feishu/Lark QR setup, auto-fill, credential omission, manual edit protection, stale result rejection and missing-ID feedback",
  );
} finally {
  await act(async () => root.unmount());
  dom.window.close();
}
