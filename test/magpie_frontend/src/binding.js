"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.loadBindings = void 0;
function loadBindings(env, providers) {
    // We match env-vars using the form BINDING_<KIND>_VALUE, so group them by that structure.
    // Each binding type get a collection of key-value pairs
    var valuesByBinding = {};
    Object.entries(env).forEach(function (entry) {
        var name = entry[0];
        var value = entry[1];
        var parsed = parseEnvVar(name);
        if (!parsed) {
            return;
        }
        var type = parsed.type, key = parsed.key;
        var values = valuesByBinding[type];
        if (!values) {
            values = {};
            valuesByBinding[type] = values;
        }
        values[key] = value;
    });
    // Now that we've got all the values grouped by type, we can walk that list and instantiate
    // all the bindings.
    var bindings = [];
    Object.entries(valuesByBinding).forEach(function (entry) {
        var provider = providers[entry[0]];
        if (!provider) {
            throw new Error("no provider could be found for binding of type ".concat(entry[0]));
        }
        var binding = provider(entry[1]);
        bindings.push(binding);
    });
    return bindings;
}
exports.loadBindings = loadBindings;
function parseEnvVar(name) {
    if (!name.startsWith('BINDING_') && !name.startsWith('CONNECTION_')) {
        return null;
    }
    var parts = name.split('_');
    if (parts.length != 3) {
        return null;
    }
    return { type: parts[1].toUpperCase(), key: parts[2].toUpperCase(), };
}
