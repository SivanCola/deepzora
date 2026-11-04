// Real Desktop frontend with the existing preview bridge. This gate verifies
// UI ownership and navigation; it does not claim real model or native-host QA.
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { startPreviewServer } from "./vite-preview-server.mjs";

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),"..");
process.env.PLAYWRIGHT_BROWSERS_PATH = !process.env.PLAYWRIGHT_BROWSERS_PATH || process.env.PLAYWRIGHT_BROWSERS_PATH === ".pw-browsers"
  ? path.join(root, ".pw-browsers") : process.env.PLAYWRIGHT_BROWSERS_PATH;
const { chromium } = await import("playwright");
const port=Number(process.env.DEEPZORA_TEAM_BROWSER_PORT??4689);
const external=process.env.DEEPZORA_TEAM_BASE_URL;
const preview=external?null:await startPreviewServer(root,port);
const browser=await chromium.launch({headless:true});
const output=path.join(os.tmpdir(),"deepzora-team-browser");await mkdir(output,{recursive:true});
const errors=[];
const page=await browser.newPage({locale:"en-US",viewport:{width:1440,height:1000}});
page.on("pageerror",error=>errors.push(error.message));
try {
  await page.goto(`${external??`http://127.0.0.1:${port}`}/?mock=deepseek_upgrade&bench=1&benchTeamSecrets=1&platform=windows`,{waitUntil:"domcontentloaded"});
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
  const ada=settings.locator('.agent-preset-row').filter({has:page.getByRole('heading',{name:'Ada With Spaces',exact:true})});
  await ada.getByRole('button',{name:'Edit',exact:true}).click();
  await settings.getByRole('button',{name:'Messaging channels',exact:true}).click();
  await settings.locator('.bot-provider-card').first().click();
  const channel=settings.locator('.bot-channel-editor');
  const member=channel.getByRole('button',{name:'Team member',exact:true});
  assert.equal(await member.innerText(),'Ada With Spaces');
  assert.equal(await member.isDisabled(),true);
  assert.equal(await channel.getByRole('button',{name:'Model service',exact:true}).count(),0,'member channels must not own a second model config');
  await channel.getByRole('textbox',{name:'Connection name',exact:true}).fill('Member Weixin');
  await channel.getByRole('button',{name:'Cancel',exact:true}).click();
  await page.locator('.settings-leave-dialog').waitFor();
  await page.getByRole('button',{name:'Discard and leave',exact:true}).click();
  await settings.locator('.settings-subpage-header').first().getByRole('button').first().click();
  await settings.getByRole('button',{name:'Cancel',exact:true}).click();
  await ada.getByRole('button',{name:'Archive member',exact:true}).click();
  const archived=settings.locator('.settings-section').filter({has:page.getByRole('heading',{name:'Archived members',exact:true})});
  await archived.getByRole('button',{name:'Restore member',exact:true}).click();
  await ada.getByRole('button',{name:'Edit',exact:true}).waitFor();
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
  await schedules.getByRole('button',{name:'Edit',exact:true}).click();
  await schedules.getByRole('textbox',{name:'Name',exact:true}).fill('Release review retained draft');
  // Saving a separate Team setting changes the roster while the schedule
  // editor retains its original CAS revision, just as background work can.
  const reviewPrefs=settings.locator('.settings-section').filter({has:page.getByRole('heading',{name:'Automatic memory review',exact:true})});
  await reviewPrefs.getByRole('checkbox',{name:'Automatic memory review',exact:true}).check();
  const savePrefs=reviewPrefs.getByRole('button',{name:'Save',exact:true});
  await savePrefs.click();
  await page.waitForFunction(el=>el.disabled,await savePrefs.elementHandle());
  await schedules.getByRole('button',{name:'Save',exact:true}).click();
  await schedules.getByRole('button',{name:'Review latest saved version',exact:true}).click();
  await schedules.getByRole('heading',{name:'Latest saved version',exact:true}).waitFor();
  assert.equal(await schedules.getByRole('textbox',{name:'Name',exact:true}).inputValue(),'Release review retained draft');
  assert.equal(await schedules.getByRole('button',{name:'Save',exact:true}).isDisabled(),true);
  for(const width of [1440,400]) {
    await page.setViewportSize({width,height:1000});
    await schedules.locator('.settings-section').filter({has:page.getByRole('heading',{name:'Latest saved version',exact:true})}).scrollIntoViewIfNeeded();
    assert.ok(await schedules.evaluate(el=>el.scrollWidth-el.clientWidth)<=1,`schedule conflict review fits ${width}px`);
    await page.screenshot({path:path.join(output,`team-schedule-conflict-${width}.png`)});
  }
  await page.setViewportSize({width:1440,height:1000});
  await schedules.getByRole('button',{name:'Keep my draft for the next save',exact:true}).click();
  await schedules.getByRole('button',{name:'Save',exact:true}).click();
  await schedules.getByRole('heading',{name:'Release review retained draft',exact:true}).waitFor();
  await schedules.getByRole('button',{name:'Run now',exact:true}).click();
  await schedules.getByText('Scheduled input accepted. Check the target conversation for execution and approvals.',{exact:true}).waitFor();
  const secrets=settings.locator('.settings-section').filter({has:page.getByRole('heading',{name:'Team secrets',exact:true})});
  await secrets.getByRole('button',{name:'Add secret',exact:true}).click();
  await secrets.getByRole('textbox',{name:'Variable name',exact:true}).fill('FIXTURE');
  const password=secrets.locator('input[type=password]');
  await password.fill('browser-only-disposable-value');
  assert.equal(await secrets.getByRole('checkbox',{name:'Chief',exact:true}).isChecked(),false,'grants default to nobody');
  await secrets.getByRole('checkbox',{name:'Chief',exact:true}).check();
  await secrets.getByRole('button',{name:'Save',exact:true}).click();
  await secrets.getByRole('heading',{name:'FIXTURE',exact:true}).waitFor();
  assert.equal((await secrets.textContent()).includes('browser-only-disposable-value'),false,'saved metadata never displays the value');
  await secrets.getByRole('button',{name:'Edit',exact:true}).click();
  assert.equal(await password.inputValue(),'','authorization edit does not fetch a credential');
  await secrets.getByRole('checkbox',{name:'Chief',exact:true}).uncheck();
  await secrets.getByRole('button',{name:'Save',exact:true}).click();
  await secrets.getByText('No members authorized',{exact:true}).waitFor();
  await secrets.getByRole('button',{name:'Delete',exact:true}).click();
  assert.equal(await secrets.getByRole('heading',{name:'FIXTURE',exact:true}).count(),1,'delete requires a second confirmation');
  await secrets.getByRole('button',{name:'Delete',exact:true}).click();
  await secrets.getByRole('heading',{name:'FIXTURE',exact:true}).waitFor({state:'detached'});
  await secrets.getByRole('button',{name:'Forget retired masking values',exact:true}).click();
  await secrets.getByRole('button',{name:'Remove masking of old values, including in history?',exact:true}).click();
  await secrets.getByRole('button',{name:'Forget retired masking values',exact:true}).waitFor({state:'detached'});
  await settings.getByRole('heading',{name:'Member usage',exact:true}).waitFor();
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
  console.log("PASS Team creation, archive/restore, memory, review opt-in, schedules with explicit conflict review and retained drafts, secret password/explicit grants/revocation/confirmed retirement, usage, mentions, navigation, drafts and responsive group chat (preview fixture)");
} finally {
  await page.close();await browser.close();if(preview)await preview.httpServer.close();
}
