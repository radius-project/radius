import { Binding, BindingStatus} from '../binding'
import { DefaultAzureCredential } from '@azure/identity'
import { SecretClient } from '@azure/keyvault-secrets'

// Use this with a values like: BINDING_KEYVAULT_URI
export class KeyVaultBinding implements Binding {
    private uri: string;
    private client: SecretClient;

    constructor(map: { [key: string]: string }) {
        this.uri = map['URI'];
        if (!this.uri) {
            throw new Error('URI is required');
        }

        // This is safe to construct. It doesn't hit the network until you use it.
        this.client = new SecretClient(this.uri, new DefaultAzureCredential());
    }

    public async status(): Promise<BindingStatus> {
        // nothing complex, we just read some of the secret metadata.
        for await (let properties of this.client.listPropertiesOfSecrets()) {
        }
        return { ok: true, message: "secrets accessed"};
    }

    public toString = () : string => {
        return 'Azure KeyVault';
    }
}