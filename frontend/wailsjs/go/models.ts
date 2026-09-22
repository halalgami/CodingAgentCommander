export namespace bedrock {
	
	export class Model {
	    id: string;
	    label: string;
	    upstream: string;
	    region: string;
	    anthropic: boolean;
	    agentCapable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Model(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.upstream = source["upstream"];
	        this.region = source["region"];
	        this.anthropic = source["anthropic"];
	        this.agentCapable = source["agentCapable"];
	    }
	}

}

export namespace deps {
	
	export class Tool {
	    name: string;
	    label: string;
	    found: boolean;
	    path: string;
	    version: string;
	    managed: boolean;
	    canInstall: boolean;
	    required: boolean;
	    hint: string;
	
	    static createFrom(source: any = {}) {
	        return new Tool(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.found = source["found"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.managed = source["managed"];
	        this.canInstall = source["canInstall"];
	        this.required = source["required"];
	        this.hint = source["hint"];
	    }
	}

}

export namespace main {
	
	export class BuildInfo {
	    version: string;
	    commit: string;
	    buildDate: string;
	
	    static createFrom(source: any = {}) {
	        return new BuildInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.commit = source["commit"];
	        this.buildDate = source["buildDate"];
	    }
	}
	export class CompanionJiggle {
	    hair: number;
	    bust: number;
	    skirt: number;
	
	    static createFrom(source: any = {}) {
	        return new CompanionJiggle(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hair = source["hair"];
	        this.bust = source["bust"];
	        this.skirt = source["skirt"];
	    }
	}
	export class CompanionConfig {
	    enabled: boolean;
	    kind: string;
	    size: number;
	    opacity: number;
	    jiggle: CompanionJiggle;
	    modelPath: string;
	    packPath: string;
	
	    static createFrom(source: any = {}) {
	        return new CompanionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.kind = source["kind"];
	        this.size = source["size"];
	        this.opacity = source["opacity"];
	        this.jiggle = this.convertValues(source["jiggle"], CompanionJiggle);
	        this.modelPath = source["modelPath"];
	        this.packPath = source["packPath"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SessionMood {
	    windowID: string;
	    name: string;
	    status: string;
	    statusSinceMs: number;
	    lastFinishMs: number;
	    errorMs: number;
	    ackMs: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionMood(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.windowID = source["windowID"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.statusSinceMs = source["statusSinceMs"];
	        this.lastFinishMs = source["lastFinishMs"];
	        this.errorMs = source["errorMs"];
	        this.ackMs = source["ackMs"];
	    }
	}
	export class CompanionState {
	    sessions: SessionMood[];
	    selected: string;
	    running: number;
	    finished: number;
	    finishSeq: number;
	    lastFinished: string;
	    freshInstall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CompanionState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = this.convertValues(source["sessions"], SessionMood);
	        this.selected = source["selected"];
	        this.running = source["running"];
	        this.finished = source["finished"];
	        this.finishSeq = source["finishSeq"];
	        this.lastFinished = source["lastFinished"];
	        this.freshInstall = source["freshInstall"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DocEntry {
	    rel: string;
	    modTime: number;
	    size: number;
	    sinceStart: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DocEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rel = source["rel"];
	        this.modTime = source["modTime"];
	        this.size = source["size"];
	        this.sinceStart = source["sinceStart"];
	    }
	}
	export class DocLink {
	    text: string;
	    href: string;
	
	    static createFrom(source: any = {}) {
	        return new DocLink(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.href = source["href"];
	    }
	}
	export class DocListing {
	    entries: DocEntry[];
	    root: string;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DocListing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], DocEntry);
	        this.root = source["root"];
	        this.truncated = source["truncated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DocRender {
	    html: string;
	    css: string;
	    kind: string;
	    lang: string;
	    links: DocLink[];
	
	    static createFrom(source: any = {}) {
	        return new DocRender(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.html = source["html"];
	        this.css = source["css"];
	        this.kind = source["kind"];
	        this.lang = source["lang"];
	        this.links = this.convertValues(source["links"], DocLink);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class KeyInfo {
	    env: string;
	    set: boolean;
	    optional: boolean;
	
	    static createFrom(source: any = {}) {
	        return new KeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.env = source["env"];
	        this.set = source["set"];
	        this.optional = source["optional"];
	    }
	}
	export class ModelDetail {
	    id: string;
	    label: string;
	    provider: string;
	    routed: boolean;
	    upstream: string;
	    apiBase: string;
	    keyEnv: string;
	    region: string;
	    inputPrice: number;
	    outputPrice: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.provider = source["provider"];
	        this.routed = source["routed"];
	        this.upstream = source["upstream"];
	        this.apiBase = source["apiBase"];
	        this.keyEnv = source["keyEnv"];
	        this.region = source["region"];
	        this.inputPrice = source["inputPrice"];
	        this.outputPrice = source["outputPrice"];
	    }
	}
	export class ModelInfo {
	    id: string;
	    label: string;
	    routed: boolean;
	    ready: boolean;
	    default: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.routed = source["routed"];
	        this.ready = source["ready"];
	        this.default = source["default"];
	    }
	}
	export class ModelInput {
	    id: string;
	    label: string;
	    provider: string;
	    upstream: string;
	    apiBase: string;
	    keyEnv: string;
	    region: string;
	    inputPrice: number;
	    outputPrice: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.provider = source["provider"];
	        this.upstream = source["upstream"];
	        this.apiBase = source["apiBase"];
	        this.keyEnv = source["keyEnv"];
	        this.region = source["region"];
	        this.inputPrice = source["inputPrice"];
	        this.outputPrice = source["outputPrice"];
	    }
	}
	export class OllamaModel {
	    id: string;
	    label: string;
	    upstream: string;
	
	    static createFrom(source: any = {}) {
	        return new OllamaModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.upstream = source["upstream"];
	    }
	}
	export class PackCanvas {
	    w: number;
	    h: number;
	    scale: number;
	    anchor: string;
	
	    static createFrom(source: any = {}) {
	        return new PackCanvas(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.w = source["w"];
	        this.h = source["h"];
	        this.scale = source["scale"];
	        this.anchor = source["anchor"];
	    }
	}
	export class Pack {
	    schema: number;
	    name: string;
	    author: string;
	    license: string;
	    canvas: PackCanvas;
	    alpha: boolean;
	    slots: Record<string, Array<PackVariant>>;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new Pack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.name = source["name"];
	        this.author = source["author"];
	        this.license = source["license"];
	        this.canvas = this.convertValues(source["canvas"], PackCanvas);
	        this.alpha = source["alpha"];
	        this.slots = this.convertValues(source["slots"], Array<PackVariant>, true);
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PackAnim {
	    strip: string;
	    frames: number;
	    fps: number;
	
	    static createFrom(source: any = {}) {
	        return new PackAnim(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.strip = source["strip"];
	        this.frames = source["frames"];
	        this.fps = source["fps"];
	    }
	}
	export class PackVariantInfo {
	    slot: string;
	    when: string;
	    mood?: string;
	    file: string;
	    preview: string;
	    focusX: number;
	    focusY: number;
	    prompt?: string;
	    model?: string;
	    seed?: string;
	    generatedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new PackVariantInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.slot = source["slot"];
	        this.when = source["when"];
	        this.mood = source["mood"];
	        this.file = source["file"];
	        this.preview = source["preview"];
	        this.focusX = source["focusX"];
	        this.focusY = source["focusY"];
	        this.prompt = source["prompt"];
	        this.model = source["model"];
	        this.seed = source["seed"];
	        this.generatedAt = source["generatedAt"];
	    }
	}
	export class PackBrowse {
	    name: string;
	    path: string;
	    hasBase: boolean;
	    variants: PackVariantInfo[];
	    warnings: string[];
	    missing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PackBrowse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.hasBase = source["hasBase"];
	        this.variants = this.convertValues(source["variants"], PackVariantInfo);
	        this.warnings = source["warnings"];
	        this.missing = source["missing"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class PackGenBase {
	    path: string;
	    w: number;
	    h: number;
	    warning?: string;
	
	    static createFrom(source: any = {}) {
	        return new PackGenBase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.w = source["w"];
	        this.h = source["h"];
	        this.warning = source["warning"];
	    }
	}
	export class PackGenCondition {
	    name: string;
	    class: string;
	    slots: string[];
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new PackGenCondition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.class = source["class"];
	        this.slots = source["slots"];
	        this.label = source["label"];
	    }
	}
	export class PackGenCost {
	    usd: number;
	    known: boolean;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new PackGenCost(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.usd = source["usd"];
	        this.known = source["known"];
	        this.note = source["note"];
	    }
	}
	export class PackGenItem {
	    slot: string;
	    when: string;
	    status: string;
	    error?: string;
	    preview?: string;
	
	    static createFrom(source: any = {}) {
	        return new PackGenItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.slot = source["slot"];
	        this.when = source["when"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.preview = source["preview"];
	    }
	}
	export class PackGenModel {
	    id: string;
	    label: string;
	    mode: string;
	    usd: number;
	    known: boolean;
	    refusal?: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new PackGenModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.mode = source["mode"];
	        this.usd = source["usd"];
	        this.known = source["known"];
	        this.refusal = source["refusal"];
	        this.note = source["note"];
	    }
	}
	export class PackGenProvider {
	    id: string;
	    models: PackGenModel[];
	    hasKey: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PackGenProvider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.models = this.convertValues(source["models"], PackGenModel);
	        this.hasKey = source["hasKey"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PackGenSlot {
	    slot: string;
	    when: string;
	    staging: string;
	    prompt: string;
	
	    static createFrom(source: any = {}) {
	        return new PackGenSlot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.slot = source["slot"];
	        this.when = source["when"];
	        this.staging = source["staging"];
	        this.prompt = source["prompt"];
	    }
	}
	export class PackGenRequest {
	    name: string;
	    providerId: string;
	    model: string;
	    styleId: string;
	    basePath: string;
	    slots: PackGenSlot[];
	    regenerate: boolean;
	    packPath: string;
	    overwrite: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PackGenRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.providerId = source["providerId"];
	        this.model = source["model"];
	        this.styleId = source["styleId"];
	        this.basePath = source["basePath"];
	        this.slots = this.convertValues(source["slots"], PackGenSlot);
	        this.regenerate = source["regenerate"];
	        this.packPath = source["packPath"];
	        this.overwrite = source["overwrite"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class PackGenState {
	    id: string;
	    name: string;
	    status: string;
	    total: number;
	    done: number;
	    billed: number;
	    items: PackGenItem[];
	    error?: string;
	    saved: boolean;
	    hasArt: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PackGenState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.total = source["total"];
	        this.done = source["done"];
	        this.billed = source["billed"];
	        this.items = this.convertValues(source["items"], PackGenItem);
	        this.error = source["error"];
	        this.saved = source["saved"];
	        this.hasArt = source["hasArt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PackGenStyle {
	    id: string;
	    label: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new PackGenStyle(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.text = source["text"];
	    }
	}
	export class PackSummary {
	    name: string;
	    path: string;
	    folder: string;
	    scenes: number;
	    thumb: string;
	    hasBase: boolean;
	    active: boolean;
	    warnings: number;
	
	    static createFrom(source: any = {}) {
	        return new PackSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.folder = source["folder"];
	        this.scenes = source["scenes"];
	        this.thumb = source["thumb"];
	        this.hasBase = source["hasBase"];
	        this.active = source["active"];
	        this.warnings = source["warnings"];
	    }
	}
	export class PackVariant {
	    id: string;
	    file: string;
	    mood: string;
	    rarity: number;
	    when: string;
	    weight: number;
	    focusX: number;
	    focusY: number;
	    headScale: number;
	    anim?: PackAnim;
	    meta: Record<string, string>;
	    motion: string;
	    motionId: string;
	
	    static createFrom(source: any = {}) {
	        return new PackVariant(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.file = source["file"];
	        this.mood = source["mood"];
	        this.rarity = source["rarity"];
	        this.when = source["when"];
	        this.weight = source["weight"];
	        this.focusX = source["focusX"];
	        this.focusY = source["focusY"];
	        this.headScale = source["headScale"];
	        this.anim = this.convertValues(source["anim"], PackAnim);
	        this.meta = source["meta"];
	        this.motion = source["motion"];
	        this.motionId = source["motionId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class UsageWindow {
	    key: string;
	    label: string;
	    weekly: boolean;
	    utilization: number;
	    resetsAt: string;
	
	    static createFrom(source: any = {}) {
	        return new UsageWindow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.weekly = source["weekly"];
	        this.utilization = source["utilization"];
	        this.resetsAt = source["resetsAt"];
	    }
	}
	export class PlanUsage {
	    windows: UsageWindow[];
	    fetchedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new PlanUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.windows = this.convertValues(source["windows"], UsageWindow);
	        this.fetchedAt = source["fetchedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProjectEntry {
	    folder: string;
	    label?: string;
	    lastModelID: string;
	    lastOpened: number;
	    openCount: number;
	    pinned: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProjectEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.label = source["label"];
	        this.lastModelID = source["lastModelID"];
	        this.lastOpened = source["lastOpened"];
	        this.openCount = source["openCount"];
	        this.pinned = source["pinned"];
	    }
	}
	export class ProjectView {
	    folder: string;
	    label: string;
	    lastModelID: string;
	    lastOpened: number;
	    openCount: number;
	    pinned: boolean;
	    missing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProjectView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.label = source["label"];
	        this.lastModelID = source["lastModelID"];
	        this.lastOpened = source["lastOpened"];
	        this.openCount = source["openCount"];
	        this.pinned = source["pinned"];
	        this.missing = source["missing"];
	    }
	}
	export class ProviderInfo {
	    type: string;
	    defined: boolean;
	    active: boolean;
	    apiBase: string;
	    region: string;
	    modelCnt: number;
	
	    static createFrom(source: any = {}) {
	        return new ProviderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.defined = source["defined"];
	        this.active = source["active"];
	        this.apiBase = source["apiBase"];
	        this.region = source["region"];
	        this.modelCnt = source["modelCnt"];
	    }
	}
	export class SessionInfo {
	    windowID: string;
	    name: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.windowID = source["windowID"];
	        this.name = source["name"];
	        this.model = source["model"];
	    }
	}
	
	export class SessionStats {
	    contextTokens: number;
	    estCostPerTurn: number;
	    unpriced: boolean;
	    band: string;
	    turns: number;
	    model: string;
	    provider: string;
	    uptimeSeconds: number;
	    status: string;
	    remoteControl: boolean;
	    cwd: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.contextTokens = source["contextTokens"];
	        this.estCostPerTurn = source["estCostPerTurn"];
	        this.unpriced = source["unpriced"];
	        this.band = source["band"];
	        this.turns = source["turns"];
	        this.model = source["model"];
	        this.provider = source["provider"];
	        this.uptimeSeconds = source["uptimeSeconds"];
	        this.status = source["status"];
	        this.remoteControl = source["remoteControl"];
	        this.cwd = source["cwd"];
	    }
	}

}

export namespace router {
	
	export class RuntimeStatus {
	    installed: boolean;
	    path: string;
	    managed: boolean;
	    python: string;
	    canInstall: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.path = source["path"];
	        this.managed = source["managed"];
	        this.python = source["python"];
	        this.canInstall = source["canInstall"];
	    }
	}

}

export namespace zen {
	
	export class Model {
	    id: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new Model(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	    }
	}

}

