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

export namespace city {
	
	export class Block {
	    week: string;
	    hours: number;
	    sessions: number;
	
	    static createFrom(source: any = {}) {
	        return new Block(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.week = source["week"];
	        this.hours = source["hours"];
	        this.sessions = source["sessions"];
	    }
	}
	export class Building {
	    sessionID: string;
	    title: string;
	    archetype: string;
	    hours: number;
	    seed: number;
	    startedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Building(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionID = source["sessionID"];
	        this.title = source["title"];
	        this.archetype = source["archetype"];
	        this.hours = source["hours"];
	        this.seed = source["seed"];
	        this.startedAt = source["startedAt"];
	    }
	}
	export class Landmark {
	    kind: string;
	    sessionID: string;
	
	    static createFrom(source: any = {}) {
	        return new Landmark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.sessionID = source["sessionID"];
	    }
	}
	export class Plaque {
	    id: string;
	    name: string;
	    at: number;
	
	    static createFrom(source: any = {}) {
	        return new Plaque(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.at = source["at"];
	    }
	}
	export class District {
	    key: string;
	    label: string;
	    rank: string;
	    hours: number;
	    plot: number;
	    land: number;
	    roads: number;
	    parks: number;
	    cracks: number;
	    oldTown: boolean;
	    ruins: boolean;
	    plaques: Plaque[];
	    buildings: Building[];
	    blocks: Block[];
	    oldTownCount: number;
	    landmarks: Landmark[];
	
	    static createFrom(source: any = {}) {
	        return new District(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.rank = source["rank"];
	        this.hours = source["hours"];
	        this.plot = source["plot"];
	        this.land = source["land"];
	        this.roads = source["roads"];
	        this.parks = source["parks"];
	        this.cracks = source["cracks"];
	        this.oldTown = source["oldTown"];
	        this.ruins = source["ruins"];
	        this.plaques = this.convertValues(source["plaques"], Plaque);
	        this.buildings = this.convertValues(source["buildings"], Building);
	        this.blocks = this.convertValues(source["blocks"], Block);
	        this.oldTownCount = source["oldTownCount"];
	        this.landmarks = this.convertValues(source["landmarks"], Landmark);
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
	export class CityView {
	    districts: District[];
	    locked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CityView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.districts = this.convertValues(source["districts"], District);
	        this.locked = source["locked"];
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
	
	

}

export namespace codecity {
	
	export class Slab {
	    folder: string;
	    files: number;
	    code: number;
	
	    static createFrom(source: any = {}) {
	        return new Slab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.files = source["files"];
	        this.code = source["code"];
	    }
	}
	export class Touch {
	    reads: number;
	    edits: number;
	    writes: number;
	    lastAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Touch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reads = source["reads"];
	        this.edits = source["edits"];
	        this.writes = source["writes"];
	        this.lastAt = source["lastAt"];
	    }
	}
	export class File {
	    path: string;
	    folder: string;
	    lang: string;
	    code: number;
	    complexity: number;
	    todos: number;
	    hotspot: number;
	    flagged: boolean;
	    test: boolean;
	    ghost: boolean;
	    new: boolean;
	    touch: Touch;
	
	    static createFrom(source: any = {}) {
	        return new File(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.folder = source["folder"];
	        this.lang = source["lang"];
	        this.code = source["code"];
	        this.complexity = source["complexity"];
	        this.todos = source["todos"];
	        this.hotspot = source["hotspot"];
	        this.flagged = source["flagged"];
	        this.test = source["test"];
	        this.ghost = source["ghost"];
	        this.new = source["new"];
	        this.touch = this.convertValues(source["touch"], Touch);
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
	export class Folder {
	    path: string;
	    code: number;
	
	    static createFrom(source: any = {}) {
	        return new Folder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.code = source["code"];
	    }
	}
	export class CodeView {
	    key: string;
	    label: string;
	    locked: boolean;
	    noMetrics: boolean;
	    missing: boolean;
	    scannedAt: number;
	    folders: Folder[];
	    files: File[];
	    slabs: Slab[];
	
	    static createFrom(source: any = {}) {
	        return new CodeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.locked = source["locked"];
	        this.noMetrics = source["noMetrics"];
	        this.missing = source["missing"];
	        this.scannedAt = source["scannedAt"];
	        this.folders = this.convertValues(source["folders"], Folder);
	        this.files = this.convertValues(source["files"], File);
	        this.slabs = this.convertValues(source["slabs"], Slab);
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

export namespace ingest {
	
	export class Progress {
	    files: number;
	    total: number;
	    bytes: number;
	    done: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Progress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.total = source["total"];
	        this.bytes = source["bytes"];
	        this.done = source["done"];
	    }
	}

}

export namespace live {
	
	export class Agent {
	    id: string;
	    type: string;
	    description: string;
	    model: string;
	    background: boolean;
	    depth: number;
	    state: string;
	    tool: string;
	    target: string;
	    targetRel: string;
	    startedMs: number;
	    endedMs: number;
	    tools: number;
	
	    static createFrom(source: any = {}) {
	        return new Agent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.description = source["description"];
	        this.model = source["model"];
	        this.background = source["background"];
	        this.depth = source["depth"];
	        this.state = source["state"];
	        this.tool = source["tool"];
	        this.target = source["target"];
	        this.targetRel = source["targetRel"];
	        this.startedMs = source["startedMs"];
	        this.endedMs = source["endedMs"];
	        this.tools = source["tools"];
	    }
	}
	export class Session {
	    id: string;
	    cwd: string;
	    branch: string;
	    title: string;
	    summary: string;
	    state: string;
	    tool: string;
	    target: string;
	    targetRel: string;
	    compactedMs: number;
	    sinceMs: number;
	    turnOpen: boolean;
	    unread: boolean;
	    external: boolean;
	    contextTokens: number;
	    messages: number;
	    lastEventMs: number;
	    model: string;
	    crew: Agent[];
	    delegating: number;
	    crewOut: number;
	    orphan: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.cwd = source["cwd"];
	        this.branch = source["branch"];
	        this.title = source["title"];
	        this.summary = source["summary"];
	        this.state = source["state"];
	        this.tool = source["tool"];
	        this.target = source["target"];
	        this.targetRel = source["targetRel"];
	        this.compactedMs = source["compactedMs"];
	        this.sinceMs = source["sinceMs"];
	        this.turnOpen = source["turnOpen"];
	        this.unread = source["unread"];
	        this.external = source["external"];
	        this.contextTokens = source["contextTokens"];
	        this.messages = source["messages"];
	        this.lastEventMs = source["lastEventMs"];
	        this.model = source["model"];
	        this.crew = this.convertValues(source["crew"], Agent);
	        this.delegating = source["delegating"];
	        this.crewOut = source["crewOut"];
	        this.orphan = source["orphan"];
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
	export class TimelineEvent {
	    session: string;
	    at: number;
	    kind: string;
	    label: string;
	    agent?: string;
	
	    static createFrom(source: any = {}) {
	        return new TimelineEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session = source["session"];
	        this.at = source["at"];
	        this.kind = source["kind"];
	        this.label = source["label"];
	        this.agent = source["agent"];
	    }
	}

}

export namespace main {
	
	export class Rejection {
	    name: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new Rejection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.reason = source["reason"];
	    }
	}
	export class Attachment {
	    id: string;
	    path: string;
	    name: string;
	    kind: string;
	    thumb: string;
	    originalName: string;
	
	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.thumb = source["thumb"];
	        this.originalName = source["originalName"];
	    }
	}
	export class AttachResult {
	    inserted: Attachment[];
	    rejected: Rejection[];
	
	    static createFrom(source: any = {}) {
	        return new AttachResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inserted = this.convertValues(source["inserted"], Attachment);
	        this.rejected = this.convertValues(source["rejected"], Rejection);
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
	export class CommanderView {
	    level: number;
	    xp: number;
	    next: number;
	    unlocks: store.Unlock[];
	
	    static createFrom(source: any = {}) {
	        return new CommanderView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.xp = source["xp"];
	        this.next = source["next"];
	        this.unlocks = this.convertValues(source["unlocks"], store.Unlock);
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
	export class DashSnapshot {
	    sessions: live.Session[];
	    projects: store.ProjectInfo[];
	    timeline: Record<string, Array<live.TimelineEvent>>;
	
	    static createFrom(source: any = {}) {
	        return new DashSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = this.convertValues(source["sessions"], live.Session);
	        this.projects = this.convertValues(source["projects"], store.ProjectInfo);
	        this.timeline = this.convertValues(source["timeline"], Array<live.TimelineEvent>, true);
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
	export class LedgerStatus {
	    enabled: boolean;
	    storeTitles: boolean;
	    readOnly: boolean;
	    excluded: string[];
	    progress: ingest.Progress;
	    lastError: string;
	    dir: string;
	    hasData: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LedgerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.storeTitles = source["storeTitles"];
	        this.readOnly = source["readOnly"];
	        this.excluded = source["excluded"];
	        this.progress = this.convertValues(source["progress"], ingest.Progress);
	        this.lastError = source["lastError"];
	        this.dir = source["dir"];
	        this.hasData = source["hasData"];
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
	export class ModelDetail {
	    id: string;
	    label: string;
	    provider: string;
	    routed: boolean;
	    upstream: string;
	    apiBase: string;
	    keyEnv: string;
	    region: string;
	    contextWindow: number;
	    maxOutputTokens: number;
	    autoContinue: boolean;
	    autoContinueSet: boolean;
	    autoContinueReady: boolean;
	
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
	        this.contextWindow = source["contextWindow"];
	        this.maxOutputTokens = source["maxOutputTokens"];
	        this.autoContinue = source["autoContinue"];
	        this.autoContinueSet = source["autoContinueSet"];
	        this.autoContinueReady = source["autoContinueReady"];
	    }
	}
	export class ModelInfo {
	    id: string;
	    label: string;
	    routed: boolean;
	    ready: boolean;
	    default: boolean;
	    provider: string;
	
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
	        this.provider = source["provider"];
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
	export class PowerInfo {
	    lowPower: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PowerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lowPower = source["lowPower"];
	    }
	}
	export class ProjectProgress {
	    key: string;
	    label: string;
	    rank: string;
	    hours: number;
	    difficulty: number;
	    heroes: progress.Hero[];
	    unlocks: store.Unlock[];
	
	    static createFrom(source: any = {}) {
	        return new ProjectProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.rank = source["rank"];
	        this.hours = source["hours"];
	        this.difficulty = source["difficulty"];
	        this.heroes = this.convertValues(source["heroes"], progress.Hero);
	        this.unlocks = this.convertValues(source["unlocks"], store.Unlock);
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
	export class ProgressView {
	    commander: CommanderView;
	    projects: ProjectProgress[];
	    live: progress.Hero[];
	    locked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProgressView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commander = this.convertValues(source["commander"], CommanderView);
	        this.projects = this.convertValues(source["projects"], ProjectProgress);
	        this.live = this.convertValues(source["live"], progress.Hero);
	        this.locked = source["locked"];
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
	export class ProjectMetricsView {
	    files: Record<string, scan.FileMetrics>;
	    snapshots: scan.Snapshot[];
	
	    static createFrom(source: any = {}) {
	        return new ProjectMetricsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = this.convertValues(source["files"], scan.FileMetrics, true);
	        this.snapshots = this.convertValues(source["snapshots"], scan.Snapshot);
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
	export class RaidPackSummary {
	    dir: string;
	    name: string;
	    count: number;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RaidPackSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.name = source["name"];
	        this.count = source["count"];
	        this.active = source["active"];
	    }
	}
	export class RaidPackView {
	    dir: string;
	    name: string;
	    files: Record<string, string>;
	    heroines: Record<string, raidpack.Heroine>;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new RaidPackView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.name = source["name"];
	        this.files = source["files"];
	        this.heroines = this.convertValues(source["heroines"], raidpack.Heroine, true);
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
	    contextWindow: number;
	    band: string;
	    turns: number;
	    model: string;
	    provider: string;
	    uptimeSeconds: number;
	    status: string;
	    remoteControl: boolean;
	    cwd: string;
	    sessionID: string;
	    title: string;
	    activity: string;
	    tool: string;
	    target: string;
	    summary: string;
	    activitySince: number;
	    unread: boolean;
	    hotspot: boolean;
	    autoContinues: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.contextTokens = source["contextTokens"];
	        this.contextWindow = source["contextWindow"];
	        this.band = source["band"];
	        this.turns = source["turns"];
	        this.model = source["model"];
	        this.provider = source["provider"];
	        this.uptimeSeconds = source["uptimeSeconds"];
	        this.status = source["status"];
	        this.remoteControl = source["remoteControl"];
	        this.cwd = source["cwd"];
	        this.sessionID = source["sessionID"];
	        this.title = source["title"];
	        this.activity = source["activity"];
	        this.tool = source["tool"];
	        this.target = source["target"];
	        this.summary = source["summary"];
	        this.activitySince = source["activitySince"];
	        this.unread = source["unread"];
	        this.hotspot = source["hotspot"];
	        this.autoContinues = source["autoContinues"];
	    }
	}

}

export namespace progress {
	
	export class Loot {
	    commits: number;
	    lines: number;
	    tests: number;
	
	    static createFrom(source: any = {}) {
	        return new Loot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commits = source["commits"];
	        this.lines = source["lines"];
	        this.tests = source["tests"];
	    }
	}
	export class Hero {
	    sessionID: string;
	    xp: number;
	    level: number;
	    title: string;
	    tier: number;
	    build: string;
	    mix: Record<string, number>;
	    loot: Loot;
	
	    static createFrom(source: any = {}) {
	        return new Hero(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionID = source["sessionID"];
	        this.xp = source["xp"];
	        this.level = source["level"];
	        this.title = source["title"];
	        this.tier = source["tier"];
	        this.build = source["build"];
	        this.mix = source["mix"];
	        this.loot = this.convertValues(source["loot"], Loot);
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

}

export namespace raidpack {
	
	export class Heroine {
	    name?: string;
	    tint?: string;
	
	    static createFrom(source: any = {}) {
	        return new Heroine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.tint = source["tint"];
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

export namespace scan {
	
	export class FileMetrics {
	    lang: string;
	    code: number;
	    comment: number;
	    blank: number;
	    complexity: number;
	    todos?: number;
	    churn90?: number;
	    hotspot?: number;
	    flagged?: boolean;
	    test?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileMetrics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lang = source["lang"];
	        this.code = source["code"];
	        this.comment = source["comment"];
	        this.blank = source["blank"];
	        this.complexity = source["complexity"];
	        this.todos = source["todos"];
	        this.churn90 = source["churn90"];
	        this.hotspot = source["hotspot"];
	        this.flagged = source["flagged"];
	        this.test = source["test"];
	    }
	}
	export class Snapshot {
	    day: string;
	    files: number;
	    code: number;
	    complexity: number;
	    tests: number;
	    todos: number;
	    langs: Record<string, number>;
	    testRatio: number;
	    head: string;
	    partial?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.files = source["files"];
	        this.code = source["code"];
	        this.complexity = source["complexity"];
	        this.tests = source["tests"];
	        this.todos = source["todos"];
	        this.langs = source["langs"];
	        this.testRatio = source["testRatio"];
	        this.head = source["head"];
	        this.partial = source["partial"];
	    }
	}

}

export namespace store {
	
	export class TokenCounts {
	    in: number;
	    out: number;
	    cacheRead: number;
	    cacheWrite: number;
	
	    static createFrom(source: any = {}) {
	        return new TokenCounts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.in = source["in"];
	        this.out = source["out"];
	        this.cacheRead = source["cacheRead"];
	        this.cacheWrite = source["cacheWrite"];
	    }
	}
	export class Counters {
	    activeMs: number;
	    turns: number;
	    sessions?: number;
	    tools?: Record<string, number>;
	    errors: number;
	    compactions: number;
	    commits: number;
	    testRuns: number;
	    testFails: number;
	    linesAdded: number;
	    linesRemoved: number;
	    subagents: number;
	    skipped: number;
	    tokens: TokenCounts;
	
	    static createFrom(source: any = {}) {
	        return new Counters(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.activeMs = source["activeMs"];
	        this.turns = source["turns"];
	        this.sessions = source["sessions"];
	        this.tools = source["tools"];
	        this.errors = source["errors"];
	        this.compactions = source["compactions"];
	        this.commits = source["commits"];
	        this.testRuns = source["testRuns"];
	        this.testFails = source["testFails"];
	        this.linesAdded = source["linesAdded"];
	        this.linesRemoved = source["linesRemoved"];
	        this.subagents = source["subagents"];
	        this.skipped = source["skipped"];
	        this.tokens = this.convertValues(source["tokens"], TokenCounts);
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
	export class ProjectInfo {
	    key: string;
	    label: string;
	    root: string;
	    missing: boolean;
	    firstSeen: number;
	    lastSeen: number;
	    totals: Counters;
	    scannedAt: number;
	    scanHead: string;
	    latest?: scan.Snapshot;
	    plot: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.root = source["root"];
	        this.missing = source["missing"];
	        this.firstSeen = source["firstSeen"];
	        this.lastSeen = source["lastSeen"];
	        this.totals = this.convertValues(source["totals"], Counters);
	        this.scannedAt = source["scannedAt"];
	        this.scanHead = source["scanHead"];
	        this.latest = this.convertValues(source["latest"], scan.Snapshot);
	        this.plot = source["plot"];
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
	
	export class Unlock {
	    id: string;
	    at: number;
	    sessionID?: string;
	
	    static createFrom(source: any = {}) {
	        return new Unlock(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = source["at"];
	        this.sessionID = source["sessionID"];
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

