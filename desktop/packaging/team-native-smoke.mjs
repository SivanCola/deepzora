// Packaged Electron + Go Team acceptance in a disposable state directory.
// The loopback provider is a controlled protocol fixture, not a real model.
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { createRequire } from "node:module";
import { mkdtempSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { packagedSmokeEnv } from "./smoke-env.mjs";
import { closeAndVerify } from "./smoke-lifecycle.mjs";
import { parseServiceReady, waitForSmokeCondition } from "./smoke-poll.mjs";

const desktop=resolve(dirname(fileURLToPath(import.meta.url)),"..");
const require=createRequire(join(desktop,"electron/package.json"));
const {_electron}=require("playwright");
const executable=resolve(process.argv[2]);
const evidence=resolve(process.argv[3]||join(tmpdir(),"deepzora-team-native"));
const home=mkdtempSync(join(tmpdir(),"deepzora-team-native-home-"));
const value="native-disposable-secret-38195072";
const checks=[],errors=[];
let application,page,releaseBackground,backgroundStarted=false,requests=0,secretObserved=false;
mkdirSync(evidence,{recursive:true});
writeFileSync(join(evidence,"team-result.json"),JSON.stringify({completed:false,platform:process.platform}));
const record=check=>{checks.push(check);console.log(`PASS ${check}`);};
const fixture=createServer(async(req,res)=>{
  let raw="";for await(const chunk of req)raw+=chunk;
  assert.equal(raw.includes(value),false,"a registered credential reached the provider");
  requests++;
  const messages=JSON.parse(raw).messages||[];
  const human=[...messages].reverse().find(message=>message.role==="user"&&/NATIVE_(PRIVATE|GROUP|BACKGROUND|SCHEDULE|APPROVAL|SECRET)/.test(message.content||""));
  const marker=/NATIVE_(PRIVATE|GROUP|BACKGROUND|SCHEDULE|APPROVAL|SECRET)/.exec(human?.content||"")?.[0]||"NATIVE_GROUP";
  const callID=`native-${marker}`;
  const completed=messages.some(message=>message.role==="tool"&&message.tool_call_id===callID);
  let delta={content:`${marker}_OK`};
  if(marker==="NATIVE_BACKGROUND"&&!completed){backgroundStarted=true;await new Promise(resolve=>{releaseBackground=resolve;});}
  if(marker==="NATIVE_APPROVAL"&&!completed)delta={tool_calls:[{index:0,id:callID,type:"function",function:{name:"remember_shared",arguments:JSON.stringify({tier:"team",operation:"save",name:"native-fact",description:"Native reviewed fact",body:"Use plain language",type:"feedback"})}}]};
  if(marker==="NATIVE_SECRET"){
    if(completed){secretObserved=JSON.stringify(messages).includes("[secret:NATIVE]");assert.ok(secretObserved,`tool output was not masked: ${JSON.stringify(messages.filter(message=>message.tool_call_id===callID))}`);}
    else delta={tool_calls:[{index:0,id:callID,type:"function",function:{name:process.platform==="win32"?"pwsh":"bash",arguments:JSON.stringify({command:process.platform==="win32"?"[Console]::Out.Write($env:DEEPZORA_SECRET_NATIVE)":"printf '%s' \"$DEEPZORA_SECRET_NATIVE\"",description:"Print the disposable authorized native test credential"})}}]};
  }
  res.writeHead(200,{"Content-Type":"text/event-stream"});
  res.end(`data: ${JSON.stringify({choices:[{index:0,delta,finish_reason:delta.tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:11,completion_tokens:2,total_tokens:13}})}\n\ndata: [DONE]\n\n`);
});
await new Promise(resolve=>fixture.listen(0,"127.0.0.1",resolve));
writeFileSync(join(home,"config.toml"),`default_model = "fixture/model"\n[desktop]\nprovider_access = ["fixture"]\n[[providers]]\nname = "fixture"\nkind = "openai"\nbase_url = "http://127.0.0.1:${fixture.address().port}/v1"\nmodels = ["model"]\ndefault = "model"\napi_key_env = "TEAM_NATIVE_FIXTURE_KEY"\nno_proxy = true\n`);
const invoke=(method,args=[])=>page.evaluate(({method,args})=>window.deepzoraDesktop.invoke(method,args),{method,args});
const id=()=>crypto.randomUUID();
async function launch(){
  application=await _electron.launch({executablePath:executable,args:[],env:{...packagedSmokeEnv(process.env,home),TEAM_NATIVE_FIXTURE_KEY:"loopback-only"},timeout:60000});
  await waitForSmokeCondition(async()=>{
    for(const candidate of application.windows())try{if(await candidate.evaluate(()=>Boolean(window.deepzoraDesktop))){await candidate.evaluate(()=>window.deepzoraDesktop.invoke("Version",[]));page=candidate;return true;}}catch{}
    return false;
  },{timeout:60000});
  page.setDefaultTimeout(30000);page.on("pageerror",error=>errors.push(error.message));
  assert.equal(await application.evaluate(({app})=>app.isPackaged),true);
}
async function close(){
  const shellPid=await application.evaluate(()=>process.pid);
  const ready=parseServiceReady(readFileSync(join(home,"desktop-shell/logs/shell.log"),"utf8"));
  assert.ok(ready,"service handshake evidence is required");
  try{await closeAndVerify(application,{shellPid,servicePid:ready.pid});}
  catch(error){
    const logs=join(home,"desktop-shell/logs");
    for(const name of readdirSync(logs)){
      const path=join(logs,name);
      try{writeFileSync(join(evidence,`failed-${name}`),readFileSync(path,"utf8").split(value).join("[secret:NATIVE]"));}catch{}
    }
    throw error;
  }
  application=null;
}
const settle=()=>waitForSmokeCondition(async()=>(await invoke("GetTeam")).seats.every(seat=>!seat.work?.running&&!seat.work?.pendingPrompt));
async function historyHas(tabID,text){return JSON.stringify(await invoke("SessionHistoryWindowForTab",[tabID,{anchor:"newest",limit:64}])).includes(text);}
async function send(tab,text){await invoke("SubmitToTabWithID",[tab.id,text,id()]);await waitForSmokeCondition(()=>historyHas(tab.id,`${text}_OK`));await settle();}
async function create(name,main){
  const team=await invoke("GetTeam");
  const result=await invoke("SaveTeamAgent",[{expectedRevision:team.roster.revision,requestId:id(),settings:{name,role:"native qualification",instructions:"Follow the user's task.",permission:"danger-full-access",initialModel:"fixture/model",main}}]);
  assert.equal(result.outcome,"applied",JSON.stringify(result));return result.targetId;
}
try{
  await launch();
  const chief=await create("Native Chief",true),ada=await create("Native Ada",false);
  let tab=await invoke("OpenTeamAgent",[chief,""]);
  const initialSession=tab.session?.sessionId||tab.sessionId;
  await send(tab,"NATIVE_PRIVATE");record("packaged member creation and private Controller/provider/history flow");
  const team=await invoke("GetTeam");
  const group=await invoke("SaveTeamGroup",[{expectedRevision:team.roster.revision,requestId:id(),settings:{name:"Native group",members:[chief,ada],admin:chief,mode:"everyone",relayLimit:2}}]);
  assert.equal(group.outcome,"applied",JSON.stringify(group));
  const post=await invoke("SendTeamGroupMessage",[{groupId:group.targetId,requestId:id(),text:"NATIVE_GROUP",mentions:[],everyone:true}]);
  assert.equal(post.outcome,"accepted");
  await waitForSmokeCondition(async()=>{const history=await invoke("ReadTeamGroupHistory",[group.targetId,""]);return history.lines.filter(line=>line.from!=="user"&&line.text?.includes("NATIVE_GROUP_OK")).length===2;});
  await settle();record("packaged sequential group round with two member identities");
  await invoke("SubmitToTabWithID",[tab.id,"NATIVE_BACKGROUND",id()]);
  await waitForSmokeCondition(()=>backgroundStarted);
  await invoke("CloseTabWithPolicy",[tab.id,"keep_running"]);
  const adaTab=await invoke("OpenTeamAgent",[ada,""]);
  releaseBackground();
  await waitForSmokeCondition(async()=>{const restored=await invoke("GetTeam");return restored.seats.some(seat=>seat.agentId===chief&&!seat.groupId&&!seat.work?.running);});
  assert.equal((await invoke("ListTabs")).find(tab=>tab.active)?.id,adaTab.id,"background completion stole focus");
  tab=await invoke("OpenTeamAgent",[chief,""]);
  await waitForSmokeCondition(()=>historyHas(tab.id,"NATIVE_BACKGROUND_OK"));record("closing a page retains background work without stealing focus");
  await invoke("SubmitToTabWithID",[tab.id,"NATIVE_APPROVAL",id()]);
  let pending;
  await waitForSmokeCondition(async()=>{pending=(await invoke("PendingPromptIdentitiesForTab",[tab.id]))[0];return Boolean(pending);});
  assert.equal((await invoke("GetTeamMemory",[{tier:"team"}])).facts.length,0,"fresh approval was bypassed under full access");
  await invoke("ResolvePromptForTab",[tab.id,pending.promptId,pending.turnId,pending.runtimeEpoch,pending.kind,{allow:true}]);
  await waitForSmokeCondition(()=>historyHas(tab.id,"NATIVE_APPROVAL_OK"));
  await settle();
  assert.equal((await invoke("GetTeamMemory",[{tier:"team"}])).facts.length,1);record("fresh human approval binds the shared-memory change under full access");
  const schedules=await invoke("GetTeamSchedules"),current=await invoke("GetTeam");
  const saved=await invoke("SaveTeamSchedule",[{expectedRevision:schedules.revision,expectedRosterRevision:current.roster.revision,requestId:id(),schedule:{name:"Native schedule",agent:chief,prompt:"NATIVE_SCHEDULE",repeat:"once",timezone:"Etc/UTC",anchor:new Date(Date.now()+3600000).toISOString(),days:[],enabled:false}}]);
  assert.equal(saved.outcome,"applied",JSON.stringify(saved));
  const run=await invoke("RunTeamSchedule",[{id:saved.targetId,expectedRevision:saved.view.revision,requestId:id()}]);assert.equal(run.outcome,"accepted",JSON.stringify(run));
  await waitForSmokeCondition(()=>historyHas(tab.id,"NATIVE_SCHEDULE_OK"));await settle();record("scheduler occurrence enters the Team inbox with independent provenance");
  const secret=await invoke("GetTeamSecrets");
  if(secret.available){
    const savedSecret=await invoke("SaveTeamSecret",[{expectedRevision:secret.revision,expectedRosterRevision:secret.rosterRevision,requestId:id(),operation:"save",name:"NATIVE",purpose:"isolated native fixture",value,agents:[chief]}]);
    assert.equal(savedSecret.outcome,"applied",JSON.stringify(savedSecret));assert.equal(JSON.stringify(savedSecret).includes(value),false);
    await send(tab,"NATIVE_SECRET");assert.ok(secretObserved);record("native authorized shell environment and masked provider/tool/history output");
  }else{assert.notEqual(process.env.TEAM_REQUIRE_SECRETS,"1","secret qualification requires the masking gate enabled");record("secret feature remains gated; this run does not qualify secret availability");}
  await close();
  function scan(dir){for(const entry of readdirSync(dir,{withFileTypes:true})){if(entry.name==="team-secret-slots")continue;const path=join(dir,entry.name);if(entry.isDirectory())scan(path);else if(entry.isFile())assert.equal(readFileSync(path).includes(Buffer.from(value)),false,`credential leaked into ${entry.name}`);}}
  scan(home);record("no credential in persisted sessions, metadata, statistics or logs outside its private slots");
  await launch();tab=await invoke("OpenTeamAgent",[chief,""]);
  assert.equal(tab.session?.sessionId||tab.sessionId,initialSession,"restart replaced the stable private identity");
  await waitForSmokeCondition(()=>historyHas(tab.id,"NATIVE_PRIVATE_OK"));
  assert.equal((await invoke("GetTeam")).roster.groups[0].id,group.targetId);record("packaged restart restores stable private/group identities and history");
  assert.deepEqual(errors,[]);await close();
  writeFileSync(join(evidence,"team-result.json"),JSON.stringify({completed:true,platform:process.platform,checks,provider:"controlled loopback fixture",requests,secretObserved},null,2));
}finally{
  releaseBackground?.();if(application)await application.close();await new Promise(resolve=>fixture.close(resolve));rmSync(home,{recursive:true,force:true});
}
