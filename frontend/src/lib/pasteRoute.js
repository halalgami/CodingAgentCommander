import { pasteImages, pasteFinderFiles, activeWindowID } from "./attachments.svelte.js";
import { toast } from "./stores.svelte.js";

// Decide what a paste is, from the event alone. Claimed ONLY when focus is
// inside the terminal (.xterm) and the clipboard carries files. Drawer
// inputs, the folder field and the palette paste exactly as before — focus
// routing is the bug class that once shipped past 99 green specs.
//
// Text wins (decision D8): Excel/Numbers/Word put real text AND a picture on
// the clipboard, and the user wants the text.
//
// A FILE COPY (user decision U5) is Finder ⌘C: text/plain is just the file
// name(s), or there is no text and the files are not all images. The webview
// withholds the real path, so Go reads it from the pasteboard; the verdict
// "file-copy" carries the guidance used if Go has nothing to offer. This is
// decided BEFORE the image check: a Finder-copied PNG is a file copy whose
// bytes are only a preview, and the original's path is what the user wants.
// Pure images with no text (screenshot, Copy Image) are saved as before.
export function classifyPaste(activeEl, text, files) {
  const list = files ? [...files] : [];
  if (!list.length) return { verdict: "ignore" };
  if (!activeEl || !activeEl.closest || !activeEl.closest(".xterm")) return { verdict: "ignore" };
  const t = (text || "").trim();
  if (t) {
    const names = new Set(list.map((f) => f.name));
    const lines = t.split(/\r\n|\r|\n/).map((l) => l.trim()).filter(Boolean);
    if (!lines.every((l) => names.has(l))) return { verdict: "ignore" };
    return { verdict: "file-copy", name: list[0].name, fallback: `Copied files cannot be pasted. To attach ${list[0].name}, drag it onto the session instead.` };
  }
  // Every image/* is claimed, so Go names an unsupported format (HEIC, TIFF)
  // instead of the paste silently becoming nothing.
  const images = list.filter((f) => (f.type || "").startsWith("image/"));
  if (images.length !== list.length) {
    return { verdict: "file-copy", name: list[0].name, fallback: `Only images can be pasted. To attach ${list[0].name}, drag it onto the session instead.` };
  }
  return { verdict: "images", images };
}

export function onPaste(e) {
  const dt = e.clipboardData;
  const c = classifyPaste(document.activeElement, dt ? dt.getData("text/plain") : "", dt ? dt.files : null);
  if (c.verdict === "ignore") return;
  e.preventDefault();
  e.stopPropagation();
  if (c.verdict === "file-copy") {
    pasteFinderFiles(activeWindowID(), c.fallback);
    return;
  }
  pasteImages(activeWindowID(), c.images);
}
