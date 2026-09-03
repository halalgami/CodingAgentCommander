// Frontend access to the Go-authoritative listing/render. Thin async wrappers,
// matching history.js: components never import wailsjs directly. These throw,
// so the viewer renders the failure inline instead of a blank pane — each one
// is a refusal worth reading.
import {
  ListProjectDocs, RenderProjectDoc, OpenProjectDoc, ListSessionDocs,
} from "../../wailsjs/go/main/App.js";

export async function listProjectDocs(root) { return await ListProjectDocs(root); }
export async function renderProjectDoc(root, rel) { return await RenderProjectDoc(root, rel); }
export async function openProjectDoc(root, rel) { return await OpenProjectDoc(root, rel); }
export async function listSessionDocs(windowID) { return await ListSessionDocs(windowID); }
