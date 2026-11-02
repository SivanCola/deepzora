// Registers the css-loader hook before tsx boots, so `.css` imports in
// component modules resolve to an empty module instead of failing on the
// extension. Mirrors svg-stub-register.mjs, including its registerHooks use.
import { registerHooks } from "node:module";
import { load } from "./css-loader.mjs";
registerHooks({ load });
