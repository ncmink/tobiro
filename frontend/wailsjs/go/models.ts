export namespace domain {
	
	export class AuthMethodInfo {
	    id: string;
	    displayName: string;
	    needsSecret: boolean;
	    implemented: boolean;
	    description: string;
	    kinds: string[];
	
	    static createFrom(source: any = {}) {
	        return new AuthMethodInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.displayName = source["displayName"];
	        this.needsSecret = source["needsSecret"];
	        this.implemented = source["implemented"];
	        this.description = source["description"];
	        this.kinds = source["kinds"];
	    }
	}
	export class Capabilities {
	    installed: boolean;
	    cliAvailable: boolean;
	    canConnect: boolean;
	    canDisconnect: boolean;
	    canReadStatus: boolean;
	    supportsPassword: boolean;
	    supportsOtp: boolean;
	    requiresNativeUi: boolean;
	    accountOptional: boolean;
	    canOpenNativeClient: boolean;
	    canFillNativeLogin: boolean;
	    nativeClientLabel: string;
	    canControlStartup: boolean;
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new Capabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.cliAvailable = source["cliAvailable"];
	        this.canConnect = source["canConnect"];
	        this.canDisconnect = source["canDisconnect"];
	        this.canReadStatus = source["canReadStatus"];
	        this.supportsPassword = source["supportsPassword"];
	        this.supportsOtp = source["supportsOtp"];
	        this.requiresNativeUi = source["requiresNativeUi"];
	        this.accountOptional = source["accountOptional"];
	        this.canOpenNativeClient = source["canOpenNativeClient"];
	        this.canFillNativeLogin = source["canFillNativeLogin"];
	        this.nativeClientLabel = source["nativeClientLabel"];
	        this.canControlStartup = source["canControlStartup"];
	        this.notes = source["notes"];
	    }
	}
	export class ClientInfo {
	    provider: string;
	    installed: boolean;
	    version: string;
	    path: string;
	    cliPath: string;
	    platform: string;
	    detectedAt: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new ClientInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.installed = source["installed"];
	        this.version = source["version"];
	        this.path = source["path"];
	        this.cliPath = source["cliPath"];
	        this.platform = source["platform"];
	        this.detectedAt = source["detectedAt"];
	        this.detail = source["detail"];
	    }
	}
	export class CredentialInput {
	    label: string;
	    username: string;
	    authMethod: string;
	    isDefault: boolean;
	    privateKeyPath: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new CredentialInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.username = source["username"];
	        this.authMethod = source["authMethod"];
	        this.isDefault = source["isDefault"];
	        this.privateKeyPath = source["privateKeyPath"];
	        this.password = source["password"];
	    }
	}
	export class CredentialView {
	    id: string;
	    profileId: string;
	    label: string;
	    username: string;
	    secretRef: string;
	    authMethod: string;
	    isDefault: boolean;
	    hasPassword: boolean;
	    privateKeyPath?: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new CredentialView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.profileId = source["profileId"];
	        this.label = source["label"];
	        this.username = source["username"];
	        this.secretRef = source["secretRef"];
	        this.authMethod = source["authMethod"];
	        this.isDefault = source["isDefault"];
	        this.hasPassword = source["hasPassword"];
	        this.privateKeyPath = source["privateKeyPath"];
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
	export class GroupView {
	    name: string;
	    label: string;
	    hidden: boolean;
	    connections: number;
	
	    static createFrom(source: any = {}) {
	        return new GroupView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.hidden = source["hidden"];
	        this.connections = source["connections"];
	    }
	}
	export class NetworkDiagnosis {
	    known: boolean;
	    capturesAllTraffic: boolean;
	    summary: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new NetworkDiagnosis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.known = source["known"];
	        this.capturesAllTraffic = source["capturesAllTraffic"];
	        this.summary = source["summary"];
	        this.detail = source["detail"];
	    }
	}
	export class PortForward {
	    name: string;
	    localPort: number;
	    remoteHost: string;
	    remotePort: number;
	    state?: string;
	    active?: number;
	    lastError?: string;
	    remoteOk?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PortForward(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.localPort = source["localPort"];
	        this.remoteHost = source["remoteHost"];
	        this.remotePort = source["remotePort"];
	        this.state = source["state"];
	        this.active = source["active"];
	        this.lastError = source["lastError"];
	        this.remoteOk = source["remoteOk"];
	    }
	}
	export class SSHSettings {
	    user: string;
	    forwards: PortForward[];
	
	    static createFrom(source: any = {}) {
	        return new SSHSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = source["user"];
	        this.forwards = this.convertValues(source["forwards"], PortForward);
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
	export class ProfileInput {
	    name: string;
	    kind: string;
	    provider: string;
	    group: string;
	    host: string;
	    port: number;
	    vendorProfile: string;
	    clientPath: string;
	    notes: string;
	    requires: string;
	    ssh: SSHSettings;
	
	    static createFrom(source: any = {}) {
	        return new ProfileInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.provider = source["provider"];
	        this.group = source["group"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.vendorProfile = source["vendorProfile"];
	        this.clientPath = source["clientPath"];
	        this.notes = source["notes"];
	        this.requires = source["requires"];
	        this.ssh = this.convertValues(source["ssh"], SSHSettings);
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
	export class VendorProfileField {
	    label: string;
	    hint: string;
	    required: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VendorProfileField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.hint = source["hint"];
	        this.required = source["required"];
	    }
	}
	export class ProviderInfo {
	    id: string;
	    displayName: string;
	    vendor: string;
	    defaultPort: number;
	    requiresEndpoint: boolean;
	    endpointNote: string;
	    vendorProfileField: VendorProfileField;
	
	    static createFrom(source: any = {}) {
	        return new ProviderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.displayName = source["displayName"];
	        this.vendor = source["vendor"];
	        this.defaultPort = source["defaultPort"];
	        this.requiresEndpoint = source["requiresEndpoint"];
	        this.endpointNote = source["endpointNote"];
	        this.vendorProfileField = this.convertValues(source["vendorProfileField"], VendorProfileField);
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
	export class ProviderStatusView {
	    provider: string;
	    displayName: string;
	    client: ClientInfo;
	    capabilities: Capabilities;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderStatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.displayName = source["displayName"];
	        this.client = this.convertValues(source["client"], ClientInfo);
	        this.capabilities = this.convertValues(source["capabilities"], Capabilities);
	        this.error = source["error"];
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
	export class SSHCommand {
	    profile: ProfileInput;
	    privateKeyPath: string;
	
	    static createFrom(source: any = {}) {
	        return new SSHCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = this.convertValues(source["profile"], ProfileInput);
	        this.privateKeyPath = source["privateKeyPath"];
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
	
	export class SessionView {
	    id: string;
	    profileId: string;
	    credentialId: string;
	    state: string;
	    // Go type: time
	    startedAt: any;
	    // Go type: time
	    connectedAt?: any;
	    // Go type: time
	    endedAt?: any;
	    assignedIp: string;
	    interface: string;
	    identity: string;
	    forwards: PortForward[];
	    error: string;
	    durationSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.profileId = source["profileId"];
	        this.credentialId = source["credentialId"];
	        this.state = source["state"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.connectedAt = this.convertValues(source["connectedAt"], null);
	        this.endedAt = this.convertValues(source["endedAt"], null);
	        this.assignedIp = source["assignedIp"];
	        this.interface = source["interface"];
	        this.identity = source["identity"];
	        this.forwards = this.convertValues(source["forwards"], PortForward);
	        this.error = source["error"];
	        this.durationSeconds = source["durationSeconds"];
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
	export class StartupState {
	    known: boolean;
	    enabled: boolean;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new StartupState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.known = source["known"];
	        this.enabled = source["enabled"];
	        this.detail = source["detail"];
	    }
	}
	export class TunnelInput {
	    host: string;
	    port: number;
	    ssh: SSHSettings;
	
	    static createFrom(source: any = {}) {
	        return new TunnelInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.ssh = this.convertValues(source["ssh"], SSHSettings);
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
	export class VPNProfile {
	    id: string;
	    name: string;
	    kind: string;
	    provider: string;
	    group: string;
	    requires?: string;
	    host: string;
	    port: number;
	    vendorProfile: string;
	    clientPath: string;
	    credentialIds: string[];
	    ssh: SSHSettings;
	    notes: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new VPNProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.provider = source["provider"];
	        this.group = source["group"];
	        this.requires = source["requires"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.vendorProfile = source["vendorProfile"];
	        this.clientPath = source["clientPath"];
	        this.credentialIds = source["credentialIds"];
	        this.ssh = this.convertValues(source["ssh"], SSHSettings);
	        this.notes = source["notes"];
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
	
	export class VendorProfileOption {
	    name: string;
	    detail: string;
	    state: string;
	    connected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VendorProfileOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.detail = source["detail"];
	        this.state = source["state"];
	        this.connected = source["connected"];
	    }
	}

}

export namespace logging {
	
	export class Entry {
	    // Go type: time
	    time: any;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.level = source["level"];
	        this.message = source["message"];
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
	
	export class BootstrapView {
	    providers: domain.ProviderInfo[];
	    authMethods: domain.AuthMethodInfo[];
	    secretBackend: string;
	    secretAvailable: boolean;
	    secretStatusText: string;
	    dataPath: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new BootstrapView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providers = this.convertValues(source["providers"], domain.ProviderInfo);
	        this.authMethods = this.convertValues(source["authMethods"], domain.AuthMethodInfo);
	        this.secretBackend = source["secretBackend"];
	        this.secretAvailable = source["secretAvailable"];
	        this.secretStatusText = source["secretStatusText"];
	        this.dataPath = source["dataPath"];
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
	export class ConnectRequestView {
	    profileId: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectRequestView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	    }
	}
	export class RequirementView {
	    profileId: string;
	    name: string;
	    state: string;
	
	    static createFrom(source: any = {}) {
	        return new RequirementView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.name = source["name"];
	        this.state = source["state"];
	    }
	}
	export class ConnectionView {
	    profile: domain.VPNProfile;
	    providerName: string;
	    credentials: domain.CredentialView[];
	    endpoint: string;
	    managesOwnAccount: boolean;
	    state: string;
	    session?: domain.SessionView;
	    capabilities: domain.Capabilities;
	    client: domain.ClientInfo;
	    hidden: boolean;
	    requires?: RequirementView;
	    suggestedCredentialId: string;
	    lastCredentialId: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = this.convertValues(source["profile"], domain.VPNProfile);
	        this.providerName = source["providerName"];
	        this.credentials = this.convertValues(source["credentials"], domain.CredentialView);
	        this.endpoint = source["endpoint"];
	        this.managesOwnAccount = source["managesOwnAccount"];
	        this.state = source["state"];
	        this.session = this.convertValues(source["session"], domain.SessionView);
	        this.capabilities = this.convertValues(source["capabilities"], domain.Capabilities);
	        this.client = this.convertValues(source["client"], domain.ClientInfo);
	        this.hidden = source["hidden"];
	        this.requires = this.convertValues(source["requires"], RequirementView);
	        this.suggestedCredentialId = source["suggestedCredentialId"];
	        this.lastCredentialId = source["lastCredentialId"];
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
	export class ExportResultView {
	    file: string;
	    connections: number;
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ExportResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.connections = source["connections"];
	        this.cancelled = source["cancelled"];
	    }
	}
	export class FillPermissionsView {
	    supported: boolean;
	    accessibilityGranted: boolean;
	    automationHint: string;
	    inputMonitoringHint: string;
	
	    static createFrom(source: any = {}) {
	        return new FillPermissionsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.accessibilityGranted = source["accessibilityGranted"];
	        this.automationHint = source["automationHint"];
	        this.inputMonitoringHint = source["inputMonitoringHint"];
	    }
	}
	export class FillShortcutView {
	    supported: boolean;
	    combo: string;
	    display: string;
	    registered: boolean;
	    registerError: string;
	    introSeen: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FillShortcutView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.supported = source["supported"];
	        this.combo = source["combo"];
	        this.display = source["display"];
	        this.registered = source["registered"];
	        this.registerError = source["registerError"];
	        this.introSeen = source["introSeen"];
	    }
	}
	export class ImportResultView {
	    file: string;
	    connections: number;
	    accounts: number;
	    skipped: string[];
	    cancelled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ImportResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.connections = source["connections"];
	        this.accounts = source["accounts"];
	        this.skipped = source["skipped"];
	        this.cancelled = source["cancelled"];
	    }
	}
	export class OpenKubeInput {
	    profileId: string;
	    clusterId: string;
	    clusterName: string;
	    customerId: string;
	    customerName: string;
	    environmentId?: string;
	    environmentName?: string;
	    environmentKind?: string;
	    server?: string;
	
	    static createFrom(source: any = {}) {
	        return new OpenKubeInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.clusterId = source["clusterId"];
	        this.clusterName = source["clusterName"];
	        this.customerId = source["customerId"];
	        this.customerName = source["customerName"];
	        this.environmentId = source["environmentId"];
	        this.environmentName = source["environmentName"];
	        this.environmentKind = source["environmentKind"];
	        this.server = source["server"];
	    }
	}
	export class OpenKubeResultView {
	    handoffId: string;
	    link?: string;
	    delivered?: boolean;
	    launched?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OpenKubeResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.handoffId = source["handoffId"];
	        this.link = source["link"];
	        this.delivered = source["delivered"];
	        this.launched = source["launched"];
	    }
	}
	
	export class SSHConfigHostView {
	    alias: string;
	    hostName: string;
	    user: string;
	    port: number;
	    forwardCount: number;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SSHConfigHostView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alias = source["alias"];
	        this.hostName = source["hostName"];
	        this.user = source["user"];
	        this.port = source["port"];
	        this.forwardCount = source["forwardCount"];
	        this.exists = source["exists"];
	    }
	}

}

