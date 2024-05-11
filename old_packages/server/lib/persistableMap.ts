import { readFileSync, writeFile, existsSync } from "node:fs";

class PersistableMap<K, V> extends Map<K, V> {
  log(...args: any[]) {
    console.log(`[PM(${this.filename})]:`, ...args);
  }

  constructor(private filename: string) {
    super();
    try {
      if (existsSync(filename)) {
        const data = readFileSync(filename).toString("utf-8");
        for (const [k, val] of JSON.parse(data) as Array<[K, V]>)
          this.set(k, val);
        this.log("Successfully initialized persistable map");
      } else {
        this.log("Initiated new map");
      }
    } catch (error) {
      this.log("Failed to initialize persistable map: ", error);
    }
  }

  set(key: any, value: any): this {
    super.set(key, value);
    this.persist().then(() => this.log(`Persisted key ${key} successfully`));
    return this;
  }

  delete(key: any): boolean {
    const _res = super.delete(key);
    if (_res)
      this.persist().then(() => this.log(`Deleted key ${key} successfully`));
    return _res;
  }

  persist() {
    return new Promise((res, rej) => {
      writeFile(this.filename, JSON.stringify(Array.from(this.entries())), res);
    });
  }
}

export default PersistableMap;
