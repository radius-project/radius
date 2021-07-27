export interface Binding {
    status(): Promise<BindingStatus>;
}

export interface BindingStatus {
    ok: boolean;
    message: string;
}

export type BindingProvider = (map: { [key: string]: string }) => Binding

export function loadBindings(env: any, providers: { [type: string]: BindingProvider }): Binding[] {
    // We match env-vars using the form BINDING_<KIND>_VALUE, so group them by that structure.
    // Each binding type get a collection of key-value pairs
    let valuesByBinding: { [type: string]: { [key: string]: string }} = {};
    Object.entries(env).forEach(entry => {
        let name = entry[0];
        let value = entry[1] as string;

        let parsed = parseEnvVar(name);
        if (!parsed) {
            return
        }
        
        let { type, key } = parsed;
        let values = valuesByBinding[type];
        if (!values) {
            values = {};
            valuesByBinding[type] = values;
        }

        values[key] = value;
    });

    // Now that we've got all the values grouped by type, we can walk that list and instantiate
    // all the bindings.
    let bindings: Binding[] = [];
    Object.entries(valuesByBinding).forEach(entry => {
        let provider = providers[entry[0]]
        if (!provider) {
            throw new Error(`no provider could be found for binding of type ${entry[0]}`)
        }

        let binding = provider(entry[1])
        bindings.push(binding)
    });

    return bindings;
}

function parseEnvVar(name: string): { type: string; key: string; } | null {
    if (!name.startsWith('BINDING_')) {
        return null
    }

    let parts = name.split('_')
    if (parts.length != 3) {
        return null
    }

    return { type: parts[1].toUpperCase(), key: parts[2].toUpperCase(), };
}