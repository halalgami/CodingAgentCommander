import { test, expect } from "@playwright/test";

// Go's real strings (Task 4/Task 1) — the stubs below throw or return exactly
// what the bindings would.
const GO_INSERT_FAIL = "could not insert into W1: tmux paste-buffer: can't find window: @1: exit status 1";
const CONTROL_REASON = "the name contains a line break or an invisible character";

// boot mounts the real shell with Go stubbed. Base stubs record every insert;
// `stubs` (a self-contained function, serialized by Playwright) runs after
// them to override per test. Sessions use tmux-shaped window ids. ListSessions
// is deliberately NOT stubbed: refresh() swallows binding errors
// (stores.svelte.js refreshOnce), so the seeded sessions survive a select().
async function boot(page, { sessions = [["@1", "W1"]], active = "@1", stubs = null } = {}) {
  await page.addInitScript(() => {
    window.go = window.go || {}; window.go.main = window.go.main || {};
    window.__inserted = []; window.__dropped = []; window.__saved = []; window.__selected = [];
    const hid = (i) => i.toString(16).padStart(24, "0");
    // The shape InsertPaths returns (attachments.go): basename name, image
    // kind by extension, thumb only for images, 24-hex ids.
    window.__asResult = (paths) => ({
      inserted: paths.map((p, i) => {
        const name = p.split("/").pop();
        const img = /\.(png|jpe?g|gif|webp)$/i.test(name);
        return { id: hid(i + 1), path: p, name, kind: img ? "image" : "file", thumb: img ? "/media/attach/" + hid(i + 1) : "" };
      }),
      rejected: [],
    });
    window.go.main.App = Object.assign(window.go.main.App || {}, {
      ScreenshotSupport: async () => false,
      SelectSession: async (id) => { window.__selected.push(id); return null; },
      InsertPaths: async (win, paths) => { window.__inserted.push([win, paths]); return window.__asResult(paths); },
      InsertDroppedPaths: async (win, paths) => { window.__dropped.push([win, paths]); return window.__asResult(paths); },
    });
  });
  if (stubs) await page.addInitScript(stubs);
  await page.goto("/?nointro");
  await page.evaluate(([sessions, active]) => {
    window.__app.sessions = sessions.map(([windowID, name]) => ({ windowID, name }));
    window.__app.sessionKey = active ? active + ":1" : "";
    // select() shows the terminal; seeding the key directly must do the same,
    // or the dashboard keeps the terminal layer inert.
    if (active) window.__app.view = "terminal";
  }, [sessions, active]);
  if (active) await expect(page.locator(".xterm")).toBeVisible();
}

async function centre(locator) {
  const b = await locator.boundingBox();
  return [Math.round(b.x + b.width / 2), Math.round(b.y + b.height / 2)];
}

test("the shell mounts with no Go bindings at all", async ({ page }) => {
  await page.goto("/?nointro"); // no stubs: window.go is undefined
  await expect(page.getByTestId("titlebar")).toBeVisible();
  await expect(page.getByTestId("wordmark")).toHaveText("COMMANDER");
});

test("positive control: a failed insert toasts Go's error and shows no confirmation", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.InsertPaths = async () => { throw "could not insert into W1: tmux paste-buffer: can't find window: @1: exit status 1"; };
  } });
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/spec.pdf"]));
  await expect(page.getByTestId("toast")).toHaveText(GO_INSERT_FAIL);
  await expect(page.getByTestId("attach-confirm")).toHaveCount(0);
});

test("inserting shows a confirmation naming the files and the session", async ({ page }) => {
  await boot(page);
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/shot.png", "/Users/me/proj/spec.pdf"]));
  expect(await page.evaluate(() => window.__inserted)).toEqual([["@1", ["/Users/me/proj/shot.png", "/Users/me/proj/spec.pdf"]]]);
  const c = page.getByTestId("attach-confirm");
  await expect(c.getByTestId("attach-confirm-title")).toHaveText("Inserted 2 paths into W1");
  await expect(c.getByTestId("attach-name")).toHaveText(["shot.png", "spec.pdf"]);
  await expect(c.locator('bdi[data-testid="attach-name"]')).toHaveCount(2); // names isolated from bidi reordering
  await expect(c.locator("img")).toHaveAttribute("src", "/media/attach/000000000000000000000001");
});

test("a single path reads in the singular", async ({ page }) => {
  await boot(page);
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/spec.pdf"]));
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted spec.pdf into W1");
});

test("the confirmation goes away by itself", async ({ page }) => {
  await boot(page);
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/spec.pdf"]));
  await expect(page.getByTestId("attach-confirm")).toBeVisible();
  await expect(page.getByTestId("attach-confirm")).toHaveCount(0, { timeout: 6000 });
});

test("rejections are reported by name and reason, alongside what did go in", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.InsertPaths = async () => ({
      inserted: [{ id: "00000000000000000000000a", path: "/Users/me/proj/ok.pdf", name: "ok.pdf", kind: "file", thumb: "" }],
      rejected: [{ name: "evil?name.pdf", reason: "the name contains a line break or an invisible character" }],
    });
  } });
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/ok.pdf", "/Users/me/proj/evil\u2028name.pdf"]));
  await expect(page.getByTestId("toast")).toHaveText("evil?name.pdf: " + CONTROL_REASON);
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted ok.pdf into W1");
});

test("the confirmation lists at most six items, then +N more", async ({ page }) => {
  await boot(page);
  await page.evaluate(() => window.__attach.insertPaths("@1", Array.from({ length: 8 }, (_, i) => `/Users/me/proj/f${i}.pdf`)));
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted 8 paths into W1");
  await expect(page.getByTestId("attach-name")).toHaveText(["f0.pdf", "f1.pdf", "f2.pdf", "f3.pdf", "f4.pdf", "f5.pdf"]);
  await expect(page.getByTestId("attach-more")).toHaveText("+2 more");
});

test("after an insert, focus is in the terminal and the paste counted as input", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.InsertPaths = async (win, paths) => {
      window.__sinceInputAtCall = window.__attach.msSinceInput();
      return window.__asResult(paths);
    };
  } });
  await page.evaluate(() => document.activeElement && document.activeElement.blur());
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/spec.pdf"]));
  // Infinity until something calls noteInput(); < 1s proves it ran BEFORE the call.
  expect(await page.evaluate(() => window.__sinceInputAtCall)).toBeLessThan(1000);
  expect(await page.evaluate(() => document.activeElement.classList.contains("xterm-helper-textarea"))).toBe(true);
});

test("the paste's echo after a slow insert still counts as input (trailing noteInput)", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.InsertPaths = async (win, paths) => {
      await new Promise((r) => setTimeout(r, 1200));
      return window.__asResult(paths);
    };
  } });
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/spec.pdf"]));
  expect(await page.evaluate(() => window.__attach.msSinceInput())).toBeLessThan(500);
});

test("an all-rejected insert clears the previous confirmation at once", async ({ page }) => {
  await boot(page);
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/spec.pdf"]));
  await expect(page.getByTestId("attach-confirm")).toBeVisible(); // positive control
  await page.evaluate(() => {
    window.go.main.App.InsertPaths = async () => ({ inserted: [], rejected: [{ name: "evil.pdf", reason: "the name contains a line break or an invisible character" }] });
  });
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/evil.pdf"]));
  await expect(page.getByTestId("toast")).toHaveText("evil.pdf: " + CONTROL_REASON);
  await expect(page.getByTestId("attach-confirm")).toHaveCount(0, { timeout: 500 });
});

test("a thrown insert clears the previous confirmation at once", async ({ page }) => {
  await boot(page);
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/spec.pdf"]));
  await expect(page.getByTestId("attach-confirm")).toBeVisible();
  await page.evaluate(() => { window.go.main.App.InsertPaths = async () => { throw "could not insert into W1: boom"; }; });
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/proj/x.pdf"]));
  await expect(page.getByTestId("attach-confirm")).toHaveCount(0, { timeout: 500 });
});

test("with no active session nothing is inserted and the user is told why", async ({ page }) => {
  await boot(page, { active: "" });
  await page.evaluate(() => window.__attach.insertPaths("", ["/Users/me/proj/spec.pdf"]));
  await expect(page.getByTestId("toast")).toHaveText("Start or select a session first");
  expect(await page.evaluate(() => window.__inserted)).toEqual([]);
});

// The real Wails runtime calls window.runtime.OnFileDrop(cb, useDropTarget)
// and invokes cb(x, y, paths) with the paths split on "\n" (see
// WailsWebView.m + dispatcher/draganddrop.go). This stub captures cb so a
// spec can deliver exactly those shapes.
const dropStubs = () => {
  window.runtime = Object.assign(window.runtime || {}, {
    OnFileDrop: (cb, useDropTarget) => { window.__drop = cb; window.__dropUsesTarget = useDropTarget; },
  });
};

async function bootDrop(page, opts = {}) {
  await page.addInitScript(dropStubs);
  await boot(page, { sessions: [["@1", "W1"], ["@2", "W2"]], ...opts });
  await expect.poll(() => page.evaluate(() => typeof window.__drop)).toBe("function");
}

const BROKEN_DROP = "A dropped name contains a line break or was moved — nothing was inserted.";

test("a drop on the terminal inserts into the active session and focuses it", async ({ page }) => {
  await bootDrop(page);
  const [x, y] = await centre(page.locator(".xterm"));
  await page.evaluate(([x, y]) => window.__drop(x, y, ["/Users/me/proj/spec.pdf"]), [x, y]);
  await expect.poll(() => page.evaluate(() => window.__dropped)).toEqual([["@1", ["/Users/me/proj/spec.pdf"]]]);
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted spec.pdf into W1");
  expect(await page.evaluate(() => document.activeElement.classList.contains("xterm-helper-textarea"))).toBe(true);
});

test("a drop on a background card inserts into THAT session and selects it", async ({ page }) => {
  await bootDrop(page);
  const [x, y] = await centre(page.locator('[data-window-id="@2"]'));
  await page.evaluate(([x, y]) => window.__drop(x, y, ["/Users/me/other/a.pdf", "/Users/me/other/b.pdf"]), [x, y]);
  await expect.poll(() => page.evaluate(() => window.__dropped)).toEqual([["@2", ["/Users/me/other/a.pdf", "/Users/me/other/b.pdf"]]]);
  expect(await page.evaluate(() => window.__selected)).toEqual(["@2"]);
  await expect.poll(() => page.evaluate(() => window.__app.sessionKey.split(":")[0])).toBe("@2");
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted 2 paths into W2");
  await expect(page.getByTestId("toast")).toHaveCount(0); // positive control: Task 7's failed-insert spec
});

test("a newline-split drop is passed to Go untouched and refused whole", async ({ page }) => {
  await bootDrop(page, { stubs: () => {
    window.go.main.App.InsertDroppedPaths = async (win, paths) => {
      window.__dropped.push([win, paths]);
      throw "A dropped name contains a line break or was moved — nothing was inserted.";
    };
  } });
  const [x, y] = await centre(page.locator(".xterm"));
  // What Wails delivers for "/p/ok.pdf" plus "/p/evil<LF>name.pdf".
  await page.evaluate(([x, y]) => window.__drop(x, y, ["/p/ok.pdf", "/p/evil", "name.pdf"]), [x, y]);
  await expect.poll(() => page.evaluate(() => window.__dropped)).toEqual([["@1", ["/p/ok.pdf", "/p/evil", "name.pdf"]]]);
  await expect(page.getByTestId("toast")).toHaveText(BROKEN_DROP);
  await expect(page.getByTestId("attach-confirm")).toHaveCount(0);
});

test("a link or text drag (Wails delivers [\"\"]) is refused without a crash", async ({ page }) => {
  await bootDrop(page, { stubs: () => {
    window.go.main.App.InsertDroppedPaths = async (win, paths) => {
      window.__dropped.push([win, paths]);
      throw "Only files can be dropped onto a session.";
    };
  } });
  const [x, y] = await centre(page.locator(".xterm"));
  await page.evaluate(([x, y]) => window.__drop(x, y, [""]), [x, y]);
  await expect.poll(() => page.evaluate(() => window.__dropped)).toEqual([["@1", [""]]]);
  await expect(page.getByTestId("toast")).toHaveText("Only files can be dropped onto a session.");
});

test("a drop outside the pane and the cards is refused with a reason", async ({ page }) => {
  await bootDrop(page);
  const bar = await page.getByTestId("wordmark").boundingBox();
  await page.evaluate(([x, y]) => window.__drop(x, y, ["/Users/me/x.pdf"]), [Math.round(bar.x + 4), Math.round(bar.y + bar.height / 2)]);
  await expect(page.getByTestId("toast")).toHaveText("Drop onto the terminal or a session card");
  expect(await page.evaluate(() => window.__dropped)).toEqual([]);
});

test("a drop with no active session asks for one", async ({ page }) => {
  await bootDrop(page, { active: "" });
  const [x, y] = await centre(page.locator(".pane"));
  await page.evaluate(([x, y]) => window.__drop(x, y, ["/Users/me/x.pdf"]), [x, y]);
  await expect(page.getByTestId("toast")).toHaveText("Start or select a session first");
  expect(await page.evaluate(() => window.__dropped)).toEqual([]);
});

test("a drop clears the dragover highlight Wails could not clear", async ({ page }) => {
  await bootDrop(page);
  await page.evaluate(() => {
    document.querySelector(".pane").classList.add("wails-drop-target-active");
    document.querySelector('[data-window-id="@2"]').classList.add("wails-drop-target-active");
  });
  const [x, y] = await centre(page.locator(".xterm"));
  await page.evaluate(([x, y]) => window.__drop(x, y, ["/Users/me/proj/spec.pdf"]), [x, y]);
  expect(await page.evaluate(() => document.getElementsByClassName("wails-drop-target-active").length)).toBe(0);
});

test("every element is a Wails drop target so handleDrop always decides", async ({ page }) => {
  await bootDrop(page);
  expect(await page.evaluate(() => window.__dropUsesTarget)).toBe(true);
  for (const sel of ["html", ".pane", '[data-window-id="@1"]', '[data-testid="titlebar"]']) {
    const v = await page.locator(sel).first().evaluate((el) => getComputedStyle(el).getPropertyValue("--wails-drop-target").trim());
    expect(v, sel).toBe("drop");
  }
});

test("after a background-card drop the remounted terminal has focus", async ({ page }) => {
  await bootDrop(page);
  await page.evaluate(() => document.activeElement && document.activeElement.blur());
  const [x, y] = await centre(page.locator('[data-window-id="@2"]'));
  await page.evaluate(([x, y]) => window.__drop(x, y, ["/Users/me/other/a.pdf"]), [x, y]);
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted a.pdf into W2");
  expect(await page.evaluate(() => document.activeElement.classList.contains("xterm-helper-textarea"))).toBe(true);
});

test("if selecting the background card fails, nothing is inserted into the unselected window", async ({ page }) => {
  await bootDrop(page, { stubs: () => {
    window.go.main.App.SelectSession = async () => { throw "no such session"; };
  } });
  const [x, y] = await centre(page.locator('[data-window-id="@2"]'));
  await page.evaluate(([x, y]) => window.__drop(x, y, ["/Users/me/other/a.pdf"]), [x, y]);
  await expect(page.getByTestId("toast")).toHaveText("no such session");
  await page.waitForTimeout(200);
  expect(await page.evaluate(() => window.__dropped)).toEqual([]);
  expect(await page.evaluate(() => window.__app.sessionKey.split(":")[0])).toBe("@1");
});

// ---- Task 9: clipboard image paste ----
// A REAL ClipboardEvent carrying Files, dispatched on whatever element a real
// click focused. A file is { bytes, name, type } or, for a big one,
// { size, name, type } synthesized in the page.
async function pasteInto(page, { text = "", files = [] }) {
  return page.evaluate(({ text, files }) => {
    const dt = new DataTransfer();
    if (text) dt.setData("text/plain", text);
    for (const f of files) {
      const body = f.size ? new Uint8Array(f.size) : new Uint8Array(f.bytes);
      dt.items.add(new File([body], f.name, { type: f.type }));
    }
    const ev = new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true });
    document.activeElement.dispatchEvent(ev);
    return ev.defaultPrevented;
  }, { text, files });
}

const pasteStubs = () => {
  let n = 0;
  window.go.main.App.SaveClipboardImage = async (b64, mime) => {
    window.__saved.push([b64, mime]);
    if (mime === "image/heic") throw "image/heic images are not supported — use PNG, JPEG, GIF or WebP";
    n += 1;
    return `/Users/me/.config/commander/attachments/paste-20261004-120000-0000000${n}.png`;
  };
  // Go reads the real paths off the pasteboard (decision U5). window.__clipPaths
  // is what the native read returns; unset means "no files on the clipboard".
  window.__clipCalls = [];
  window.go.main.App.PasteClipboardFiles = async (win) => {
    window.__clipCalls.push(win);
    if (!window.__clipPaths) throw "The clipboard has no files";
    return window.__asResult(window.__clipPaths);
  };
};

const PNG = { bytes: [137, 80, 78, 71], name: "image.png", type: "image/png" };
const SAVED_1 = "/Users/me/.config/commander/attachments/paste-20261004-120000-00000001.png";
const SAVED_2 = "/Users/me/.config/commander/attachments/paste-20261004-120000-00000002.png";

test("an image pasted into the terminal is saved and inserted, not typed", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { files: [PNG] })).toBe(true);
  await expect.poll(() => page.evaluate(() => window.__inserted)).toEqual([["@1", [SAVED_1]]]);
  expect(await page.evaluate(() => window.__saved)).toEqual([["iVBORw==", "image/png"]]);
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted paste-20261004-120000-00000001.png into W1");
});

test("two images in one paste are saved, then inserted in ONE call, in order", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { files: [PNG, { bytes: [255, 216, 255], name: "b.jpg", type: "image/jpeg" }] })).toBe(true);
  await expect.poll(() => page.evaluate(() => window.__inserted)).toEqual([["@1", [SAVED_1, SAVED_2]]]);
  expect(await page.evaluate(() => window.__saved.map((s) => s[1]))).toEqual(["image/png", "image/jpeg"]);
});

test("a HEIC paste is claimed and Go's refusal names the format", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { files: [{ bytes: [0, 0, 0, 24], name: "IMG_0001.heic", type: "image/heic" }] })).toBe(true);
  await expect(page.getByTestId("toast")).toHaveText("image/heic images are not supported — use PNG, JPEG, GIF or WebP");
  expect(await page.evaluate(() => [window.__saved.map((s) => s[1]), window.__inserted])).toEqual([["image/heic"], []]);
});

test("an image over 20 MB is refused before it is read", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { files: [{ size: 20 * 1024 * 1024 + 1, name: "big.png", type: "image/png" }] })).toBe(true);
  await expect(page.getByTestId("toast")).toHaveText("big.png is larger than 20 MB, the paste limit");
  expect(await page.evaluate(() => [window.__saved, window.__inserted])).toEqual([[], []]);
});

test("a paste into a drawer input is left alone even if it carries an image", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.getByTestId("open-models").click();
  await page.getByTestId("add-model-id").click();
  expect(await pasteInto(page, { text: "hello", files: [PNG] })).toBe(false);
  await page.waitForTimeout(150);
  expect(await page.evaluate(() => [window.__saved, window.__inserted])).toEqual([[], []]);
});

test("an image-only paste into a drawer input is left alone (focus routing, not text-wins)", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.getByTestId("open-models").click();
  await page.getByTestId("add-model-id").click();
  expect(await pasteInto(page, { files: [PNG] })).toBe(false);
  await page.waitForTimeout(150);
  expect(await page.evaluate(() => [window.__saved, window.__inserted])).toEqual([[], []]);
});

test("a text-only paste into the terminal is not claimed", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { text: "plain words" })).toBe(false);
  expect(await page.evaluate(() => window.__saved)).toEqual([]);
});

test("a spreadsheet copy (text plus image) pastes as text", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { text: "Q1\t1200\nQ2\t1350", files: [PNG] })).toBe(false);
  await page.waitForTimeout(150);
  expect(await page.evaluate(() => [window.__saved, window.__inserted])).toEqual([[], []]);
});

test("a Finder-copied PDF (name as text) inserts the ORIGINAL path via the native read", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.evaluate(() => { window.__clipPaths = ["/Users/me/Documents/my report.pdf"]; });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { text: "my report.pdf", files: [{ bytes: [37, 80, 68, 70], name: "my report.pdf", type: "application/pdf" }] })).toBe(true);
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted my report.pdf into W1");
  expect(await page.evaluate(() => [window.__clipCalls, window.__saved, window.__inserted])).toEqual([["@1"], [], []]);
});

test("a Finder-copied PNG inserts the original's path and does NOT save its preview bytes", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.evaluate(() => { window.__clipPaths = ["/Users/me/Pictures/report.png"]; });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { text: "report.png", files: [{ bytes: [137, 80, 78, 71], name: "report.png", type: "image/png" }] })).toBe(true);
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted report.png into W1");
  expect(await page.evaluate(() => [window.__clipCalls, window.__saved])).toEqual([["@1"], []]);
});

test("several Finder-copied files insert in one go", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.evaluate(() => { window.__clipPaths = ["/Users/me/a.pdf", "/Users/me/b.pdf"]; });
  await page.locator(".xterm").click();
  const f = (name) => ({ bytes: [37], name, type: "application/pdf" });
  expect(await pasteInto(page, { text: "a.pdf\nb.pdf", files: [f("a.pdf"), f("b.pdf")] })).toBe(true);
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted 2 paths into W1");
});

test("a non-image file with no text is a file copy too", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.evaluate(() => { window.__clipPaths = ["/Users/me/report.pdf"]; });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { files: [{ bytes: [37, 80, 68, 70], name: "report.pdf", type: "application/pdf" }] })).toBe(true);
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted report.pdf into W1");
});

test("a Finder copy Go cannot read falls back to the drag-and-drop guidance", async ({ page }) => {
  await boot(page, { stubs: pasteStubs }); // __clipPaths unset: Go says "The clipboard has no files"
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { text: "report.png", files: [{ bytes: [137, 80, 78, 71], name: "report.png", type: "image/png" }] })).toBe(true);
  await expect(page.getByTestId("toast")).toHaveText("Copied files cannot be pasted. To attach report.png, drag it onto the session instead.");
  expect(await page.evaluate(() => [window.__saved, window.__inserted])).toEqual([[], []]);
});

test("an unsupported platform falls back to the guidance for a bare non-image file", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.PasteClipboardFiles = async () => { throw "Pasting files isn't supported here — drag them onto the session instead"; };
  } });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { files: [{ bytes: [37, 80, 68, 70], name: "report.pdf", type: "application/pdf" }] })).toBe(true);
  await expect(page.getByTestId("toast")).toHaveText("Only images can be pasted. To attach report.pdf, drag it onto the session instead.");
});

test("any other native-read failure is shown as Go said it", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.PasteClipboardFiles = async () => { throw "W1 is waiting for you to answer a prompt — answer it, then try again"; };
  } });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { text: "a.pdf", files: [{ bytes: [37], name: "a.pdf", type: "application/pdf" }] })).toBe(true);
  await expect(page.getByTestId("toast")).toHaveText("W1 is waiting for you to answer a prompt — answer it, then try again");
});

test("a Finder copy pasted into a drawer input is left alone (name pastes as text)", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.evaluate(() => { window.__clipPaths = ["/Users/me/report.png"]; });
  await page.getByTestId("open-models").click();
  await page.getByTestId("add-model-id").click();
  expect(await pasteInto(page, { text: "report.png", files: [{ bytes: [137, 80, 78, 71], name: "report.png", type: "image/png" }] })).toBe(false);
  await page.waitForTimeout(150);
  expect(await page.evaluate(() => [window.__clipCalls, window.__saved, window.__inserted])).toEqual([[], [], []]);
});

test("macOS-style (native reader present): an image paste uses the native read, not the webview bytes", async ({ page }) => {
  await boot(page, { stubs: pasteStubs });
  await page.evaluate(() => { window.__clipPaths = ["/Users/me/.config/commander/attachments/convert-20261005-120000.000.jpg"]; });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { files: [{ bytes: [0, 0, 0, 24], name: "IMG_0001.heic", type: "image/heic" }] })).toBe(true);
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted convert-20261005-120000.000.jpg into W1");
  expect(await page.evaluate(() => [window.__clipCalls, window.__saved])).toEqual([["@1"], []]);
});

test("no native reader (Windows/Linux): an image paste falls back to the webview bytes", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.PasteClipboardFiles = async () => { throw "Pasting files isn't supported here — drag them onto the session instead"; };
    window.go.main.App.SaveClipboardImage = async (b64, mime) => { window.__saved.push([b64, mime]); return "/Users/me/.config/commander/attachments/paste-20261005-120000-00000001.png"; };
  } });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { files: [PNG] })).toBe(true);
  await expect.poll(() => page.evaluate(() => window.__inserted)).toEqual([["@1", ["/Users/me/.config/commander/attachments/paste-20261005-120000-00000001.png"]]]);
  expect(await page.evaluate(() => window.__saved)).toEqual([["iVBORw==", "image/png"]]);
  await expect(page.getByTestId("toast")).toHaveCount(0);
});

test("a native image failure still falls back to the webview bytes, which then refuse for the same reason", async ({ page }) => {
  await boot(page, { stubs: () => {
    const why = "W1 is waiting for you to answer a prompt — answer it, then try again";
    window.go.main.App.PasteClipboardFiles = async () => { throw why; };
    window.go.main.App.SaveClipboardImage = async (b64, mime) => { window.__saved.push([b64, mime]); return "/Users/me/.config/commander/attachments/paste-20261005-120000-00000001.png"; };
    window.go.main.App.InsertPaths = async () => { throw why; };
  } });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { files: [PNG] })).toBe(true);
  await expect(page.getByTestId("toast")).toHaveText("W1 is waiting for you to answer a prompt — answer it, then try again");
  expect(await page.evaluate(() => window.__saved.length)).toBe(1);
});

test("a native read that fails oddly (timeout) still pastes a plain image via the webview bytes", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.PasteClipboardFiles = async () => { throw "signal: killed"; };
    window.go.main.App.SaveClipboardImage = async (b64, mime) => { window.__saved.push([b64, mime]); return "/Users/me/.config/commander/attachments/paste-20261005-120000-00000001.png"; };
  } });
  await page.locator(".xterm").click();
  expect(await pasteInto(page, { files: [PNG] })).toBe(true);
  await expect.poll(() => page.evaluate(() => window.__inserted.length)).toBe(1);
});

test("a converted file is confirmed under its original name", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.InsertPaths = async () => ({
      inserted: [{ id: "00000000000000000000000b", path: "/Users/me/.config/commander/attachments/convert-20261005-120000.000.jpg", name: "convert-20261005-120000.000.jpg", originalName: "IMG_0001.HEIC", kind: "image", thumb: "/media/attach/00000000000000000000000b" }],
      rejected: [],
    });
  } });
  await page.evaluate(() => window.__attach.insertPaths("@1", ["/Users/me/IMG_0001.HEIC"]));
  await expect(page.getByTestId("attach-confirm-title")).toHaveText("Inserted IMG_0001.HEIC (converted) into W1");
  await expect(page.getByTestId("attach-name")).toHaveText("IMG_0001.HEIC (converted)");
});

// ---- Task 10: attach menu and palette rows ----
const shotStubs = () => {
  window.go.main.App.ScreenshotSupport = async () => true;
  window.go.main.App.CaptureScreenshot = async () => "/Users/me/.config/commander/attachments/capture-20261004-120000.000.png";
  window.go.main.App.LastScreenshot = async () => "/Users/me/.config/commander/attachments/capture-20261004-120001.000.png";
  window.go.main.App.PickAttachFiles = async () => ["/Users/me/proj/a.pdf", "/Users/me/proj/b.png"];
  window.go.main.App.ListProjects = async () => [];
};
const CAPTURE = "/Users/me/.config/commander/attachments/capture-20261004-120000.000.png";
const LAST = "/Users/me/.config/commander/attachments/capture-20261004-120001.000.png";
const two = { sessions: [["@1", "W1"], ["@2", "W2"]] };

test("only the active card has the attach menu", async ({ page }) => {
  await boot(page, { ...two, active: "@2", stubs: shotStubs });
  await expect(page.getByTestId("attach-@2")).toBeVisible();
  await expect(page.getByTestId("attach-@1")).toHaveCount(0); // positive control: the line above
});

test("the active card's Capture region inserts into that session", async ({ page }) => {
  await boot(page, { ...two, active: "@2", stubs: shotStubs });
  await page.getByTestId("attach-@2").click();
  await page.getByRole("menuitem", { name: "Capture region…" }).click();
  await expect.poll(() => page.evaluate(() => window.__inserted)).toEqual([["@2", [CAPTURE]]]);
});

test("Last screenshot inserts the copied-in screenshot", async ({ page }) => {
  await boot(page, { stubs: shotStubs });
  await page.getByTestId("attach-@1").click();
  await page.getByRole("menuitem", { name: "Last screenshot" }).click();
  await expect.poll(() => page.evaluate(() => window.__inserted)).toEqual([["@1", [LAST]]]);
});

test("Attach file… inserts every picked file in one call", async ({ page }) => {
  await boot(page, { stubs: shotStubs });
  await page.getByTestId("attach-@1").click();
  await page.getByRole("menuitem", { name: "Attach file…" }).click();
  await expect.poll(() => page.evaluate(() => window.__inserted)).toEqual([["@1", ["/Users/me/proj/a.pdf", "/Users/me/proj/b.png"]]]);
});

test("positive control: a failing Last screenshot toasts Go's error", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.ScreenshotSupport = async () => true;
    window.go.main.App.LastScreenshot = async () => { throw "no screenshots found in /Users/me/Desktop"; };
  } });
  await page.getByTestId("attach-@1").click();
  await page.getByRole("menuitem", { name: "Last screenshot" }).click();
  await expect(page.getByTestId("toast")).toHaveText("no screenshots found in /Users/me/Desktop");
});

test("a cancelled capture inserts nothing and says nothing", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.ScreenshotSupport = async () => true;
    window.go.main.App.CaptureScreenshot = async () => ""; // CaptureScreenshot's cancel
  } });
  await page.getByTestId("attach-@1").click();
  await page.getByRole("menuitem", { name: "Capture region…" }).click();
  await page.waitForTimeout(200);
  expect(await page.evaluate(() => window.__inserted)).toEqual([]);
  await expect(page.getByTestId("toast")).toHaveCount(0);
});

test("Escape closes the attach menu", async ({ page }) => {
  await boot(page, { stubs: shotStubs });
  await page.getByTestId("attach-@1").click();
  await expect(page.getByRole("menu")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menu")).toHaveCount(0);
});

test("without screenshot support: no card menu, no capture rows, but Attach: file… remains", async ({ page }) => {
  await boot(page, { stubs: () => {
    window.go.main.App.ScreenshotSupport = async () => false;
    window.go.main.App.ListProjects = async () => [];
  } });
  await expect(page.locator('[data-testid^="attach-@"]')).toHaveCount(0);
  await page.keyboard.press("Meta+KeyK");
  await expect(page.getByRole("button", { name: /^Attach: / })).toHaveCount(1);
  await expect(page.getByRole("button", { name: /^Attach: file…/ })).toBeVisible();
});

test("the palette captures into the active session", async ({ page }) => {
  await boot(page, { stubs: shotStubs });
  await page.keyboard.press("Meta+KeyK");
  await page.getByRole("button", { name: /^Attach: capture region/ }).click();
  await expect.poll(() => page.evaluate(() => window.__inserted)).toEqual([["@1", [CAPTURE]]]);
});

test("Attach rows survive the 12-row cap behind a long history", async ({ page }) => {
  await boot(page, { ...two, stubs: () => {
    window.go.main.App.ScreenshotSupport = async () => true;
    window.go.main.App.ListProjects = async () => Array.from({ length: 15 }, (_, i) => ({
      folder: `/Users/me/code/proj${i}`, lastModelID: "claude-sonnet-5", missing: false,
    }));
  } });
  await page.keyboard.press("Meta+KeyK");
  await expect(page.getByRole("button", { name: /^Open: / }).first()).toBeVisible(); // history rendered
  await expect(page.getByRole("button", { name: /^Attach: / })).toHaveCount(3);
});

test("with no session there are no Attach rows", async ({ page }) => {
  await boot(page, { active: "", stubs: shotStubs });
  await page.keyboard.press("Meta+KeyK");
  await expect(page.getByTestId("palette")).toBeVisible();
  await expect(page.getByRole("button", { name: /^Attach: / })).toHaveCount(0);
});