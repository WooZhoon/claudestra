export namespace main {
	
	export class AgentDetailInfo {
	    id: string;
	    role: string;
	    status: string;
	    isConsumer: boolean;
	    instruction: string;
	    output: string;
	    logs: string;
	    allowedTools: string[];
	
	    static createFrom(source: any = {}) {
	        return new AgentDetailInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.status = source["status"];
	        this.isConsumer = source["isConsumer"];
	        this.instruction = source["instruction"];
	        this.output = source["output"];
	        this.logs = source["logs"];
	        this.allowedTools = source["allowedTools"];
	    }
	}
	export class AgentStatusInfo {
	    id: string;
	    role: string;
	    status: string;
	    isConsumer: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentStatusInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.status = source["status"];
	        this.isConsumer = source["isConsumer"];
	    }
	}

}

