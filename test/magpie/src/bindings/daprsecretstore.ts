import { Binding, BindingStatus} from '../binding'
import { DaprClient } from '@roadwork/dapr-js-sdk/http'

// Use this with a values like:
// - BINDING_DAPRPUBSUB_NAME
// - BINDING_DAPRPUBSUB_TOPIC
export class DaprSecretStoreBinding implements Binding {
    private name: string;
    private topic: string;
    private client: DaprClient;

    constructor(map: { [key: string]: string }) {

        this.name = map['SECRETSTORENAME'];
        if (!this.name) {
            throw new Error('SECRETSTORENAME is  required');
        }
        // This is safe to construct. It doesn't hit the network until you use it.
        this.client = new DaprClient('localhost', process.env.DAPR_HTTP_PORT)
    }

    public async status(): Promise<BindingStatus> {

        const res =  await this.client.secret.get(this.name, 'SOME_SECRET');
        return { ok: true, message: "secrets accessed"};
    }

    public toString = () : string => {
        return 'Dapr SecretStore';
    }
}