import assert from "node:assert/strict";
import { createSessionComposerLifecycle } from "../app-runtime/useSessionComposerLifecycle";

function deferred() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}

for (const fails of [false, true]) {
  const oldSave = deferred(), newSave = deferred();
  let inputs = 0, teams = 0, resumes = 0;
  const lifecycle = createSessionComposerLifecycle(async () => ({
    flushInput: () => ++inputs === 1 ? oldSave.promise : newSave.promise,
    flushTeam: async () => { teams++; },
    resumeInput: () => { resumes++; }, resumeTeam: () => { resumes++; },
  }));
  const previous = lifecycle.flush();
  const previousRejected = assert.rejects(previous, /superseded|late failure/);
  await Promise.resolve();
  await lifecycle.resume();
  assert.equal(resumes, 2);
  const current = lifecycle.flush();
  await Promise.resolve();
  if (fails) oldSave.reject(new Error("late failure")); else oldSave.resolve();
  await previousRejected;
  assert.equal(teams, 0, "a cancelled quit must not freeze another draft owner later");
  assert.equal(resumes, 2, "a stale failure must not unlock the new quit's drafts");
  newSave.resolve(); await current;
  assert.equal(teams, 1);
}

// Even loading the lifecycle modules can finish out of order after a timeout.
const loading = deferred();
let resumes = 0, loads = 0, saves = 0;
const lifecycle = createSessionComposerLifecycle(async () => {
  if (++loads === 1) await loading.promise;
  return { flushInput: async () => { saves++; }, flushTeam: async () => {},
    resumeInput: () => { resumes++; }, resumeTeam: () => { resumes++; } };
});
const resume = lifecycle.resume();
await lifecycle.flush();
loading.resolve(); await resume;
assert.equal(saves, 1);
assert.equal(resumes, 0, "a delayed resume must not unlock a newer quit");
console.log("PASS composer exit generation fences late saves, failures and resumes");
