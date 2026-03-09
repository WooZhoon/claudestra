export namespace main {
	
	export class AgentDetailInfo {
	    id: string;
	    role: string;
	    status: string;
	    isConsumer: boolean;
	    instruction: string;
	    output: string;
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
	export class TaskInfo {
	    agentId: string;
	    instruction: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentId = source["agentId"];
	        this.instruction = source["instruction"];
	    }
	}
	export class StepInfo {
	    step: number;
	    title: string;
	    tasks: TaskInfo[];
	
	    static createFrom(source: any = {}) {
	        return new StepInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.step = source["step"];
	        this.title = source["title"];
	        this.tasks = this.convertValues(source["tasks"], TaskInfo);
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
	export class TeamPlanInfo {
	    role: string;
	    description: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new TeamPlanInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.description = source["description"];
	        this.type = source["type"];
	    }
	}
	export class ProposalInfo {
	    userInput: string;
	    teamPlans: TeamPlanInfo[];
	    steps: StepInfo[];
	    contract: string;
	    needTeam: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProposalInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userInput = source["userInput"];
	        this.teamPlans = this.convertValues(source["teamPlans"], TeamPlanInfo);
	        this.steps = this.convertValues(source["steps"], StepInfo);
	        this.contract = source["contract"];
	        this.needTeam = source["needTeam"];
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

