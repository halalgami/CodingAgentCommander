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

