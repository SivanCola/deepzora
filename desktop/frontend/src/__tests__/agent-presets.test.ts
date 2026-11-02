import assert from "node:assert/strict";
import { AGENT_PRESET_ID_PATTERN, agentPresetTools, nextAgentPresetCopyID, suggestedAgentPresetID, renderAgentPreset, type AgentPreset } from "../lib/agentPresets";
import { makeMockAgentPresetBindings } from "../lib/agentPresetMock";
import type { TabMeta } from "../lib/types";
import { isPresetIconImage, presetIconCrop } from "../lib/agentPresetImage";

const special = { name: 'Reviewer: "safe"\n---', description: "true # comment\n&alias", body: "# Review\n\n---\nCheck everything.", keepCoding: false, complete: true, includeRuntimeContext: false, tools: ["read_*", "mcp:research"] };
const content = renderAgentPreset(special);
assert.match(content, /name: "Reviewer: \\"safe\\"\\n---"/u);
assert.match(content, /keep-coding-instructions: false/u);
assert.match(content, /^complete: true$/mu);
assert.match(content, /^include-runtime-context: false$/mu);
assert.doesNotMatch(renderAgentPreset({ ...special, complete: false, includeRuntimeContext: true }), /^(?:complete|include-runtime-context):/mu, "default prompt options are omitted");
assert.equal(content.split("\n").filter(line => line === "---").length, 3, "persona fences stay in the body");
assert.doesNotMatch(renderAgentPreset({ ...special, tools: null }), /^tools:/mu, "null inherits the available surface");
assert.match(renderAgentPreset({ ...special, tools: [] }), /^tools: \[\]$/mu, "an explicit empty surface remains empty");
assert.deepEqual(agentPresetTools("read_file, grep\nread_file\r\nmcp:research, \n"), ["read_file", "grep", "mcp:research"]);
assert.equal(nextAgentPresetCopyID("review", [{ id: "review-copy" }, { id: "review-copy-2" }] as AgentPreset[]), "review-copy-3");
assert.equal(suggestedAgentPresetID("Code Reviewer", "user", []), "code-reviewer");
assert.equal(suggestedAgentPresetID("Café & Docs", "user", []), "cafe-docs");
assert.equal(suggestedAgentPresetID("default", "user", []), "default-2");
assert.equal(suggestedAgentPresetID("Review", "user", [{ id: "review", scope: "user" }, { id: "review-2", scope: "user" }] as AgentPreset[]), "review-3");
assert.equal(suggestedAgentPresetID("Review", "project", [{ id: "review", scope: "user" }] as AgentPreset[]), "review", "project override remains possible");
assert.equal(suggestedAgentPresetID("  ", "user", []), "");
for (const name of ["代码审查员", "🎨", "a".repeat(100)]) {
  const generated = suggestedAgentPresetID(name, "user", []);
  assert.match(generated, AGENT_PRESET_ID_PATTERN);
  assert.equal(suggestedAgentPresetID(name, "user", []), generated, "non-Latin fallback is deterministic");
  assert.match(suggestedAgentPresetID(name, "user", [{ id: generated, scope: "user" }] as AgentPreset[]), AGENT_PRESET_ID_PATTERN);
}

let tabs = [{ id: "a", active: true, workspaceRoot: "/project-a", topicId: "blank", agentPreset: "default" },
  { id: "b", workspaceRoot: "/project-b", topicId: "blank" }] as TabMeta[];
const changed: string[] = [];
const mock = makeMockAgentPresetBindings({ tabs: () => tabs, update: tab => { tabs = tabs.map(item => item.id === tab.id ? tab : item); }, isBlank: id => id === "blank", changed: async id => { changed.push(id); } });
const image = "data:image/png;base64,iVBORw0KGgo=";
const withImage = await mock.SaveAgentPreset("a", "user", "avatar", renderAgentPreset({ ...special, icon: "search", iconImage: image, iconColor: "#60a5fa" }));
assert.equal(withImage.iconColor, "#60a5fa");
await assert.rejects(mock.SaveAgentPreset("a", "user", "bad-color", renderAgentPreset({ ...special, iconColor: "url(https://example.com/)" })), /Invalid icon color/u);
assert.equal(withImage.iconImage, image);
assert.equal(withImage.icon, "search", "mock and backend preserve glyph fallback alongside custom images");
const imageCopy = await mock.SaveAgentPreset("a", "user", "avatar-copy", renderAgentPreset(withImage));
await mock.DeleteAgentPreset("a", "user", "avatar");
assert.equal(imageCopy.iconImage, image, "copies embed their own image");
assert.equal(isPresetIconImage("https://example.com/avatar.png"), false);
assert.equal(isPresetIconImage("data:image/svg+xml;base64,PHN2Zy8+"), false);
assert.equal(isPresetIconImage("data:image/png;base64," + "A".repeat(128 * 1024)), false);
assert.deepEqual(presetIconCrop(400, 200, 1, 50, 50), { side: 200, left: 100, top: 0 });
assert.deepEqual(presetIconCrop(400, 200, 2, 100, 100), { side: 100, left: 300, top: 100 });
const saved = await mock.SaveAgentPreset("a", "user", "review", content);
assert.deepEqual({ name: saved.name, description: saved.description, body: saved.body, keepCoding: saved.keepCoding, complete: saved.complete, includeRuntimeContext: saved.includeRuntimeContext, tools: saved.tools }, special, "canonical YAML preserves prompt options, special scalars and persona body");
const legacy = await mock.SaveAgentPreset("a", "user", "legacy", '---\nname: "Legacy"\n---\n');
assert.equal(legacy.complete, false, "older preset files default to an assembled system prompt");
assert.equal(legacy.includeRuntimeContext, true, "older preset files include runtime context");
assert.equal(legacy.tools, null, "omitted tools inherit the available surface");
const defaultPreset = (await mock.ListAgentPresets("a")).presets.find(p => p.id === "default")!;
assert.equal(defaultPreset.complete, false);
assert.equal(defaultPreset.includeRuntimeContext, true);
for (const complete of [false, true]) {
  for (const includeRuntimeContext of [false, true]) {
    const preset = { ...special, complete, includeRuntimeContext, keepCoding: true, tools: null };
    const roundTrip = await mock.SaveAgentPreset("a", "user", "prompt-options", renderAgentPreset(preset));
    assert.deepEqual([roundTrip.complete, roundTrip.includeRuntimeContext, roundTrip.keepCoding, roundTrip.tools], [complete, includeRuntimeContext, true, null], "prompt options vary independently and preserve keepCoding and inherited tools");
  }
}
for (const field of ["complete", "include-runtime-context", "keep-coding-instructions"]) {
  for (const value of ['"false"', "0", "null", "[]"]) {
    await assert.rejects(mock.SaveAgentPreset("a", "user", "invalid", `---\n${field}: ${value}\n---\nBody\n`), /must be a boolean/u);
  }
}
await assert.rejects(mock.SaveAgentPreset("a", "user", "invalid", '---\ncomplete: true\ncomplete: false\n---\nBody\n'), /Duplicate field/u);
await assert.rejects(mock.SaveAgentPreset("a", "user", "invalid", '---\nunknown: true\n---\nBody\n'), /Unknown field/u);
await assert.rejects(mock.SaveAgentPreset("a", "user", "invalid", '---\ntools: [1]\n---\nBody\n'), /list of strings/u);
await assert.rejects(mock.SaveAgentPreset("a", "user", "empty-complete", renderAgentPreset({ ...special, keepCoding: true, body: "  \n " })), /body is required/u);
await mock.SaveAgentPreset("a", "project", "review", renderAgentPreset({ ...special, name: "Project review", tools: [] }));
assert.equal((await mock.ListAgentPresets("a")).presets.find(p => p.id === "review")?.scope, "project", "project overrides user definition");
assert.equal((await mock.ListAgentPresets("b")).presets.find(p => p.id === "review")?.scope, "user", "project definitions stay in their workspace");
assert.deepEqual((await mock.ListAgentPresets("a")).presets.find(p => p.id === "review")?.tools, []);
await mock.SetAgentPresetForTab("a", "review");
assert.equal((await mock.ListAgentPresets("a")).current, "review");
assert.deepEqual(changed, ["a"]);
const detached = await mock.ListAgentPresets("a");
detached.presets.find(p => p.id === "review")!.tools!.push("bash");
assert.deepEqual((await mock.ListAgentPresets("a")).presets.find(p => p.id === "review")?.tools, [], "views do not mutate saved definitions");
await mock.DeleteAgentPreset("a", "project", "review");
assert.equal((await mock.ListAgentPresets("a")).presets.find(p => p.id === "review")?.scope, "user", "deleting override reveals user preset");
await assert.rejects(mock.SaveAgentPreset("a", "builtin", "default", content));
await assert.rejects(mock.DeleteAgentPreset("a", "builtin", "default"));
await assert.rejects(mock.SaveAgentPreset("a", "user", "../escape", content));
await assert.rejects(mock.SaveAgentPreset("a", "user", "empty", renderAgentPreset({ ...special, body: "" })));
tabs = tabs.map(tab => tab.id === "a" ? { ...tab, agentPresetLocked: true } : tab);
await assert.rejects(mock.SetAgentPresetForTab("a", "default"), /locked/u);
assert.equal((await mock.ListAgentPresets("a")).current, "review", "a locked selection is retained");
console.log("Agent presets: prompt option defaults and strict validation, canonical escaping, null/empty tools, scoped CRUD, copy IDs and locked selection passed.");
