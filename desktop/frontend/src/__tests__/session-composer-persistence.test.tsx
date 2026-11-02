import assert from "node:assert/strict";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import { installDesktopHostStub } from "./desktopHostStub";
import { makeSessionUIMock } from "../lib/sessionUIMock";
import { flushAllSessionComposers, resumeSessionComposerEditing, sendPersistedComposer, useSessionComposerPersistence } from "../lib/sessionComposerPersistence";
import type { SessionRef } from "../lib/sessionRef";
import { SessionInputRecovery } from "../components/SessionInputRecovery";
import { LocaleProvider } from "../lib/i18n";

const dom = new JSDOM("<div id='root'></div>", {url:"http://localhost"});
Object.assign(globalThis,{window:dom.window,document:dom.window.document,IS_REACT_ACT_ENVIRONMENT:true});
const backend = makeSessionUIMock(async()=>{});
installDesktopHostStub(backend);
let editor!: ReturnType<typeof useSessionComposerPersistence>;
const a:SessionRef={hostId:"local",sessionId:"persist-a"}, b:SessionRef={hostId:"local",sessionId:"persist-b"};
function Probe({refValue}:{refValue:SessionRef}) { editor=useSessionComposerPersistence(refValue,refValue.sessionId); return <LocaleProvider><SessionInputRecovery input={editor}/></LocaleProvider>; }
const root=createRoot(document.getElementById("root")!);
async function paint(ref:SessionRef) { await act(async()=>{root.render(<Probe refValue={ref}/>); await new Promise(resolve=>setTimeout(resolve,0));}); }
function patch(text:string) { const target=editor.target!; target.onPatch!(target.draftId,target.generation,{text}); }
async function persist() { await act(async()=>{await flushAllSessionComposers();resumeSessionComposerEditing();}); }
try {
 await paint(a);
 await act(async()=>patch("source A"));
 const source=editor.target!;
 await paint(b);
 await act(async()=>patch("source B"));
 await act(async()=>source.onPatch!(source.draftId,source.generation,{text:"late A"}));
 await persist();
 assert.equal(JSON.parse((await backend.GetSessionComposerState(a)).contentJson).text,"late A");
 assert.equal(JSON.parse((await backend.GetSessionComposerState(b)).contentJson).text,"source B");

 // Exit waits for source-bound attachment work and persists its late result.
 let resolveTask!:()=>void;
 const attachment=new Promise<void>(resolve=>{resolveTask=resolve;});
 const target=editor.target!;
 const work=attachment.then(()=>target.onPatch!(target.draftId,target.generation,{text:"attachment completed"}));
 target.trackTask!(target.draftId,target.generation,work);
 let exited=false;
 const exit=act(async()=>{await flushAllSessionComposers();exited=true;});
 assert.equal(exited,false); resolveTask(); await exit;
 assert.equal(JSON.parse((await backend.GetSessionComposerState(b)).contentJson).text,"attachment completed");
 await act(async()=>resumeSessionComposerEditing());

 // A second writer must not replace what this window is editing.
 const prior=await backend.GetSessionComposerState(b);
 await backend.SaveSessionComposerState({ref:b,expectedRevision:prior.revision,contentJson:'{"text":"other window"}',contentVersion:1});
 await act(async()=>patch("local window"));
 await persist();
 assert.equal(editor.blocked,false);
 assert.equal(JSON.parse((await backend.GetSessionComposerState(b)).contentJson).text,"local window");

 // A normal in-flight send keeps its durable occupancy without flashing the
 // recovery warning while the host acknowledgement is delayed.
 let acknowledge!: () => void, started!: () => void;
 const acknowledgement = new Promise<void>(resolve => { acknowledge = resolve; });
 const sendingStarted = new Promise<void>(resolve => { started = resolve; });
 let normalSend!: Promise<void>;
 await act(async()=>{
  normalSend = sendPersistedComposer(b.sessionId,"local window","local window","normal-delayed",async()=>{started();await acknowledgement;});
  await sendingStarted;
 });
 assert.equal(editor.blocked,true,"normal send still owns the draft until acknowledged");
 assert.equal(editor.attention,false,"normal submission marker is not an uncertain outcome");
 assert.equal(document.querySelector('.session-input-recovery'),null,"no recovery warning during normal send");
 await act(async()=>{acknowledge();await normalSend;});
 assert.equal(editor.blocked,false);
 assert.equal(document.querySelector('.session-input-recovery'),null,"normal acknowledgement does not flash a recovery warning");
 await act(async()=>patch("local window"));

 // A lost response preserves input and must never call the send callback again.
 const rejected=Object.assign(new Error("settings pending"),{data:{submissionOutcome:"not_accepted"}});
 await act(async()=>{await assert.rejects(sendPersistedComposer(b.sessionId,"local window","local window","configuration-rejected",async()=>{throw rejected;}));});
 assert.equal(editor.blocked,false,"definite rejection releases submission occupancy");
 assert.equal(JSON.parse((await backend.GetSessionComposerState(b)).contentJson).text,"local window","configuration rejection preserves input");
 let sends=0;
 const unknown=Object.assign(new Error("submission not accepted\ntransport acknowledgement lost"),{data:{submissionOutcome:"unknown"}});
 await act(async()=>{await assert.rejects(sendPersistedComposer(b.sessionId,"local window","local window","submission-lost",async()=>{sends++;throw unknown;}));});
 await act(async()=>editor.retry());
 assert.equal(sends,1); assert.equal(editor.blocked,true);
 assert.equal(editor.attention,true,"a genuinely unknown result still needs attention");
 assert.ok(document.querySelector('.session-input-recovery'),"unknown outcome retains the recovery prompt");
 assert.equal(JSON.parse((await backend.GetSessionComposerState(b)).contentJson).text,"local window");
 await act(async()=>{await assert.rejects(sendPersistedComposer(b.sessionId,"local window","local window","different-id",async()=>{sends++;}));});
 assert.equal(sends,1);
 await act(async()=>root.unmount());
 console.log("session input: source isolation, exit barrier, CAS and unknown submission passed");
} finally { resumeSessionComposerEditing();dom.window.close(); }
