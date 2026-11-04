import assert from "node:assert/strict";
import { TeamHistoryWindow } from "../lib/teamHistoryWindow";
import { TeamGroupProjection } from "../lib/teamGroupProjection";
import type { GroupLine, TeamAgentView, TeamGroupHistoryPage } from "../generated/desktopContract.generated";
import { t } from "../lib/i18n";

function page(start:number):TeamGroupHistoryPage {
  const lines:GroupLine[]=Array.from({length:32},(_,offset)=>({seq:start+offset,from:"a-0000000000000001",kind:"post",at:"2026-11-03T00:00:00Z",text:`message ${start+offset}`}));
  return {status:"ready",snapshot:"fixed",maxSequence:320,lines,olderCursor:start>1?`older:${start}`:"",newerCursor:start+31<320?`newer:${start+31}`:""};
}
const window=new TeamHistoryWindow();
assert.ok(window.accept(page(289),"latest"));
const lastLine=window.lines[0];
for(const start of [257,225,193,161,129,97,65,33,1]) {
  assert.ok(window.accept(page(start),"older"));
  assert.ok(window.lines.length<=96,"paging must release bodies from the opposite end");
  assert.equal(window.lines[0].seq,start);
  if(start<=193)assert.ok(window.newerCursor,"reclaimed messages remain reachable");
}
for(const start of [97,129,161,193,225,257,289])assert.ok(window.accept(page(start),"newer"));
assert.equal(window.lines[window.lines.length-1].seq,320);
assert.equal(window.newerCursor,"");
assert.ok(!window.accept({...page(289),snapshot:"replaced"},"older"));
assert.ok(!window.accept({...page(289),status:"stale_cursor"},"latest"));
assert.equal(window.lines[window.lines.length-1].seq,320,"a stale result preserves the reader's current page");
assert.ok(window.accept(page(289),"latest"));
const resident=window.lines[0];
assert.ok(window.accept(page(289),"latest"));
assert.equal(window.lines[0],resident,"unchanged event identities preserve their objects");
assert.notEqual(resident,lastLine,"reclaimed data was released and refetched");

const member:TeamAgentView={id:"a-0000000000000001",name:"Ada With Spaces",role:"",instructions:"",avatar:{glyph:"bot"},folder:"",initialModel:"",permission:"read-only",main:true,pinned:false,hidden:false,sessionPath:"",createdBy:"user",createdAt:"2026-11-03T00:00:00Z"};
const projection=new TeamGroupProjection();
const first=projection.update("group",window.lines,[member],t);
const second=projection.update("group",window.lines,[{...member}],t);
assert.equal(first,second,"unchanged projections preserve node and order references");
assert.equal(first[0].speaker?.id,member.id);
assert.equal(first[0].speaker?.name,"Ada With Spaces");
const renamed=projection.update("group",window.lines,[{...member,name:"New Name"}],t);
assert.equal(renamed[0].id,first[0].id,"renaming does not remount the message host");
assert.equal(renamed[0].speaker?.name,"New Name");
const human:GroupLine={seq:321,from:"user",kind:"post",at:"2026-11-03T00:00:00Z",text:"hello"};
const userNamedAgent={...member,name:"user"};
const identities=projection.update("group",[human,window.lines[0]],[userNamedAgent],t);
assert.equal(identities[0].kind,"user");
assert.equal(identities[1].kind,"assistant","display name cannot impersonate the human role");
console.log("PASS Team bounded history and speaker projection");
