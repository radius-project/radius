import { Binding, BindingStatus} from '../binding'
import { DaprClient } from '@roadwork/dapr-js-sdk/http'

// Use this with a values like:
// - BINDING_DAPRSECRETSTORE_SECRETSTORENAME
export class DaprSecretStoreBinding implements Binding {
    private name: string;
    private client: DaprClient;

    constructor(map: { [key: string]: string }) {
        this.name = map['NAME'];
        if (!this.name) {
            throw new Error('NAME is required');
        }

        // This is safe to construct. It doesn't hit the network until you use it.
        this.client = new DaprClient('localhost', process.env.DAPR_HTTP_PORT)
    }

    public async status(): Promise<BindingStatus> {
        // Do a round-trip save get.
        await this.client.state.save(this.name, [{ key:"key", value: "value"}]);

        const res = await this.client.state.get(this.name, "key");
        return { ok: res == "value", message: "message sent"};
    }

    public toString = () : string => {
        return 'Dapr SecretStore';
    }
}