// Registers the svg-loader hook before tsx boots, so asset imports in
// component tests resolve to a stub instead of failing on the extension.
// Uses the in-thread registerHooks API: module.register() is deprecated
// (DEP0205) and its warning reaches console.error, which tests that assert
// a warning-free run would count.
import { registerHooks } from "node:module";
import { load } from "./svg-loader.mjs";
registerHooks({ load });
