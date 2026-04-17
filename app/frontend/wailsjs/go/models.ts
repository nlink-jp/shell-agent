export namespace config {
	
	export class APIConfig {
	    endpoint: string;
	    model: string;
	    api_key?: string;
	
	    static createFrom(source: any = {}) {
	        return new APIConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpoint = source["endpoint"];
	        this.model = source["model"];
	        this.api_key = source["api_key"];
	    }
	}
	export class WindowConfig {
	    x: number;
	    y: number;
	    width: number;
	    height: number;
	
	    static createFrom(source: any = {}) {
	        return new WindowConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}
	export class GuardianConfig {
	    binary_path: string;
	    config_path: string;
	
	    static createFrom(source: any = {}) {
	        return new GuardianConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.binary_path = source["binary_path"];
	        this.config_path = source["config_path"];
	    }
	}
	export class ToolsConfig {
	    script_dir: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolsConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.script_dir = source["script_dir"];
	    }
	}
	export class MemoryConfig {
	    hot_token_limit: number;
	    warm_retention_mins: number;
	    cold_retention_mins: number;
	
	    static createFrom(source: any = {}) {
	        return new MemoryConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hot_token_limit = source["hot_token_limit"];
	        this.warm_retention_mins = source["warm_retention_mins"];
	        this.cold_retention_mins = source["cold_retention_mins"];
	    }
	}
	export class Config {
	    api: APIConfig;
	    memory: MemoryConfig;
	    tools: ToolsConfig;
	    guardian: GuardianConfig;
	    window: WindowConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.api = this.convertValues(source["api"], APIConfig);
	        this.memory = this.convertValues(source["memory"], MemoryConfig);
	        this.tools = this.convertValues(source["tools"], ToolsConfig);
	        this.guardian = this.convertValues(source["guardian"], GuardianConfig);
	        this.window = this.convertValues(source["window"], WindowConfig);
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

export namespace main {
	
	export class ChatMessage {
	    role: string;
	    content: string;
	    timestamp: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.timestamp = source["timestamp"];
	    }
	}
	export class LLMStatus {
	    current_time: string;
	    hot_messages: number;
	    warm_summaries: number;
	    cold_summaries: number;
	    tokens_used: number;
	    token_limit: number;
	
	    static createFrom(source: any = {}) {
	        return new LLMStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current_time = source["current_time"];
	        this.hot_messages = source["hot_messages"];
	        this.warm_summaries = source["warm_summaries"];
	        this.cold_summaries = source["cold_summaries"];
	        this.tokens_used = source["tokens_used"];
	        this.token_limit = source["token_limit"];
	    }
	}
	export class SessionInfo {
	    id: string;
	    title: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class ToolInfo {
	    name: string;
	    description: string;
	    category: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.category = source["category"];
	    }
	}

}

export namespace memory {
	
	export class PinnedMemory {
	    fact: string;
	    category: string;
	    // Go type: time
	    source_time: any;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new PinnedMemory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fact = source["fact"];
	        this.category = source["category"];
	        this.source_time = this.convertValues(source["source_time"], null);
	        this.created_at = this.convertValues(source["created_at"], null);
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

