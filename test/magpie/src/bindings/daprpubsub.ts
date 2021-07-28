import { Binding, BindingStatus} from '../binding'
import { DaprClient } from '@roadwork/dapr-js-sdk/http'

// Use this with a values like:
// - BINDING_DAPRPUBSUB_NAME
// - BINDING_DAPRPUBSUB_TOPIC
export class DaprPubSubBinding implements Binding {
    private name: string;
    private topic: string;
    private client: DaprClient;

    constructor(map: { [key: string]: string }) {
        this.name = map['NAME'];
        if (!this.name) {
            throw new Error('NAME is required');
        }

        this.topic = map['TOPIC']
        if (!this.topic) {
            throw new Error('TOPIC is required');
        }

        // This is safe to construct. It doesn't hit the network until you use it.
        this.client = new DaprClient('localhost', process.env.DAPR_HTTP_PORT)
    }

    public async status(): Promise<BindingStatus> {
        // nothing complex, we just send a message.
        await this.client.pubsub.publish(this.name, this.topic, { message: 'hello, world!' })
        return { ok: true, message: "message sent"};
    }

    public toString = () : string => {
        return 'Dapr PubSub';
    }
}