// Which document the viewer is showing. Deliberately decision-free: $state is
// a compiler rune, so a .svelte.js file cannot be reached by `node --test` —
// everything that DECIDES lives in docs.js.
import { app } from "./stores.svelte.js";

export const doc = $state({ root: "", rel: "" });

export function openDoc(root, rel) {
  doc.root = root;
  doc.rel = rel;
  app.drawer = "docview";
}

// Which folder the index is showing, and on whose behalf. windowID is "" for a
// project-wide listing; sinceOnly is meaningless without one.
export const docsCtx = $state({ root: "", windowID: "", sinceOnly: false });

export function openDocsList(root, windowID, sinceOnly) {
  docsCtx.root = root;
  docsCtx.windowID = windowID ?? "";
  docsCtx.sinceOnly = !!sinceOnly && !!windowID;
  app.drawer = "docs";
}
