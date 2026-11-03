// Manual, native qualification only. Builds installable artifacts and runs
// production-shell acceptance; it never publishes or installs into user data.
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root=resolve(dirname(fileURLToPath(import.meta.url)),"..");
const platform={darwin:"darwin",linux:"linux",win32:"windows"}[process.platform];
const architecture={arm64:"arm64",x64:"amd64"}[process.arch];
if(!platform||!architecture)throw new Error("unsupported native qualification target");
const target=`${platform}/${architecture}`;
const evidence=join(root,"desktop/build/team-acceptance",`${platform}-${architecture}`);
mkdirSync(evidence,{recursive:true});
function run(command,args,options={}){
  const result=spawnSync(command,args,{cwd:root,env:process.env,stdio:"inherit",...options});
  if(result.error)throw result.error;
  if(result.status!==0)throw new Error(`${command} exited ${result.status}`);
}
const sha=spawnSync("git",["rev-parse","HEAD"],{cwd:root,encoding:"utf8"}).stdout.trim();
const version=`v0.0.0-team.${sha.slice(0,12)}`;
writeFileSync(join(evidence,"identity.json"),JSON.stringify({sha,target,version,provider:"controlled loopback fixture",complete:false},null,2));
// This regression temporarily enables the gate only in the test process and
// executes the platform's actual shell/parser and persistent credential slots.
run("go",["test","-count=1",".","-run","^TestDesktopTeamSecretSaveCommandRotationAndOutboundProjection$"],{cwd:join(root,"desktop")});
run("bash",["scripts/desktop-build.sh",target,version,"stable"]);
let executable;
if(platform==="darwin")executable=join(root,`desktop/build/candidate/darwin-${architecture}/Deepzora.app/Contents/MacOS/Deepzora`);
else if(platform==="windows")executable=join(root,"desktop/build/windows/signing-payload/app/Deepzora.exe");
else executable=join(root,"desktop/build/bin/app/Deepzora");
if(!existsSync(executable))throw new Error(`packaged executable missing: ${executable}`);
run(process.execPath,["desktop/packaging/smoke.mjs",executable]);
run(process.execPath,["desktop/packaging/team-native-smoke.mjs",executable,evidence]);
run(process.execPath,["desktop/electron/scripts/browser-runtime-smoke.mjs"],{env:{...process.env,DEEPZORA_BROWSER_EVIDENCE_PATH:join(evidence,"browser")}});
const body=readFileSync(executable);
writeFileSync(join(evidence,"identity.json"),JSON.stringify({sha,target,version,executableSHA256:createHash("sha256").update(body).digest("hex"),provider:"controlled loopback fixture",complete:true,secretRequired:process.env.TEAM_REQUIRE_SECRETS==="1"},null,2));
