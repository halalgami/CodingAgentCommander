// Al Asturlab AI card state (spec §7). Opening the view only reads the cached
// briefing; the model is called on a click (RunBriefing) or an Enter (AskStats).
import { Briefing, RunBriefing, AskStats } from "../../../wailsjs/go/main/App.js";

export const ai = $state({ brief: null, briefBusy: false, ask: null, askBusy: false, question: "" });

export async function loadBriefing() {
  try { ai.brief = await Briefing(); } catch { ai.brief = null; }
}

export async function runBriefing() {
  if (ai.briefBusy) return;
  ai.briefBusy = true;
  try { ai.brief = await RunBriefing(); } catch (e) { ai.brief = { ...(ai.brief || {}), error: String(e) }; }
  ai.briefBusy = false;
}

export async function ask() {
  const q = ai.question.trim();
  if (!q || ai.askBusy) return;
  ai.askBusy = true;
  try { ai.ask = await AskStats(q); } catch (e) { ai.ask = { question: q, error: String(e), results: [], queries: [] }; }
  ai.askBusy = false;
}

export { weekLabel, answerBars } from "./ai.js";
