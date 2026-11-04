import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { app } from "./bridge";
import { desktopHost } from "./desktopHost";
import { TeamHistoryWindow } from "./teamHistoryWindow";
import { TEAM_CHANGED } from "./useTeam";
import type { HistoryLoadOutcome } from "./useController";

export function useTeamGroupHistory(groupID:string) {
  const window=useMemo(()=>new TeamHistoryWindow(),[groupID]);
  const [,publish]=useState(0);
  const [loading,setLoading]=useState(false);
  const [error,setError]=useState<string|null>(null);
  const [newAvailable,setNewAvailable]=useState(false);
  const request=useRef(0);
  const active=useRef(true);
  const following=useRef(true);
  const flight=useRef<Promise<HistoryLoadOutcome>|null>(null);
  const refreshQueued=useRef(false);
  const drainRefresh=useRef(()=>{});
  const marked=useRef("");
  const markRead=()=>{
    if(!active.current||!following.current||window.newerCursor||document.visibilityState!=="visible")return;
    const last=window.lines[window.lines.length-1]?.seq??0,key=`${window.snapshot}:${last}`;
    if(!last||!window.snapshot||marked.current===key)return;
    marked.current=key;
    void app.MarkTeamGroupRead(groupID,window.snapshot,last).catch(()=>{if(marked.current===key)marked.current="";});
  };
  const markCurrent=useRef(markRead);markCurrent.current=markRead;
  const load=useCallback((direction:"older"|"newer"|"latest"):Promise<HistoryLoadOutcome>=>{
    if(!active.current)return Promise.resolve("empty");
    if(flight.current)return flight.current;
    const cursor=direction==="older"?window.olderCursor:direction==="newer"?window.newerCursor:"";
    if(direction!=="latest"&&!cursor)return Promise.resolve("empty");
    const sequence=++request.current;
    setLoading(true);setError(null);
    const run=async():Promise<HistoryLoadOutcome>=>{
      try {
        const page=await app.ReadTeamGroupHistory(groupID,cursor);
        if(!active.current||request.current!==sequence)return "empty";
        if(!window.accept(page,direction)){setError("stale_cursor");return "stale";}
        publish(revision=>revision+1);if(direction==="latest")setNewAvailable(false);
        queueMicrotask(()=>markCurrent.current());
        return page.lines.length?"loaded":"empty";
      }catch(cause){if(active.current&&request.current===sequence)setError(cause instanceof Error?cause.message:String(cause));return "empty";}
      finally{if(active.current&&request.current===sequence)setLoading(false);if(flight.current===pending){flight.current=null;queueMicrotask(()=>drainRefresh.current());}}
    };
    const pending=Promise.resolve().then(run);flight.current=pending;
    return pending;
  },[groupID,window]);
  drainRefresh.current=()=>{if(active.current&&refreshQueued.current){refreshQueued.current=false;if(following.current&&!window.newerCursor)void load("latest");else setNewAvailable(true);}};
  useEffect(()=>{
    active.current=true;flight.current=null;refreshQueued.current=false;setNewAvailable(false);void load("latest");
    const changed=()=>{if(flight.current){refreshQueued.current=true;return;}if(following.current&&!window.newerCursor)void load("latest");else setNewAvailable(true);};
    const off=desktopHost().events.on("team:changed",changed);
    const offResync=desktopHost().events.on("desktop:resync",changed);
    windowAdd(changed);
    const visibility=()=>markCurrent.current();document.addEventListener("visibilitychange",visibility);
    return()=>{active.current=false;request.current++;off();offResync();windowRemove(changed);document.removeEventListener("visibilitychange",visibility);};
  },[load,window]);
  return {lines:window.lines,hasOlder:!!window.olderCursor,hasNewer:!!window.newerCursor||newAvailable,loading,error,newAvailable,
    loadOlder:()=>load("older"),loadNewer:(latest=false)=>load(latest||!window.newerCursor?"latest":"newer"),refresh:()=>load("latest"),
    onFollowChange:(value:boolean)=>{following.current=value;if(value)markCurrent.current();}};
}
function windowAdd(callback:()=>void){globalThis.window.addEventListener(TEAM_CHANGED,callback);}
function windowRemove(callback:()=>void){globalThis.window.removeEventListener(TEAM_CHANGED,callback);}
