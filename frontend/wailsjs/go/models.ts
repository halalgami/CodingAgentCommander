export namespace bedrock {
	
	export class Model {
	    id: string;
	    label: string;
	    upstream: string;
	    region: string;
	    anthropic: boolean;
	
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
	    }
	}

}

export namespace main {
	
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
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.routed = source["routed"];
	        this.ready = source["ready"];
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
	    }
	}

}

