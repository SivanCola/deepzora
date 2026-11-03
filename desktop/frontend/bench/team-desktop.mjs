// Real Desktop frontend with the existing preview bridge. This gate verifies
// UI ownership and navigation; it does not claim real model or native-host QA.
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";
import { startPreviewServer } from "./vite-preview-server.mjs";

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),"..");
const port=Number(process.env.DEEPZORA_TEAM_BROWSER_PORT??4689);
const external=process.env.DEEPZORA_TEAM_BASE_URL;
const preview=external?null:await startPreviewServer(root,port);
const browser=await chromium.launch({headless:true});
const output=path.join(os.tmpdir(),"deepzora-team-browser");await mkdir(output,{recursive:true});
const errors=[];
const page=await browser.newPage({locale:"en-US",viewport:{width:1440,height:1000}});
page.on("pageerror",error=>errors.push(error.message));
try {
  await page.goto(`${external??`http://127.0.0.1:${port}`}/?mock=deepseek_upgrade&bench=1&platform=windows`,{waitUntil:"domcontentloaded"});
  await page.locator(".team-sidebar").waitFor();
  await page.locator(".team-sidebar").getByRole("button",{name:"Manage team",exact:true}).click();
  const settings=page.locator(".settings-page--team");await settings.waitFor();
  for(const name of ["Chief","Ada With Spaces"]) {
    await settings.getByRole("button",{name:"New member",exact:true}).click();
    await settings.getByRole("textbox",{name:"Name",exact:true}).fill(name);
    await settings.locator("button[type=submit]").click();
    await settings.getByRole("heading",{name,exact:true}).waitFor();
  }
  await settings.getByRole("button",{name:"Move Ada With Spaces up",exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('.settings-page--team .agent-preset-row h3')?.textContent==='Ada With Spaces');
  await settings.getByRole("button",{name:"New group",exact:true}).click();
  await settings.getByRole("textbox",{name:"Name",exact:true}).fill("Project group");
  for(const name of ["Chief","Ada With Spaces"])await settings.getByRole("checkbox",{name,exact:true}).check();
  await settings.locator("button[type=submit]").click();
  await settings.getByRole("heading",{name:"Project group",exact:true}).waitFor();
  const memory=settings.locator('.settings-section').filter({has:page.getByRole('heading',{name:'Team memory',exact:true})});
  assert.equal(await settings.getByRole('checkbox',{name:'Automatic memory review',exact:true}).isChecked(),false,'review is opt-in');
  await memory.getByRole('button',{name:'Add memory',exact:true}).click();
  await memory.getByRole('textbox',{name:'Summary',exact:true}).fill('Working language');
  await memory.getByRole('textbox',{name:'Content',exact:true}).fill('Use Chinese for explanations');
  await memory.getByRole('button',{name:'Save',exact:true}).click();
  await memory.getByRole('heading',{name:'Working language',exact:true}).waitFor();
  const schedules=settings.locator('.settings-section').filter({has:page.getByRole('heading',{name:'Team schedules',exact:true})});
  await schedules.getByRole('button',{name:'New schedule',exact:true}).click();
  await schedules.getByRole('textbox',{name:'Name',exact:true}).fill('Release review');
  await schedules.getByRole('textbox',{name:'Task',exact:true}).fill('Review the release checklist');
  await schedules.getByRole('button',{name:'Save',exact:true}).click();
  await schedules.getByRole('heading',{name:'Release review',exact:true}).waitFor();
  await schedules.getByRole('button',{name:'Run now',exact:true}).click();
  await schedules.getByText('Scheduled input accepted. Check the target conversation for execution and approvals.',{exact:true}).waitFor();
  await page.getByRole("button",{name:"Back to workspace",exact:true}).click();
  const sidebar=page.locator(".team-sidebar");
  await sidebar.getByRole("button",{name:/Project group/}).click();
  const chat=page.locator(".team-chat");await chat.waitFor();
  const input=chat.getByRole("textbox",{name:"Message the group…",exact:true});
  await input.fill("Retain this group draft");
  await chat.getByRole("checkbox",{name:"Ada With Spaces",exact:true}).check();
  await sidebar.getByRole("button",{name:/^Chief/}).click();
  await chat.waitFor({state:"detached"});
  await sidebar.getByRole("button",{name:/Project group/}).click();
  await input.waitFor();assert.equal(await input.inputValue(),"Retain this group draft");
  assert.equal(await chat.getByRole("checkbox",{name:"Ada With Spaces",exact:true}).isChecked(),true);
  await chat.getByRole("button",{name:"Send to group",exact:true}).click();
  await chat.getByText("Retain this group draft",{exact:true}).waitFor();
  await page.waitForFunction(()=>document.querySelector('.team-chat textarea')?.value==='');
  assert.equal(await chat.locator('[data-chat-kind="user"]').count(),1,"one accepted submission is rendered once");
  assert.equal(await chat.locator('[data-chat-kind="assistant"]').count(),0,"preview does not fabricate model replies");
  for(const width of [1440,900,400]) {
    await page.setViewportSize({width,height:1000});
    const overflow=await chat.evaluate(el=>el.scrollWidth-el.clientWidth);
    assert.ok(overflow<=1,`group chat fits ${width}px`);
    await page.screenshot({path:path.join(output,`team-group-${width}.png`)});
  }
  assert.deepEqual(errors,[],"no browser runtime errors");
  console.log("PASS Team creation, ordering, memory edits, review opt-in, schedule acceptance, structured mentions, canonical switching, retained drafts, at-most-once display and responsive group chat");
} finally {
  await page.close();await browser.close();if(preview)await preview.httpServer.close();
}
