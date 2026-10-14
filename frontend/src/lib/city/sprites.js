// Decodes the sprite pack once (spec §7). Pending or failed sprites are left
// out; the painter draws a flat box for anything missing.
async function defaultLoad(file) {
  const res = await fetch(file);
  if (!res.ok) throw new Error(`${file}: ${res.status}`);
  return drawable(await res.blob());
}

// drawable turns a blob into something drawImage accepts: an ImageBitmap, or a
// decoded <img> where createImageBitmap is missing (never the Blob itself).
export async function drawable(blob, env = globalThis) {
  if (typeof env.createImageBitmap === "function") return env.createImageBitmap(blob);
  const img = new env.Image();
  const url = env.URL.createObjectURL(blob);
  img.src = url;
  try { await img.decode(); } finally { env.URL.revokeObjectURL(url); }
  return img;
}

export async function loadSprites(manifest, load = defaultLoad) {
  const out = new Map();
  await Promise.all(Object.entries(manifest).map(async ([k, s]) => {
    if (s.pending) return;
    try { out.set(k, await load(s.file)); } catch { /* missing → box fallback */ }
  }));
  return out;
}

export function releaseSprites(map) {
  for (const img of map.values()) if (img && typeof img.close === "function") img.close();
  map.clear();
}
