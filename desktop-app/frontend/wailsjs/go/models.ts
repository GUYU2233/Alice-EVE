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
	export class TradeDepthLevel {
	    price: number;
	    volume: number;
	    cumulativeVolume: number;
	    minimumVolume: number;

	    static createFrom(source: any = {}) {
	        return new TradeDepthLevel(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.price = source["price"];
	        this.volume = source["volume"];
	        this.cumulativeVolume = source["cumulativeVolume"];
	        this.minimumVolume = source["minimumVolume"];
	    }
	}
	export class AllTradeCandidate {
	    TypeID: number;
	    SourceRegionID: number;
	    SourceLocationID: number;
	    SourceSystemID: number;
	    DestinationRegionID: number;
	    DestinationLocationID: number;
	    DestinationSystemID: number;
	    BuyPrice: number;
	    SellPrice: number;
	    ItemVolumeM3: number;
	    Quantity: number;
	    Capital: number;
	    GrossProfit: number;
	    Fees: number;
	    NetProfit: number;
	    ProfitRate: number;
	    CargoUsedM3: number;
	    SourceSnapshotAt: string;
	    DestinationSnapshotAt: string;
	    RouteSafetyStatus: string;
	    AskLevels: TradeDepthLevel[];
	    BidLevels: TradeDepthLevel[];

	    static createFrom(source: any = {}) {
	        return new AllTradeCandidate(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TypeID = source["TypeID"];
	        this.SourceRegionID = source["SourceRegionID"];
	        this.SourceLocationID = source["SourceLocationID"];
	        this.SourceSystemID = source["SourceSystemID"];
	        this.DestinationRegionID = source["DestinationRegionID"];
	        this.DestinationLocationID = source["DestinationLocationID"];
	        this.DestinationSystemID = source["DestinationSystemID"];
	        this.BuyPrice = source["BuyPrice"];
	        this.SellPrice = source["SellPrice"];
	        this.ItemVolumeM3 = source["ItemVolumeM3"];
	        this.Quantity = source["Quantity"];
	        this.Capital = source["Capital"];
	        this.GrossProfit = source["GrossProfit"];
	        this.Fees = source["Fees"];
	        this.NetProfit = source["NetProfit"];
	        this.ProfitRate = source["ProfitRate"];
	        this.CargoUsedM3 = source["CargoUsedM3"];
	        this.SourceSnapshotAt = source["SourceSnapshotAt"];
	        this.DestinationSnapshotAt = source["DestinationSnapshotAt"];
	        this.RouteSafetyStatus = source["RouteSafetyStatus"];
	        this.AskLevels = this.convertValues(source["AskLevels"], TradeDepthLevel);
	        this.BidLevels = this.convertValues(source["BidLevels"], TradeDepthLevel);
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
	export class CargoCapacity {
	    m3: number;
	    estimated: boolean;
	    source: string;

	    static createFrom(source: any = {}) {
	        return new CargoCapacity(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.m3 = source["m3"];
	        this.estimated = source["estimated"];
	        this.source = source["source"];
	    }
	}
	export class CharacterSnapshot {
	    AccountID: string;
	    CharacterID: number;
	    Domain: string;
	    Payload: number[];
	    // Go type: time
	    FetchedAt: any;
	    // Go type: time
	    ExpiresAt: any;
	    Stale: boolean;
	    Source: string;
	    ETag: string;

	    static createFrom(source: any = {}) {
	        return new CharacterSnapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.AccountID = source["AccountID"];
	        this.CharacterID = source["CharacterID"];
	        this.Domain = source["Domain"];
	        this.Payload = source["Payload"];
	        this.FetchedAt = this.convertValues(source["FetchedAt"], null);
	        this.ExpiresAt = this.convertValues(source["ExpiresAt"], null);
	        this.Stale = source["Stale"];
	        this.Source = source["Source"];
	        this.ETag = source["ETag"];
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
	export class CharacterTradeContext {
	    characterID: number;
	    solarSystemID: number;
	    stationID?: number;
	    structureID?: number;
	    shipItemID: number;
	    shipTypeID: number;
	    shipName: string;
	    cargo: CargoCapacity;
	    walletISK: number;
	    availableBudgetISK: number;
	    // Go type: time
	    observedAt: any;
	    stale: boolean;

	    static createFrom(source: any = {}) {
	        return new CharacterTradeContext(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.characterID = source["characterID"];
	        this.solarSystemID = source["solarSystemID"];
	        this.stationID = source["stationID"];
	        this.structureID = source["structureID"];
	        this.shipItemID = source["shipItemID"];
	        this.shipTypeID = source["shipTypeID"];
	        this.shipName = source["shipName"];
	        this.cargo = this.convertValues(source["cargo"], CargoCapacity);
	        this.walletISK = source["walletISK"];
	        this.availableBudgetISK = source["availableBudgetISK"];
	        this.observedAt = this.convertValues(source["observedAt"], null);
	        this.stale = source["stale"];
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
	export class ChatIntelEvent {
	    id: string;
	    channel: string;
	    listener?: string;
	    author: string;
	    // Go type: time
	    timestamp: any;
	    findings: protocol.IntelFinding[];

	    static createFrom(source: any = {}) {
	        return new ChatIntelEvent(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.channel = source["channel"];
	        this.listener = source["listener"];
	        this.author = source["author"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.findings = this.convertValues(source["findings"], protocol.IntelFinding);
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
	export class ChatLogStatus {
	    directory: string;
	    running: boolean;
	    files: number;
	    messages: number;
	    findings: number;
	    // Go type: time
	    lastMessage?: any;
	    lastError?: string;

	    static createFrom(source: any = {}) {
	        return new ChatLogStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.directory = source["directory"];
	        this.running = source["running"];
	        this.files = source["files"];
	        this.messages = source["messages"];
	        this.findings = source["findings"];
	        this.lastMessage = this.convertValues(source["lastMessage"], null);
	        this.lastError = source["lastError"];
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
	export class EVEAlliance {
	    creator_corporation_id: number;
	    creator_id: number;
	    // Go type: time
	    date_founded: any;
	    executor_corporation_id: number;
	    faction_id: number;
	    name: string;
	    ticker: string;

	    static createFrom(source: any = {}) {
	        return new EVEAlliance(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.creator_corporation_id = source["creator_corporation_id"];
	        this.creator_id = source["creator_id"];
	        this.date_founded = this.convertValues(source["date_founded"], null);
	        this.executor_corporation_id = source["executor_corporation_id"];
	        this.faction_id = source["faction_id"];
	        this.name = source["name"];
	        this.ticker = source["ticker"];
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
	export class EVECorporation {
	    alliance_id: number;
	    ceo_id: number;
	    creator_id: number;
	    description: string;
	    home_station_id: number;
	    member_count: number;
	    name: string;
	    shares: number;
	    tax_rate: number;
	    ticker: string;
	    url: string;
	    war_eligible: boolean;

	    static createFrom(source: any = {}) {
	        return new EVECorporation(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alliance_id = source["alliance_id"];
	        this.ceo_id = source["ceo_id"];
	        this.creator_id = source["creator_id"];
	        this.description = source["description"];
	        this.home_station_id = source["home_station_id"];
	        this.member_count = source["member_count"];
	        this.name = source["name"];
	        this.shares = source["shares"];
	        this.tax_rate = source["tax_rate"];
	        this.ticker = source["ticker"];
	        this.url = source["url"];
	        this.war_eligible = source["war_eligible"];
	    }
	}
	export class EVEDogmaAttribute {
	    attribute_id: number;
	    value: number;

	    static createFrom(source: any = {}) {
	        return new EVEDogmaAttribute(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.attribute_id = source["attribute_id"];
	        this.value = source["value"];
	    }
	}
	export class EVEDogmaEffect {
	    effect_id: number;
	    is_default: boolean;

	    static createFrom(source: any = {}) {
	        return new EVEDogmaEffect(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.effect_id = source["effect_id"];
	        this.is_default = source["is_default"];
	    }
	}
	export class EVETradeHub {
	    Code: string;
	    Name: string;
	    NameZH: string;
	    RegionID: number;
	    StationID: number;
	    SystemID: number;

	    static createFrom(source: any = {}) {
	        return new EVETradeHub(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Code = source["Code"];
	        this.Name = source["Name"];
	        this.NameZH = source["NameZH"];
	        this.RegionID = source["RegionID"];
	        this.StationID = source["StationID"];
	        this.SystemID = source["SystemID"];
	    }
	}
	export class EVEHubQuote {
	    hub: EVETradeHub;
	    typeId: number;
	    bestBuy: number;
	    bestSell: number;
	    spread: number;
	    spreadPercent: number;
	    buyVolume: number;
	    sellVolume: number;
	    buyOrders: number;
	    sellOrders: number;
	    // Go type: time
	    fetchedAt: any;
	    // Go type: time
	    expiresAt: any;

	    static createFrom(source: any = {}) {
	        return new EVEHubQuote(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hub = this.convertValues(source["hub"], EVETradeHub);
	        this.typeId = source["typeId"];
	        this.bestBuy = source["bestBuy"];
	        this.bestSell = source["bestSell"];
	        this.spread = source["spread"];
	        this.spreadPercent = source["spreadPercent"];
	        this.buyVolume = source["buyVolume"];
	        this.sellVolume = source["sellVolume"];
	        this.buyOrders = source["buyOrders"];
	        this.sellOrders = source["sellOrders"];
	        this.fetchedAt = this.convertValues(source["fetchedAt"], null);
	        this.expiresAt = this.convertValues(source["expiresAt"], null);
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
	export class EVEItemType {
	    capacity: number;
	    description: string;
	    dogma_attributes: EVEDogmaAttribute[];
	    dogma_effects: EVEDogmaEffect[];
	    graphic_id: number;
	    group_id: number;
	    icon_id: number;
	    market_group_id: number;
	    mass: number;
	    name: string;
	    packaged_volume: number;
	    portion_size: number;
	    published: boolean;
	    radius: number;
	    type_id: number;
	    volume: number;

	    static createFrom(source: any = {}) {
	        return new EVEItemType(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capacity = source["capacity"];
	        this.description = source["description"];
	        this.dogma_attributes = this.convertValues(source["dogma_attributes"], EVEDogmaAttribute);
	        this.dogma_effects = this.convertValues(source["dogma_effects"], EVEDogmaEffect);
	        this.graphic_id = source["graphic_id"];
	        this.group_id = source["group_id"];
	        this.icon_id = source["icon_id"];
	        this.market_group_id = source["market_group_id"];
	        this.mass = source["mass"];
	        this.name = source["name"];
	        this.packaged_volume = source["packaged_volume"];
	        this.portion_size = source["portion_size"];
	        this.published = source["published"];
	        this.radius = source["radius"];
	        this.type_id = source["type_id"];
	        this.volume = source["volume"];
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
	export class EVEMarketHistory {
	    average: number;
	    highest: number;
	    lowest: number;
	    date: string;
	    order_count: number;
	    volume: number;

	    static createFrom(source: any = {}) {
	        return new EVEMarketHistory(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.average = source["average"];
	        this.highest = source["highest"];
	        this.lowest = source["lowest"];
	        this.date = source["date"];
	        this.order_count = source["order_count"];
	        this.volume = source["volume"];
	    }
	}
	export class EVEMarketOrder {
	    duration: number;
	    is_buy_order: boolean;
	    // Go type: time
	    issued: any;
	    location_id: number;
	    min_volume: number;
	    order_id: number;
	    price: number;
	    range: string;
	    system_id: number;
	    type_id: number;
	    volume_remain: number;
	    volume_total: number;

	    static createFrom(source: any = {}) {
	        return new EVEMarketOrder(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.duration = source["duration"];
	        this.is_buy_order = source["is_buy_order"];
	        this.issued = this.convertValues(source["issued"], null);
	        this.location_id = source["location_id"];
	        this.min_volume = source["min_volume"];
	        this.order_id = source["order_id"];
	        this.price = source["price"];
	        this.range = source["range"];
	        this.system_id = source["system_id"];
	        this.type_id = source["type_id"];
	        this.volume_remain = source["volume_remain"];
	        this.volume_total = source["volume_total"];
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
	export class EVEMarketPrice {
	    adjusted_price: number;
	    average_price: number;
	    type_id: number;

	    static createFrom(source: any = {}) {
	        return new EVEMarketPrice(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.adjusted_price = source["adjusted_price"];
	        this.average_price = source["average_price"];
	        this.type_id = source["type_id"];
	    }
	}
	export class EVEPosition {
	    x: number;
	    y: number;
	    z: number;

	    static createFrom(source: any = {}) {
	        return new EVEPosition(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.z = source["z"];
	    }
	}
	export class  {
	    planet_id: number;

	    static createFrom(source: any = {}) {
	        return new (source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.planet_id = source["planet_id"];
	    }
	}
	export class EVESolarSystem {
	    constellation_id: number;
	    name: string;
	    planets: [];
	    position: EVEPosition;
	    security_class: string;
	    security_status: number;
	    star_id: number;
	    stargates: number[];
	    stations: number[];
	    system_id: number;

	    static createFrom(source: any = {}) {
	        return new EVESolarSystem(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.constellation_id = source["constellation_id"];
	        this.name = source["name"];
	        this.planets = this.convertValues(source["planets"], );
	        this.position = this.convertValues(source["position"], EVEPosition);
	        this.security_class = source["security_class"];
	        this.security_status = source["security_status"];
	        this.star_id = source["star_id"];
	        this.stargates = source["stargates"];
	        this.stations = source["stations"];
	        this.system_id = source["system_id"];
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
	export class EVEStation {
	    max_dockable_ship_volume: number;
	    name: string;
	    office_rental_cost: number;
	    owner: number;
	    position: EVEPosition;
	    race_id: number;
	    reprocessing_efficiency: number;
	    reprocessing_stations_take: number;
	    services: string[];
	    station_id: number;
	    system_id: number;
	    type_id: number;

	    static createFrom(source: any = {}) {
	        return new EVEStation(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_dockable_ship_volume = source["max_dockable_ship_volume"];
	        this.name = source["name"];
	        this.office_rental_cost = source["office_rental_cost"];
	        this.owner = source["owner"];
	        this.position = this.convertValues(source["position"], EVEPosition);
	        this.race_id = source["race_id"];
	        this.reprocessing_efficiency = source["reprocessing_efficiency"];
	        this.reprocessing_stations_take = source["reprocessing_stations_take"];
	        this.services = source["services"];
	        this.station_id = source["station_id"];
	        this.system_id = source["system_id"];
	        this.type_id = source["type_id"];
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
	export class EVEStatus {
	    players: number;
	    server_version: string;
	    // Go type: time
	    start_time: any;
	    vip: boolean;

	    static createFrom(source: any = {}) {
	        return new EVEStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.players = source["players"];
	        this.server_version = source["server_version"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.vip = source["vip"];
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
	export class EVESyncJob {
	    ID: string;
	    AccountID: string;
	    Kind: string;
	    Status: string;
	    // Go type: time
	    RunAfter: any;
	    Attempts: number;

	    static createFrom(source: any = {}) {
	        return new EVESyncJob(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.AccountID = source["AccountID"];
	        this.Kind = source["Kind"];
	        this.Status = source["Status"];
	        this.RunAfter = this.convertValues(source["RunAfter"], null);
	        this.Attempts = source["Attempts"];
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

	export class EVETradePlanRequest {
	    TypeID: number;
	    Quantity: number;
	    Source: string;
	    Destination: string;
	    SalesTaxRate: number;
	    BrokerRate: number;
	    TransportCostPerM3: number;
	    CargoM3: number;
	    ItemVolumeM3: number;
	    RouteJumps: number;
	    LowSecJumps: number;

	    static createFrom(source: any = {}) {
	        return new EVETradePlanRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TypeID = source["TypeID"];
	        this.Quantity = source["Quantity"];
	        this.Source = source["Source"];
	        this.Destination = source["Destination"];
	        this.SalesTaxRate = source["SalesTaxRate"];
	        this.BrokerRate = source["BrokerRate"];
	        this.TransportCostPerM3 = source["TransportCostPerM3"];
	        this.CargoM3 = source["CargoM3"];
	        this.ItemVolumeM3 = source["ItemVolumeM3"];
	        this.RouteJumps = source["RouteJumps"];
	        this.LowSecJumps = source["LowSecJumps"];
	    }
	}
	export class EVEUniverseName {
	    id: number;
	    name: string;
	    category: string;

	    static createFrom(source: any = {}) {
	        return new EVEUniverseName(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.category = source["category"];
	    }
	}
	export class GameLogEvent {
	    ID: string;
	    Type: string;
	    Action: string;
	    Target: string;
	    Summary: string;
	    // Go type: time
	    Timestamp: any;
	    Severity: string;

	    static createFrom(source: any = {}) {
	        return new GameLogEvent(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Type = source["Type"];
	        this.Action = source["Action"];
	        this.Target = source["Target"];
	        this.Summary = source["Summary"];
	        this.Timestamp = this.convertValues(source["Timestamp"], null);
	        this.Severity = source["Severity"];
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
	export class GameLogStatus {
	    directory: string;
	    running: boolean;
	    files: number;
	    events: number;
	    // Go type: time
	    lastEvent?: any;
	    lastError?: string;

	    static createFrom(source: any = {}) {
	        return new GameLogStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.directory = source["directory"];
	        this.running = source["running"];
	        this.files = source["files"];
	        this.events = source["events"];
	        this.lastEvent = this.convertValues(source["lastEvent"], null);
	        this.lastError = source["lastError"];
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
	export class TradePriceLevel {
	    price: number;
	    volume: number;
	    minimumVolume?: number;

	    static createFrom(source: any = {}) {
	        return new TradePriceLevel(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.price = source["price"];
	        this.volume = source["volume"];
	        this.minimumVolume = source["minimumVolume"];
	    }
	}
	export class TradeCandidate {
	    TypeID: number;
	    From: EVETradeHub;
	    To: EVETradeHub;
	    MaxQuantity: number;
	    UnitVolume: number;
	    UnitCost: number;
	    UnitReturn: number;
	    NetPerUnit: number;
	    Score: number;
	    Confidence: number;
	    AskLevels: TradePriceLevel[];
	    BidLevels: TradePriceLevel[];
	    SalesTaxRate: number;
	    BrokerRate: number;
	    Jumps: number;
	    MinSecurity: number;

	    static createFrom(source: any = {}) {
	        return new TradeCandidate(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TypeID = source["TypeID"];
	        this.From = this.convertValues(source["From"], EVETradeHub);
	        this.To = this.convertValues(source["To"], EVETradeHub);
	        this.MaxQuantity = source["MaxQuantity"];
	        this.UnitVolume = source["UnitVolume"];
	        this.UnitCost = source["UnitCost"];
	        this.UnitReturn = source["UnitReturn"];
	        this.NetPerUnit = source["NetPerUnit"];
	        this.Score = source["Score"];
	        this.Confidence = source["Confidence"];
	        this.AskLevels = this.convertValues(source["AskLevels"], TradePriceLevel);
	        this.BidLevels = this.convertValues(source["BidLevels"], TradePriceLevel);
	        this.SalesTaxRate = source["SalesTaxRate"];
	        this.BrokerRate = source["BrokerRate"];
	        this.Jumps = source["Jumps"];
	        this.MinSecurity = source["MinSecurity"];
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
	export class TradeCandidatePage {
	    items: AllTradeCandidate[];
	    Limit: number;
	    Offset: number;
	    hasMore: boolean;

	    static createFrom(source: any = {}) {
	        return new TradeCandidatePage(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], AllTradeCandidate);
	        this.Limit = source["Limit"];
	        this.Offset = source["Offset"];
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
	export class TradeCandidateSearch {
	    regionIds: number[];
	    budget: number;
	    cargoM3: number;
	    salesTaxRate: number;
	    brokerRate: number;
	    minProfit: number;
	    minProfitRate: number;
	    minSecurity: number;
	    maxJumps: number;
	    includeDepth: boolean;
	    limit: number;
	    offset: number;

	    static createFrom(source: any = {}) {
	        return new TradeCandidateSearch(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.regionIds = source["regionIds"];
	        this.budget = source["budget"];
	        this.cargoM3 = source["cargoM3"];
	        this.salesTaxRate = source["salesTaxRate"];
	        this.brokerRate = source["brokerRate"];
	        this.minProfit = source["minProfit"];
	        this.minProfitRate = source["minProfitRate"];
	        this.minSecurity = source["minSecurity"];
	        this.maxJumps = source["maxJumps"];
	        this.includeDepth = source["includeDepth"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class TradeConstraint {
	    budget: number;
	    budgetReserve: number;
	    cargoM3: number;
	    maxItemConcentration: number;
	    minSecurity: number;
	    maxStops: number;
	    maxLegs: number;
	    beamWidth: number;
	    maxItemsPerLeg: number;
	    transportCostPerM3: number;
	    jumpPenalty: number;
	    stopPenalty: number;
	    optimizationStates: number;

	    static createFrom(source: any = {}) {
	        return new TradeConstraint(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.budget = source["budget"];
	        this.budgetReserve = source["budgetReserve"];
	        this.cargoM3 = source["cargoM3"];
	        this.maxItemConcentration = source["maxItemConcentration"];
	        this.minSecurity = source["minSecurity"];
	        this.maxStops = source["maxStops"];
	        this.maxLegs = source["maxLegs"];
	        this.beamWidth = source["beamWidth"];
	        this.maxItemsPerLeg = source["maxItemsPerLeg"];
	        this.transportCostPerM3 = source["transportCostPerM3"];
	        this.jumpPenalty = source["jumpPenalty"];
	        this.stopPenalty = source["stopPenalty"];
	        this.optimizationStates = source["optimizationStates"];
	    }
	}
	export class TradeChainRequest {
	    start: EVETradeHub;
	    routes: any[];
	    candidates: TradeCandidate[];
	    snapshot: Record<string, any>;
	    constraint: TradeConstraint;

	    static createFrom(source: any = {}) {
	        return new TradeChainRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = this.convertValues(source["start"], EVETradeHub);
	        this.routes = source["routes"];
	        this.candidates = this.convertValues(source["candidates"], TradeCandidate);
	        this.snapshot = source["snapshot"];
	        this.constraint = this.convertValues(source["constraint"], TradeConstraint);
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


	export class TradePackRequest {
	    candidates: TradeCandidate[];
	    constraint: TradeConstraint;
	    limit?: number;

	    static createFrom(source: any = {}) {
	        return new TradePackRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.candidates = this.convertValues(source["candidates"], TradeCandidate);
	        this.constraint = this.convertValues(source["constraint"], TradeConstraint);
	        this.limit = source["limit"];
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

	export class TradeRegionBatch {
	    ID: string;
	    RegionID: number;
	    State: string;
	    ExpectedPages: number;
	    CollectedPages: number;
	    OrderCount: number;
	    // Go type: time
	    StartedAt: any;
	    // Go type: time
	    CompletedAt: any;
	    // Go type: time
	    ExpiresAt: any;
	    ETag: string;
	    Failure?: string;

	    static createFrom(source: any = {}) {
	        return new TradeRegionBatch(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.RegionID = source["RegionID"];
	        this.State = source["State"];
	        this.ExpectedPages = source["ExpectedPages"];
	        this.CollectedPages = source["CollectedPages"];
	        this.OrderCount = source["OrderCount"];
	        this.StartedAt = this.convertValues(source["StartedAt"], null);
	        this.CompletedAt = this.convertValues(source["CompletedAt"], null);
	        this.ExpiresAt = this.convertValues(source["ExpiresAt"], null);
	        this.ETag = source["ETag"];
	        this.Failure = source["Failure"];
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
	export class TradeRegionJob {
	    ID: string;
	    RegionID: number;
	    State: string;
	    Priority: number;
	    Trigger: string;
	    // Go type: time
	    RequestedAt: any;
	    // Go type: time
	    ClaimedAt: any;
	    // Go type: time
	    ClaimExpiresAt: any;
	    // Go type: time
	    StartedAt: any;
	    // Go type: time
	    CompletedAt: any;
	    Attempt: number;
	    FailureCode?: string;
	    FailureDetail?: string;

	    static createFrom(source: any = {}) {
	        return new TradeRegionJob(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.RegionID = source["RegionID"];
	        this.State = source["State"];
	        this.Priority = source["Priority"];
	        this.Trigger = source["Trigger"];
	        this.RequestedAt = this.convertValues(source["RequestedAt"], null);
	        this.ClaimedAt = this.convertValues(source["ClaimedAt"], null);
	        this.ClaimExpiresAt = this.convertValues(source["ClaimExpiresAt"], null);
	        this.StartedAt = this.convertValues(source["StartedAt"], null);
	        this.CompletedAt = this.convertValues(source["CompletedAt"], null);
	        this.Attempt = source["Attempt"];
	        this.FailureCode = source["FailureCode"];
	        this.FailureDetail = source["FailureDetail"];
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
	export class TradeRegionSchedule {
	    RegionID: number;
	    Enabled: boolean;
	    Tier: string;
	    Priority: number;
	    RefreshInterval: number;
	    MinRefreshInterval: number;
	    MaxStaleness: number;
	    // Go type: time
	    LastRequestedAt: any;
	    // Go type: time
	    NextRunAt: any;
	    // Go type: time
	    LastSuccessAt: any;
	    ConsecutiveFailures: number;
	    // Go type: time
	    BackoffUntil: any;
	    AccessScore: number;
	    ChangeScore: number;

	    static createFrom(source: any = {}) {
	        return new TradeRegionSchedule(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.RegionID = source["RegionID"];
	        this.Enabled = source["Enabled"];
	        this.Tier = source["Tier"];
	        this.Priority = source["Priority"];
	        this.RefreshInterval = source["RefreshInterval"];
	        this.MinRefreshInterval = source["MinRefreshInterval"];
	        this.MaxStaleness = source["MaxStaleness"];
	        this.LastRequestedAt = this.convertValues(source["LastRequestedAt"], null);
	        this.NextRunAt = this.convertValues(source["NextRunAt"], null);
	        this.LastSuccessAt = this.convertValues(source["LastSuccessAt"], null);
	        this.ConsecutiveFailures = source["ConsecutiveFailures"];
	        this.BackoffUntil = this.convertValues(source["BackoffUntil"], null);
	        this.AccessScore = source["AccessScore"];
	        this.ChangeScore = source["ChangeScore"];
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
	export class TradeRegionSnapshot {
	    RegionID: number;
	    Current: TradeRegionBatch;
	    Previous?: TradeRegionBatch;

	    static createFrom(source: any = {}) {
	        return new TradeRegionSnapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.RegionID = source["RegionID"];
	        this.Current = this.convertValues(source["Current"], TradeRegionBatch);
	        this.Previous = this.convertValues(source["Previous"], TradeRegionBatch);
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
	export class TradeRegionStatus {
	    regionId: number;
	    state?: string;
	    // Go type: time
	    startedAt?: any;
	    // Go type: time
	    endedAt?: any;
	    snapshot?: TradeRegionSnapshot;
	    error?: string;
	    schedule?: TradeRegionSchedule;
	    job?: TradeRegionJob;

	    static createFrom(source: any = {}) {
	        return new TradeRegionStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.regionId = source["regionId"];
	        this.state = source["state"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.endedAt = this.convertValues(source["endedAt"], null);
	        this.snapshot = this.convertValues(source["snapshot"], TradeRegionSnapshot);
	        this.error = source["error"];
	        this.schedule = this.convertValues(source["schedule"], TradeRegionSchedule);
	        this.job = this.convertValues(source["job"], TradeRegionJob);
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
	export class TradeRouteQuery {
	    From: number;
	    To: number;
	    MinSecurity: number;
	    MaxJumps: number;

	    static createFrom(source: any = {}) {
	        return new TradeRouteQuery(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.From = source["From"];
	        this.To = source["To"];
	        this.MinSecurity = source["MinSecurity"];
	        this.MaxJumps = source["MaxJumps"];
	    }
	}
	export class TradeRouteSystem {
	    systemId: number;
	    name: string;
	    securityStatus: number;

	    static createFrom(source: any = {}) {
	        return new TradeRouteSystem(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.systemId = source["systemId"];
	        this.name = source["name"];
	        this.securityStatus = source["securityStatus"];
	    }
	}
	export class TradeRouteResult {
	    status: string;
	    fromSystemId: number;
	    toSystemId: number;
	    jumps: number;
	    minSecurity: number;
	    systems: TradeRouteSystem[];

	    static createFrom(source: any = {}) {
	        return new TradeRouteResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.fromSystemId = source["fromSystemId"];
	        this.toSystemId = source["toSystemId"];
	        this.jumps = source["jumps"];
	        this.minSecurity = source["minSecurity"];
	        this.systems = this.convertValues(source["systems"], TradeRouteSystem);
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

export namespace evesde {

	export class System {
	    ID: number;
	    ConstellationID: number;
	    Name: string;
	    Security: number;

	    static createFrom(source: any = {}) {
	        return new System(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.ConstellationID = source["ConstellationID"];
	        this.Name = source["Name"];
	        this.Security = source["Security"];
	    }
	}
	export class RouteResult {
	    systems: System[];
	    jumps: number;
	    minSecurity: number;
	    lowSecCount: number;
	    nullSecCount: number;

	    static createFrom(source: any = {}) {
	        return new RouteResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.systems = this.convertValues(source["systems"], System);
	        this.jumps = source["jumps"];
	        this.minSecurity = source["minSecurity"];
	        this.lowSecCount = source["lowSecCount"];
	        this.nullSecCount = source["nullSecCount"];
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

	export class NormalizedLocalEvent {
	    ID: string;
	    SourceKind: string;
	    SourcePath: string;
	    FileIdentity: string;
	    RecordStart: number;
	    RecordEnd: number;
	    EventType: string;
	    ObservedMS: number;
	    PayloadJSON: string;
	    CreatedMS: number;

	    static createFrom(source: any = {}) {
	        return new NormalizedLocalEvent(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.SourceKind = source["SourceKind"];
	        this.SourcePath = source["SourcePath"];
	        this.FileIdentity = source["FileIdentity"];
	        this.RecordStart = source["RecordStart"];
	        this.RecordEnd = source["RecordEnd"];
	        this.EventType = source["EventType"];
	        this.ObservedMS = source["ObservedMS"];
	        this.PayloadJSON = source["PayloadJSON"];
	        this.CreatedMS = source["CreatedMS"];
	    }
	}
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
