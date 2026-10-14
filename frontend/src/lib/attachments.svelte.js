// Prompt attachments: every front-end (drop, paste, capture, open dialog)
// ends here. Deliberately NOT in stores.svelte.js — the public export replaces
// that file wholesale. Importing app/toast from it is fine.
import { tick } from "svelte";
import { app, toast } from "./stores.svelte.js";
import { noteInput, msSinceInput } from "./termbus.js";
import {
  InsertPaths, InsertDroppedPaths, SaveClipboardImage, CaptureScreenshot, LastScreenshot,
  ScreenshotSupport, PickAttachFiles, PasteClipboardFiles,
} from "../../wailsjs/go/main/App.js";

export const CONFIRM_MS = 3500;
export const CONFIRM_MAX_ITEMS = 6;
export const PASTE_MAX_BYTES = 20 * 1024 * 1024;
export const confirmation = $state({ items: [], session: "", at: 0 });
export const shots = $state({ supported: false });

// Never call a binding synchronously at module load: in a plain browser
// window.go is undefined, the call throws during module evaluation, and the
// whole shell fails to mount. Deferring it into a promise turns that throw
// into a rejection the catch absorbs.
Promise.resolve().then(() => ScreenshotSupport()).then((v) => { shots.supported = !!v; }).catch(() => {});

let hideTimer;

export function activeWindowID() {
  return (app.sessionKey || "").split(":")[0];
}

export function sessionName(windowID) {
  const s = (app.sessions || []).find((x) => x.windowID === windowID);
  return (s && s.name) || windowID;
}

async function focusTerminal() {
  await tick();
  const ta = document.querySelector(".pane .xterm-helper-textarea");
  if (ta) ta.focus();
}

// Inserts immediately (decision D1): no staging. The confirmation is set only
// from what Go reports as inserted, so a failed paste can never look like a
// success. noteInput() on both sides: the paste's echo comes back as pty
// output within milliseconds, and termbus would otherwise read it as the
// session working.
function clearConfirmation() {
  clearTimeout(hideTimer);
  confirmation.items = [];
}

// passthrough(e) lets a caller take over for errors that mean "not mine":
// nothing is toasted or cleared and "fallback" is returned.
async function runInsert(call, windowID, paths, allowEmpty = false, passthrough = null) {
  if (!windowID) { toast("Start or select a session first", "error"); return; }
  if (!allowEmpty && (!paths || !paths.length)) return;
  let res;
  noteInput();
  try {
    res = await call(windowID, paths);
  } catch (e) {
    if (passthrough && passthrough(e)) return "fallback";
    clearConfirmation(); // a stale "Inserted X" must not sit beside the failure
    toast(String(e), "error");
    return;
  } finally {
    noteInput();
  }
  for (const r of res.rejected || []) toast(`${r.name}: ${r.reason}`, "error");
  if (!(res.inserted || []).length) clearConfirmation();
  if ((res.inserted || []).length) {
    confirmation.items = res.inserted;
    confirmation.session = sessionName(windowID);
    confirmation.at = Date.now();
    clearTimeout(hideTimer);
    hideTimer = setTimeout(() => { confirmation.items = []; }, CONFIRM_MS);
    await focusTerminal();
  }
}

export function insertPaths(windowID, paths) { return runInsert(InsertPaths, windowID, paths); }
export function insertDropped(windowID, paths) { return runInsert(InsertDroppedPaths, windowID, paths); }

// Go has nothing to read: no native pasteboard reader on this platform, or
// nothing usable on the clipboard.
const nativeAbsent = (e) => {
  const m = String(e);
  return m === "The clipboard has no files" || m.startsWith("Pasting files isn't supported");
};

// One native pasteboard read (decisions U5/U6): Go returns real file paths
// (Finder ⌘C) or an image the pasteboard holds, converted if Claude cannot read
// its format. "fallback" means Go had nothing to give.
function pasteNative(windowID, anyFailureFallsBack = false) {
  return runInsert((w) => PasteClipboardFiles(w), windowID, [], true, anyFailureFallsBack ? () => true : nativeAbsent);
}

// Finder ⌘C: when Go cannot help, the user gets the drag-and-drop guidance.
export async function pasteFinderFiles(windowID, fallback) {
  if ((await pasteNative(windowID)) === "fallback") toast(fallback, "error");
}

// Clipboard images: the native read first, so ⌘V agrees with Claude Code's own
// Ctrl+V (HEIC included). ANY native failure (no reader, a timeout, a broken
// script) falls back to the webview's bytes, which carry the image the user
// copied; if the failure was a real refusal, such as a session waiting on a
// prompt, the insert that follows refuses with the same reason.
export async function pasteImages(windowID, files) {
  if ((await pasteNative(windowID, true)) === "fallback") await pasteImageFiles(windowID, files);
}

function toBase64(file) {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result).split(",")[1] || "");
    r.onerror = () => reject(r.error);
    r.readAsDataURL(file);
  });
}

// Saves every pasted image, then inserts them all in ONE paste, in order.
// The size check runs before FileReader so a huge image is never base64'd
// into memory just to be refused.
export async function pasteImageFiles(windowID, files) {
  const paths = [];
  for (const f of files) {
    if (f.size > PASTE_MAX_BYTES) { toast(`${f.name} is larger than 20 MB, the paste limit`, "error"); continue; }
    try { paths.push(await SaveClipboardImage(await toBase64(f), f.type)); }
    catch (e) { toast(String(e), "error"); }
  }
  if (paths.length) await insertPaths(windowID, paths);
}

export async function captureInto(windowID) {
  let path;
  try { path = await CaptureScreenshot(); } catch (e) { toast(String(e), "error"); return; }
  if (path) await insertPaths(windowID, [path]);
}

export async function lastScreenshotInto(windowID) {
  let path;
  try { path = await LastScreenshot(); } catch (e) { toast(String(e), "error"); return; }
  await insertPaths(windowID, [path]);
}

export async function attachFileInto(windowID) {
  if (!windowID) { toast("Start or select a session first", "error"); return; }
  let paths;
  try { paths = await PickAttachFiles(); } catch (e) { toast(String(e), "error"); return; }
  if (paths && paths.length) await insertPaths(windowID, paths);
}

// Test seam, same convention as window.__app.
if (typeof window !== "undefined") {
  window.__attach = { confirmation, shots, insertPaths, insertDropped, pasteImageFiles, pasteImages, pasteFinderFiles, captureInto, lastScreenshotInto, attachFileInto, msSinceInput };
}
