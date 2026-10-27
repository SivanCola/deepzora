import assert from "node:assert/strict";
import { executeClearSession } from "../app-runtime/sessionRuntimeOwner";

let owned = true;
const authority = { checkpoint() { if (!owned) throw new Error("stale"); }, ownsUI: () => owned };
const calls: string[] = [];
const ports = {
  clearSession: async () => { calls.push("local"); },
};

await executeClearSession(ports, authority);
assert.deepEqual(calls, ["local"]);
calls.length = 0;
owned = false;
await assert.rejects(executeClearSession(ports, authority), /stale/);
assert.deepEqual(calls, [], "stale source cannot clear a replacement session");
console.log("session clear owner: source/UI ownership passed");
