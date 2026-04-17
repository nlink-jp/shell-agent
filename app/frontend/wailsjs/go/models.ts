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

