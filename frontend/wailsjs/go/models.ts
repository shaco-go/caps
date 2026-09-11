export namespace main {
	
	export class State {
	    enabled: boolean;
	    running: boolean;
	    autostart: boolean;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.running = source["running"];
	        this.autostart = source["autostart"];
	    }
	}

}

export namespace model {
	
	export class Mapping {
	    source: string;
	    mode: string;
	    key: string;
	    ctrl: boolean;
	    alt: boolean;
	    shift: boolean;
	    win: boolean;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new Mapping(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.mode = source["mode"];
	        this.key = source["key"];
	        this.ctrl = source["ctrl"];
	        this.alt = source["alt"];
	        this.shift = source["shift"];
	        this.win = source["win"];
	        this.text = source["text"];
	    }
	}
	export class Settings {
	    enabled: boolean;
	    threshold: number;
	    shortPress: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.threshold = source["threshold"];
	        this.shortPress = source["shortPress"];
	    }
	}
	export class Config {
	    settings: Settings;
	    mappings: Mapping[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = this.convertValues(source["settings"], Settings);
	        this.mappings = this.convertValues(source["mappings"], Mapping);
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

