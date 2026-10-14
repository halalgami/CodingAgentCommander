// CodeCity layout (spec §4): an ordered squarified treemap of the folder tree
// in tile units. Each folder's own files pack row by row into its own block;
// sibling folders are separated by a 1-tile street. Deterministic: children
// sorted by path, never by size, so a file growing a little only moves lots
// inside its own folder.
export const lotSide = (code) => Math.max(1, Math.min(3, Math.round(Math.sqrt((code || 0) / 150))));
const SLAB = 2;

function tree(view) {
  const root = { path: "", label: "", kids: new Map(), files: [], slab: null };
  const node = (p) => {
    if (p === "") return root;
    let n = root, acc = "";
    for (const seg of p.split("/")) {
      acc = acc ? acc + "/" + seg : seg;
      if (!n.kids.has(seg)) n.kids.set(seg, { path: acc, label: seg, kids: new Map(), files: [], slab: null });
      n = n.kids.get(seg);
    }
    return n;
  };
  for (const f of view.files || []) node(f.folder || "").files.push(f);
  for (const s of view.slabs || []) node(s.folder || "").slab = s;
  return root;
}

// need: tiles a node asks for (its lots plus 30 % slack and its streets).
function need(n) {
  let own = 0;
  for (const f of n.files) own += lotSide(f.code) ** 2;
  if (n.slab) own += SLAB * SLAB;
  n.own = own ? Math.ceil(own * 1.3) + 2 : 0;
  n.children = [...n.kids.values()].sort((a, b) => (a.path < b.path ? -1 : 1));
  n.need = n.own;
  for (const c of n.children) n.need += need(c) + 2 * Math.sqrt(c.need) + 1;
  return n.need;
}

// Ordered squarify: lay items (in order) into rows along the short side,
// closing a row when the next item would worsen its worst aspect ratio.
function squarify(items, x, y, w, h) {
  const total = items.reduce((s, it) => s + it.weight, 0) || 1;
  const scale = (w * h) / total;
  const out = [];
  let i = 0;
  while (i < items.length) {
    const horiz = w >= h, side = horiz ? h : w;
    let row = [], rowSum = 0, best = Infinity;
    for (let j = i; j < items.length; j++) {
      const s = rowSum + items[j].weight * scale;
      const thick = s / side;
      let worst = 0;
      for (const it of [...row, items[j]]) {
        const len = (it.weight * scale) / thick;
        worst = Math.max(worst, thick / len, len / thick);
      }
      if (row.length && worst > best) break;
      row.push(items[j]); rowSum = s; best = worst;
    }
    const thick = rowSum / side;
    let off = 0;
    for (const it of row) {
      const len = (it.weight * scale) / thick;
      out.push(horiz ? { it, x, y: y + off, w: thick, h: len } : { it, x: x + off, y, w: len, h: thick });
      off += len;
    }
    if (horiz) { x += thick; w -= thick; } else { y += thick; h -= thick; }
    i += row.length;
  }
  return out;
}

const snap = (r) => {
  const x = Math.round(r.x), y = Math.round(r.y);
  return { x, y, w: Math.max(1, Math.round(r.x + r.w) - x), h: Math.max(1, Math.round(r.y + r.h) - y) };
};

function pack(n, rect, depth, out) {
  const folder = { path: n.path, label: n.label || "", x: rect.x, y: rect.y, w: rect.w, h: rect.h, depth, hidden: 0 };
  out.folders.push(folder);
  const items = [];
  if (n.own) items.push({ own: true, weight: n.own });
  for (const c of n.children) items.push({ node: c, weight: c.need });
  for (const r of squarify(items, rect.x, rect.y, rect.w, rect.h)) {
    const s = snap(r);
    if (r.it.own) placeLots(n, s, folder, out);
    else if (s.w > 2 && s.h > 2) pack(r.it.node, { x: s.x, y: s.y, w: s.w - 1, h: s.h - 1 }, depth + 1, out); // 1-tile street right/bottom
    else hideAll(r.it.node, out, folder);
  }
}

function hideAll(n, out, into) {
  into.hidden += n.files.length;
  for (const c of n.children) hideAll(c, out, into);
}

// Shelf packing, largest lots first (path order within a size); the slab goes last.
function placeLots(n, r, folder, out) {
  const lots = n.files.map((f) => ({ kind: "file", ref: f, s: lotSide(f.code) }))
    .sort((a, b) => b.s - a.s || (a.ref.path < b.ref.path ? -1 : 1));
  if (n.slab) lots.push({ kind: "slab", ref: n.slab, s: SLAB });
  let x = r.x, y = r.y, shelf = 0;
  for (const lot of lots) {
    if (x + lot.s > r.x + r.w) { x = r.x; y += shelf; shelf = 0; }
    if (lot.s > r.w || y + lot.s > r.y + r.h) { if (lot.kind === "file") folder.hidden++; continue; }
    out.lots.push({ kind: lot.kind, ref: lot.ref, x, y, s: lot.s });
    x += lot.s; shelf = Math.max(shelf, lot.s);
  }
}

export function layoutCode(view) {
  const root = tree(view);
  need(root);
  // Rounding can leave a small folder a strip too thin to hold it; grow the
  // root a little and retry (bounded) until every lot fits.
  let side = Math.max(4, Math.ceil(Math.sqrt(root.need))), out;
  for (let tries = 0; tries < 8; tries++) {
    out = { side, folders: [], lots: [] };
    pack(root, { x: 0, y: 0, w: side, h: side }, 0, out);
    if (!out.folders.some((f) => f.hidden)) break;
    side += Math.max(2, Math.ceil(side * 0.12));
  }
  out.bounds = { x0: 0, y0: 0, x1: out.side, y1: out.side };
  return out;
}
