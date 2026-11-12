#!/usr/bin/env python3
"""Offline end-to-end acceptance with the real CLI, Git transport and package state."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

cli = Path(sys.argv[1]).resolve()
with tempfile.TemporaryDirectory(prefix="deepzora-market-") as tmp:
    t = Path(tmp)
    repo = t / "upstream"
    repo.mkdir()
    env = dict(os.environ, HOME=str(t), DEEPZORA_HOME=str(t/"home"),
               GIT_CONFIG_GLOBAL=str(t/"gitconfig"), XDG_CONFIG_HOME=str(t/"config"),
               DEEPZORA_PLUGIN_CATALOG=str(t/"index.json"), DEEPZORA_PLUGIN_TAGGED=str(t/"tagged.json"))
    def run(*args, ok=True):
        r = subprocess.run(args, cwd=repo, env=env, text=True, capture_output=True, timeout=60)
        assert (r.returncode == 0) == ok, (args, r.stdout, r.stderr)
        return r.stdout.strip()
    def git(*args):
        return run("git",*args)
    def dz(*args,ok=True):
        return run(str(cli),"plugin",*args,ok=ok)
    git("init","-q")
    git("config","user.name","Acceptance")
    git("config","user.email","test@example.invalid")
    git("config","--global","url."+repo.as_uri()+".insteadOf","https://github.com/test/plugins.git")
    pkg=repo/"packages"/"kit"
    (pkg/"skills"/"kit").mkdir(parents=True)
    def commit(version):
        (pkg/"deepzora-plugin.json").write_text(json.dumps({"apiVersion":"deepzora.com/plugin/v2","name":"kit","version":version,"contributes":{"skills":["skills"]}}))
        (pkg/"skills"/"kit"/"SKILL.md").write_text("---\nname: kit\ndescription: Test skill\n---\n"+version)
        git("add",".");git("commit","-qm",version)
        return git("rev-parse","HEAD")
    sha=commit("1.0.0")
    git("tag","latest")
    def release(version,sha):
        return dict(id="test/kit",name="Kit",repo="test/plugins",path="packages/kit",version=version,commit=sha,manifestName="kit",manifestKind="deepzora",summary={"en":"Code review","zh":"审查"})
    def publish(rows,channel="index"):
        (t/(channel+".json")).write_text(json.dumps(dict(schemaVersion=1,releases=rows)))
    row=release("1.0.0",sha)
    publish([row]);publish([dict(row,id="third/kit")],"tagged")
    spec="catalog:test/kit"
    assert len(json.loads(dz("search","--all","review"))["entries"])==1
    plan=json.loads(dz("install",spec,"--dry-run"))
    assert plan["actions"][0]["commit"]==sha and plan["actions"][0]["skillCount"]==1
    new=commit("1.1.0")
    git("tag","-f","latest")
    dz("install",spec,"--yes","--plan-id","wrong",ok=False)
    dest=t/"home"/"plugins"/"kit"
    assert not dest.exists()
    dz("install",spec,"--yes","--plan-id",plan["planId"])
    skill=dest/"skills"/"kit"/"SKILL.md"
    assert skill.read_text().endswith("1.0.0")
    state=lambda:json.loads((t/"home"/"plugin-packages.json").read_text())["plugins"][0]
    assert state()["commit"]==sha
    dz("doctor","kit")
    dz("disable","kit")
    nextrow=release("1.1.0",new)
    publish([row,nextrow])
    update=json.loads(dz("update","kit","--dry-run"))
    publish([row,dict(nextrow,yanked=True)])
    dz("update","kit","--yes","--plan-id",update["planId"],ok=False)
    assert state()["commit"]==sha
    publish([row,nextrow])
    dz("update","kit","--yes","--plan-id",update["planId"])
    assert state()["commit"]==new and not state()["enabled"]
    assert skill.read_text().endswith("1.1.0")
    other=dict(nextrow,id="someone/kit",repo="test/other")
    git("config","--global","--add","url."+repo.as_uri()+".insteadOf","https://github.com/test/other.git")
    publish([nextrow,other])
    dz("install","catalog:someone/kit","--replace","--dry-run",ok=False)
    # The other supported manifest path: one mapped skill plus a skipped feature.
    compat=repo/"compat"
    (compat/".claude-plugin").mkdir(parents=True)
    (compat/".claude-plugin/plugin.json").write_text(json.dumps(dict(name="compat",version="1.0.0",outputStyles="./styles")))
    (compat/"skills/compat").mkdir(parents=True)
    (compat/"skills/compat/SKILL.md").write_text("---\nname: compat\ndescription: Test\n---\nOK")
    git("add",".");git("commit","-qm","Claude package")
    cr=dict(row,id="test/compat",path="compat",manifestName="compat",manifestKind="claude",commit=git("rev-parse","HEAD"))
    publish([cr])
    cp=json.loads(dz("install","catalog:test/compat","--dry-run"))
    assert cp["actions"][0]["skillCount"]==1
    dz("install","catalog:test/compat","--yes","--plan-id",cp["planId"])
    print("PASS: search, deduplication, immutable Git snapshot after tag moved, plan identity, withdrawal, upgrade, disabled state, collision and compatible package.")

