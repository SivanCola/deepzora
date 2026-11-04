import { useEffect, useRef, useState } from "react";

// Keep one retry identity for identical input, including transport failures.
export function useTeamOperation() {
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const lock=useRef(false),alive=useRef(true);
  const receipt=useRef({digest:"",id:""});
  useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
  const run=async <T extends {outcome:string;reason?:string;message?:string}>(input:unknown,call:(id:string)=>Promise<T>,done?:(result:T)=>void|Promise<void>)=>{
    if(lock.current)return false;
    const digest=JSON.stringify(input);
    if(receipt.current.digest!==digest)receipt.current={digest,id:crypto.randomUUID()};
    lock.current=true;setBusy(true);setError("");
    try {
      const result=await call(receipt.current.id);
      if(!alive.current)return false;
      if(result.outcome!=="applied"&&result.outcome!=="accepted"){setError(result.reason==="conflict"?"conflict":result.message||result.reason||"Save failed");return false;}
      await done?.(result);return true;
    }catch(cause){if(alive.current)setError(String(cause));return false;}
    finally{lock.current=false;if(alive.current)setBusy(false);}
  };
  return {run,busy,error,clearError:()=>setError("")};
}
