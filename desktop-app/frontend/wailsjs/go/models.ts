export namespace app {
	
	export class AgentConversation {
	    id: string;
	    accountId: string;
	    targetDeviceId: string;
	    agentKind: string;
	    status: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentConversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.accountId = source["accountId"];
	        this.targetDeviceId = source["targetDeviceId"];
	        this.agentKind = source["agentKind"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class AgentRun {
	    id: string;
	    conversationId: string;
	    inputMessageId: string;
	    status: string;
	    errorCode?: string;
	    startedAt: string;
	    completedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversationId = source["conversationId"];
	        this.inputMessageId = source["inputMessageId"];
	        this.status = source["status"];
	        this.errorCode = source["errorCode"];
	        this.startedAt = source["startedAt"];
	        this.completedAt = source["completedAt"];
	    }
	}
	export class AgentRunEvent {
	    id: string;
	    conversationId: string;
	    runId: string;
	    sequence: number;
	    type: string;
	    payload: number[];
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentRunEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversationId = source["conversationId"];
	        this.runId = source["runId"];
	        this.sequence = source["sequence"];
	        this.type = source["type"];
	        this.payload = source["payload"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class AlertItem {
	    messageId: string;
	    type: string;
	    sender: string;
	    payload: Record<string, any>;
	    cursor: number;
	    createdAt: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new AlertItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messageId = source["messageId"];
	        this.type = source["type"];
	        this.sender = source["sender"];
	        this.payload = source["payload"];
	        this.cursor = source["cursor"];
	        this.createdAt = source["createdAt"];
	        this.status = source["status"];
	    }
	}
	export class ConversationMessagePage {
	    messages: protocol.ConversationMessage[];
	    nextCursor: number;
	    hasMore: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConversationMessagePage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], protocol.ConversationMessage);
	        this.nextCursor = source["nextCursor"];
	        this.hasMore = source["hasMore"];
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
	export class DataPage_eve_assistant_desktop_app_internal_app_AlertItem_ {
	    items: AlertItem[];
	    cursor: number;
	    hasMore: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DataPage_eve_assistant_desktop_app_internal_app_AlertItem_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], AlertItem);
	        this.cursor = source["cursor"];
	        this.hasMore = source["hasMore"];
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
	export class OutboxItem {
	    messageId: string;
	    type: string;
	    sender: string;
	    recipient?: string;
	    payload: any;
	    cursor: number;
	    createdAt: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new OutboxItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messageId = source["messageId"];
	        this.type = source["type"];
	        this.sender = source["sender"];
	        this.recipient = source["recipient"];
	        this.payload = source["payload"];
	        this.cursor = source["cursor"];
	        this.createdAt = source["createdAt"];
	        this.status = source["status"];
	    }
	}
	export class DataPage_eve_assistant_desktop_app_internal_app_OutboxItem_ {
	    items: OutboxItem[];
	    cursor: number;
	    hasMore: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DataPage_eve_assistant_desktop_app_internal_app_OutboxItem_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], OutboxItem);
	        this.cursor = source["cursor"];
	        this.hasMore = source["hasMore"];
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
	
	export class RealtimeEvent {
	    id: string;
	    accountId: string;
	    deviceId: string;
	    type: string;
	    payload: number[];
	    cursor: number;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new RealtimeEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.accountId = source["accountId"];
	        this.deviceId = source["deviceId"];
	        this.type = source["type"];
	        this.payload = source["payload"];
	        this.cursor = source["cursor"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	export class RealtimeStatus {
	    state: string;
	    cursor: number;
	    retryInMs?: number;
	    lastError?: string;
	
	    static createFrom(source: any = {}) {
	        return new RealtimeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.cursor = source["cursor"];
	        this.retryInMs = source["retryInMs"];
	        this.lastError = source["lastError"];
	    }
	}

}

export namespace eve {
	
	export class ESIResponse {
	    status: number;
	    body: number[];
	    // Go type: time
	    expiresAt: any;
	    fromCache: boolean;
	    etag?: string;
	    cacheControl?: string;
	    retryAfter?: number;
	    attempts?: number;
	
	    static createFrom(source: any = {}) {
	        return new ESIResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.body = source["body"];
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
	        this.fromCache = source["fromCache"];
	        this.etag = source["etag"];
	        this.cacheControl = source["cacheControl"];
	        this.retryAfter = source["retryAfter"];
	        this.attempts = source["attempts"];
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
	export class SDEEntry {
	    id: string;
	    name: string;
	    kind?: string;
	    source?: string;
	
	    static createFrom(source: any = {}) {
	        return new SDEEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.source = source["source"];
	    }
	}
	export class SDEStatus {
	    directory: string;
	    // Go type: time
	    indexedAt: any;
	    files: number;
	    entries: number;
	    ready: boolean;
	    version?: string;
	    source?: string;
	    checksum?: string;
	    entriesChecksum?: string;
	    signature?: string;
	    indexVersion: number;
	
	    static createFrom(source: any = {}) {
	        return new SDEStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.directory = source["directory"];
	        this.indexedAt = this.convertValues(source["indexedAt"], null);
	        this.files = source["files"];
	        this.entries = source["entries"];
	        this.ready = source["ready"];
	        this.version = source["version"];
	        this.source = source["source"];
	        this.checksum = source["checksum"];
	        this.entriesChecksum = source["entriesChecksum"];
	        this.signature = source["signature"];
	        this.indexVersion = source["indexVersion"];
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

export namespace protocol {
	
	export class ConversationMessage {
	    id: string;
	    accountId?: string;
	    conversationId: string;
	    senderKind: string;
	    senderDeviceId?: string;
	    clientMessageId?: string;
	    operation?: string;
	    body: string;
	    metadata?: Record<string, any>;
	    status: string;
	    cursor?: number;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new ConversationMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.accountId = source["accountId"];
	        this.conversationId = source["conversationId"];
	        this.senderKind = source["senderKind"];
	        this.senderDeviceId = source["senderDeviceId"];
	        this.clientMessageId = source["clientMessageId"];
	        this.operation = source["operation"];
	        this.body = source["body"];
	        this.metadata = source["metadata"];
	        this.status = source["status"];
	        this.cursor = source["cursor"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	export class IntelFinding {
	    kind: string;
	    value: string;
	    stance: string;
	    source: string;
	    summary: string;
	    // Go type: time
	    observedAt: any;
	    // Go type: time
	    expiresAt: any;
	    confidence: number;
	
	    static createFrom(source: any = {}) {
	        return new IntelFinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.value = source["value"];
	        this.stance = source["stance"];
	        this.source = source["source"];
	        this.summary = source["summary"];
	        this.observedAt = this.convertValues(source["observedAt"], null);
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
	        this.confidence = source["confidence"];
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

export namespace storage {
	
	export class PairingState {
	    relay_url: string;
	    device_id: string;
	    // Go type: time
	    paired_at: any;
	    version: number;
	
	    static createFrom(source: any = {}) {
	        return new PairingState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relay_url = source["relay_url"];
	        this.device_id = source["device_id"];
	        this.paired_at = this.convertValues(source["paired_at"], null);
	        this.version = source["version"];
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

