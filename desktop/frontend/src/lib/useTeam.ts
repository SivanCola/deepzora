import { useCallback, useEffect, useRef, useState } from "react";
import type { TeamView } from "../generated/desktopContract.generated";
import { app } from "./bridge";
import { desktopHost } from "./desktopHost";

export const TEAM_CHANGED="deepzora:team-changed";
export const notifyTeamChanged=()=>window.dispatchEvent(new Event(TEAM_CHANGED));

export function useTeam(enabled=true) {
  const [view,setView]=useState<TeamView|null>(null);
  const [loading,setLoading]=useState(true);
  const [error,setError]=useState<string|null>(null);
  const sequence=useRef(0);
  const reload=useCallback(async()=>{
    if(!enabled)return;
    const request=++sequence.current;
    setLoading(true);
    try {
      const next=await app.GetTeam();
      if(sequence.current!==request)return;
      setView(next);setError(null);
    }catch(cause){if(sequence.current===request)setError(cause instanceof Error?cause.message:String(cause));}
    finally {if(sequence.current===request)setLoading(false);}
  },[enabled]);
  useEffect(()=>{
    if(!enabled){sequence.current++;setView(null);setLoading(false);return;}
    const refresh=()=>{void reload();};
    refresh();
    const off=desktopHost().events.on("team:changed",refresh);
    const resync=desktopHost().events.on("desktop:resync",refresh);
    window.addEventListener(TEAM_CHANGED,refresh);
    window.addEventListener("focus",refresh);
    return()=>{sequence.current++;off();resync();window.removeEventListener(TEAM_CHANGED,refresh);window.removeEventListener("focus",refresh);};
  },[enabled,reload]);
  return {view,loading,error,reload};
}
