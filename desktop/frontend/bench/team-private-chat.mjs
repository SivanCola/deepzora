// Full frontend navigation against the preview host; native/model acceptance
// remains a separate gate because this fixture does not execute a real model.
import assert from "node:assert/strict";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {startPreviewServer} from "./vite-preview-server.mjs";

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),"..");
process.env.PLAYWRIGHT_BROWSERS_PATH=!process.env.PLAYWRIGHT_BROWSERS_PATH||process.env.PLAYWRIGHT_BROWSERS_PATH===".pw-browsers"?path.join(root,".pw-browsers"):process.env.PLAYWRIGHT_BROWSERS_PATH;
const {chromium}=await import("playwright");
const port=Number(process.env.DEEPZORA_TEAM_PRIVATE_PORT??4691), external=process.env.DEEPZORA_TEAM_BASE_URL;
const preview=external?null:await startPreviewServer(root,port);
const browser=await chromium.launch({headless:true});
const page=await browser.newPage({locale:"en-US",viewport:{width:1440,height:1000}});
const errors=[];page.on("pageerror",error=>errors.push(error.message));
const sidebar=page.locator(".team-sidebar"),settings=page.locator(".settings-page--team");
async function assertPrivate(name) {
  await page.locator(".team-private-header strong").filter({hasText:name}).waitFor();
  await page.getByRole("textbox",{name:`Message ${name}…`,exact:true}).waitFor();
  assert.equal(await page.locator(".topicbar h1").innerText(),name);
  assert.equal(await page.locator(".topicbar h1").getAttribute("title"),name);
  assert.equal(await page.locator(".topicbar__workspace-label").innerText(),"Private chat");
  assert.equal(await page.locator(".composer-meta__control--agent-preset").count(),0);
  assert.equal(await page.locator(".composer-workspace-bar:not(.team-private-header)").count(),0);
  assert.ok(!/desktop-team-|a-[0-9a-f]{16}/.test(await page.locator(".topicbar").innerText()),"title must not display persisted seat identifiers");
  const model=page.locator(".composer-meta__control--model .modelsw__trigger");
  await model.waitFor();assert.equal(await model.isEnabled(),true,"session model remains selectable");
}
try {
  await page.goto(`${external??`http://127.0.0.1:${port}`}/?mock=deepseek_upgrade&bench=1&platform=windows`,{waitUntil:"domcontentloaded"});
  await sidebar.getByRole("button",{name:"Manage team",exact:true}).click();
  for(const name of ["Chief","Ada With Spaces"]) {
    await settings.getByRole("button",{name:"New member",exact:true}).click();
    await settings.getByRole("textbox",{name:"Name",exact:true}).fill(name);
    await settings.locator("button[type=submit]").click();
    await settings.getByRole("heading",{name,exact:true}).waitFor();
  }
  await page.getByRole("button",{name:"Back to workspace",exact:true}).click();
  await sidebar.getByRole("button",{name:/^Ada With Spaces/}).click();
  await assertPrivate("Ada With Spaces");
  for(const name of ["Chief","Ada With Spaces","Chief","Ada With Spaces"]) {
    await sidebar.getByRole("button",{name:new RegExp(`^${name}`)}).click();
    await assertPrivate(name);
  }
  await page.getByRole("textbox",{name:"Message Ada With Spaces…",exact:true}).fill("Private UI identity regression");
  await page.locator(".composer__btn--send").click();
  await page.locator('[data-chat-kind="user"]').filter({hasText:"Private UI identity regression"}).waitFor();
  await assertPrivate("Ada With Spaces");
  const permissions=page.locator(".composer-meta__control--approval button");
  for(let i=0;i<await permissions.count();i++)assert.equal(await permissions.nth(i).isDisabled(),true);
  await page.getByRole("button",{name:"Member settings",exact:true}).click();
  const member=settings.locator(".agent-preset-row").filter({has:page.getByRole("heading",{name:"Ada With Spaces",exact:true})});
  await member.getByRole("button",{name:"Edit",exact:true}).click();
  await settings.getByRole("textbox",{name:"Name",exact:true}).fill("Ada Renamed");
  await settings.locator("button[type=submit]").click();
  await settings.getByRole("heading",{name:"Ada Renamed",exact:true}).waitFor();
  await page.getByRole("button",{name:"Back to workspace",exact:true}).click();
  await assertPrivate("Ada Renamed");
  assert.deepEqual(errors,[]);
  console.log("PASS Team private full-app navigation, switching, empty/sent states, rename, titles, preset/workspace exclusion and permission/model affordances");
} finally {await page.close();await browser.close();if(preview)await preview.httpServer.close();}
