import PersistableMap from "./lib/persistableMap";

export interface Secret {
  key: string;
  value: string;
}

export const secrets = new PersistableMap<Secret["key"], Secret>(
  "/var/jig/secrets.json"
);
