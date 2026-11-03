// Opt-in, bounded real-provider qualification. connection.json is a private
// local file containing {provider: ProviderView, key, model}; never upload it.
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { packagedSmokeEnv } from "./smoke-env.mjs";
import { closeAndVerify } from "./smoke-lifecycle.mjs";
import { parseServiceReady, waitForSmokeCondition } from "./smoke-poll.mjs";

if(process.argv.length!==5)throw new Error("usage: team-real-model-smoke.mjs <packaged-executable> <private-connection.json> <evidence-directory>");
const desktop=resolve(dirname(fileURLToPath(import.meta.url)),"..");
const {_electron}=createRequire(join(desktop,"electron/package.json"))("playwright");
const connection=JSON.parse(readFileSync(process.argv[3],"utf8"));
const home=mkdtempSync(join(tmpdir(),"deepzora-team-real-home-"));
const evidence=resolve(process.argv[4]);mkdirSync(evidence,{recursive:true});
const checks=[],errors=[];let application,page;
const id=()=>crypto.randomUUID();
const invoke=(method,args=[])=>page.evaluate(({method,args})=>window.deepzoraDesktop.invoke(method,args),{method,args});
const record=check=>{checks.push(check);console.log(`PASS ${check}`);};
const wait=predicate=>waitForSmokeCondition(predicate,{timeout:180000,interval:100});
const historyHas=async(tabID,text)=>JSON.stringify(await invoke("SessionHistoryWindowForTab",[tabID,{anchor:"newest",limit:64}])).includes(text);
writeFileSync(join(evidence,"real-model-result.json"),JSON.stringify({completed:false,platform:process.platform,kind:connection.provider.kind,model:connection.model}));
// The isolated candidate gets finite cost/time policy and explicit per-call
// output limits, as well as the client-side deadline which closes its host.
writeFileSync(join(home,"config.toml"),'[agent]\ntask_cost_budget = 0.03\ntask_time_budget_minutes = 2\n');
try{
  application=await _electron.launch({executablePath:resolve(process.argv[2]),args:[],env:packagedSmokeEnv(process.env,home),timeout:60000});
  await wait(async()=>{for(const candidate of application.windows())try{await candidate.evaluate(()=>window.deepzoraDesktop.invoke("Version",[]));page=candidate;return true;}catch{}return false;});
  page.on("pageerror",error=>errors.push(error.message));
  assert.equal(await application.evaluate(({app})=>app.isPackaged),true);
  const saved=await invoke("SaveProviderWithKey",[{...connection.provider,name:"team-real",models:[connection.model],default:connection.model,modelOverrides:[{model:connection.model,maxOutputTokens:1024}]},connection.key]);
  const providerName=typeof saved==="string"?saved:saved?.name||"team-real";
  const model=`${providerName}/${connection.model}`;
  await invoke("SetDefaultModel",[model]);
  async function create(name,main){const team=await invoke("GetTeam");const saved=await invoke("SaveTeamAgent",[{expectedRevision:team.roster.revision,requestId:id(),settings:{name,main,permission:"read-only",initialModel:model,instructions:"For this acceptance test, follow the requested reply exactly. Do not use tools."}}]);assert.equal(saved.outcome,"applied");return saved.targetId;}
  const chief=await create("Real-model Chief",true),ada=await create("Real-model Ada",false);
  const tab=await invoke("OpenTeamAgent",[chief,""]);
  await invoke("SubmitToTabWithID",[tab.id,"Reply exactly TEAM_REAL_PRIVATE_OK. Do not use tools.",id()]);
  await wait(()=>historyHas(tab.id,"TEAM_REAL_PRIVATE_OK"));
  // A user prompt also contains the marker: require an actual assistant row.
  await wait(async()=>{const page=await invoke("SessionHistoryWindowForTab",[tab.id,{anchor:"newest",limit:64}]);return page.messages.some(message=>message.role==="assistant"&&JSON.stringify(message).includes("TEAM_REAL_PRIVATE_OK"));});
  record("real provider completed a member private conversation");
  const team=await invoke("GetTeam");const group=await invoke("SaveTeamGroup",[{expectedRevision:team.roster.revision,requestId:id(),settings:{name:"Real-model group",members:[chief,ada],admin:chief,mode:"everyone",relayLimit:2}}]);assert.equal(group.outcome,"applied");
  const post=await invoke("SendTeamGroupMessage",[{groupId:group.targetId,requestId:id(),text:"Each member: reply exactly TEAM_REAL_GROUP_OK. Do not use tools or hand off to anyone.",mentions:[],everyone:true}]);assert.equal(post.outcome,"accepted");
  await wait(async()=>{const history=await invoke("ReadTeamGroupHistory",[group.targetId,""]);return new Set(history.lines.filter(line=>[chief,ada].includes(line.from)&&line.text?.includes("TEAM_REAL_GROUP_OK")).map(line=>line.from)).size===2;});
  record("real provider completed a sequential group round for two members");
  const today=new Date();const day=`${today.getFullYear()}-${String(today.getMonth()+1).padStart(2,"0")}-${String(today.getDate()).padStart(2,"0")}`;
  let rows;
  await wait(async()=>{rows=await invoke("GetTeamUsage",[{from:day,to:day}]);return rows.reduce((sum,row)=>sum+row.requests,0)>=3;});
  assert.ok(rows.every(row=>[chief,ada].includes(row.agentId)),"real calls lost member attribution");
  assert.ok(rows.some(row=>row.groupId===group.targetId));
  assert.deepEqual(errors,[]);record("real private/group usage has member and group attribution");
  const shellPid=await application.evaluate(()=>process.pid);
  const ready=parseServiceReady(readFileSync(join(home,"desktop-shell/logs/shell.log"),"utf8"));assert.ok(ready);
  await closeAndVerify(application,{shellPid,servicePid:ready.pid});application=null;
  writeFileSync(join(evidence,"real-model-result.json"),JSON.stringify({completed:true,platform:process.platform,kind:connection.provider.kind,model:connection.model,checks,requests:rows.reduce((sum,row)=>sum+row.requests,0),tokens:rows.reduce((sum,row)=>sum+row.tokens,0)},null,2));
}finally{if(application)await application.close();rmSync(home,{recursive:true,force:true});}
