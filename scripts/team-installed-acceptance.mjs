// Qualify the distributed installer, not only its packaging input directory.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { desktopAssets, desktopName } from "./desktop-downloads.mjs";

const root=resolve(dirname(fileURLToPath(import.meta.url)),"..");
const platform={darwin:"darwin",linux:"linux",win32:"windows"}[process.platform];
const architecture={arm64:"arm64",x64:"amd64"}[process.arch];
const target=`${platform}-${architecture}`,version="v0.0.0";
const evidence=resolve(process.argv[2]||join(root,"desktop/build/team-acceptance",target,"installed"));
const spec=desktopAssets.find(asset=>asset.platform===target&&/\.(dmg|deb)$|-Setup\.exe$/.test(asset.suffix));
assert.ok(spec,"no native installer for this target");
const artifact=join(root,"dist",desktopName(version,spec));
assert.ok(existsSync(artifact),"native installer is missing");
// System package and account registration changes are restricted to clean,
// disposable hosted runners. macOS only copies into a task-owned directory.
if(platform!=="darwin")assert.ok(process.env.GITHUB_ACTIONS==="true"&&process.env.RUNNER_ENVIRONMENT==="github-hosted","installer acceptance requires a disposable hosted runner");
mkdirSync(evidence,{recursive:true});
const sha=spawnSync("git",["rev-parse","HEAD"],{cwd:root,encoding:"utf8"}).stdout.trim();
const hash=()=>createHash("sha256").update(readFileSync(artifact)).digest("hex");
const artifactSHA256=hash();
writeFileSync(join(evidence,"installer-result.json"),JSON.stringify({completed:false,sha,target,artifactSHA256}));
function run(command,args){
  const result=spawnSync(command,args,{cwd:root,env:process.env,stdio:"inherit"});
  if(result.error)throw result.error;
  assert.equal(result.status,0,`${command} failed`);
}
function qualify(executable,buildFile){
  const build=JSON.parse(readFileSync(buildFile,"utf8"));
  assert.equal(build.commit,sha,"installed source differs from the requested candidate");
  assert.equal(build.version,version);
  run(process.execPath,["desktop/packaging/smoke.mjs",executable]);
  run(process.execPath,["desktop/packaging/team-native-smoke.mjs",executable,evidence]);
}
if(platform==="darwin"){
  const temporary=mkdtempSync(join(tmpdir(),"deepzora-team-dmg-"));
  const mount=join(temporary,"mounted"),installed=join(temporary,"installed");
  mkdirSync(mount);mkdirSync(installed);
  let mounted=false;
  try{
    run("hdiutil",["attach","-readonly","-nobrowse","-mountpoint",mount,artifact]);mounted=true;
    run("ditto",[join(mount,"Deepzora.app"),join(installed,"Deepzora.app")]);
    run("hdiutil",["detach",mount]);mounted=false;
    const app=join(installed,"Deepzora.app/Contents");
    qualify(join(app,"MacOS/Deepzora"),join(app,"Resources/build.json"));
  }finally{
    if(mounted){run("hdiutil",["detach",mount]);mounted=false;}
    rmSync(temporary,{recursive:true,force:true});
  }
}else if(platform==="linux"){
  const present=spawnSync("dpkg-query",["-W","-f=${Status}","deepzora-desktop"],{encoding:"utf8"});
  assert.notEqual(present.stdout.trim(),"install ok installed","refusing to replace an existing installation");
  run("sudo",["apt-get","install","-y",artifact]);
  try{qualify("/usr/lib/deepzora/app/Deepzora","/usr/lib/deepzora/app/resources/build.json");}
  finally{run("sudo",["dpkg","--remove","deepzora-desktop"]);}
}else{
  run("pwsh",["-NoProfile","-File","scripts/team-windows-installer-acceptance.ps1","-InstallerPath",artifact,"-ExpectedVersion",version,"-EvidenceDirectory",join(evidence,"windows")]);
}
assert.equal(hash(),artifactSHA256,"installer bytes changed during acceptance");
writeFileSync(join(evidence,"installer-result.json"),JSON.stringify({completed:true,sha,target,artifactSHA256,provider:"controlled loopback fixture",secretRequired:process.env.TEAM_REQUIRE_SECRETS==="1"},null,2));
