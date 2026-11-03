import { useCallback, useEffect, useRef, useState } from "react";
import type { TeamSecretsView } from "../generated/desktopContract.generated";
import { app } from "./bridge";
import { desktopHost } from "./desktopHost";
import { TEAM_CHANGED } from "./useTeam";

export function useTeamSecrets(){
  const [view,setView]=useState<TeamSecretsView|null>(null),[error,setError]=useState("");
  const generation=useRef(0);
  const reload=useCallback(async()=>{const own=++generation.current;try{const next=await app.GetTeamSecrets();if(own===generation.current){setView(next);setError("");}}catch{if(own===generation.current)setError("unavailable");}},[]);
  useEffect(()=>{const refresh=()=>{void reload();};refresh();const off=desktopHost().events.on("team:changed",refresh);window.addEventListener(TEAM_CHANGED,refresh);return()=>{generation.current++;off();window.removeEventListener(TEAM_CHANGED,refresh);};},[reload]);
  return {view,error,reload};
}
