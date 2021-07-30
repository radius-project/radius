import { Binding, BindingStatus} from '../binding'
import { RedisClient } from 'redis'

// Use this with a value like BINDING_REDIS_CONNECTIONSTRING
export class RedisBinding implements Binding {
    private uri: string;
    private client: RedisClient;

    constructor(map: { [key: string]: string }) {
        this.uri = map['CONNECTIONSTRING'];
        if (!this.uri) {
            throw new Error('CONNECTIONSTRING is required');
        }

        this.client = RedisClient.createClient();
    }

    private async connect(): RedisClient {
        this.client.set("key", "value");
    }

    public async status(): Promise<BindingStatus> {
        // nothing complex, we just connect.
        await this.connect();
        return { ok: true, message: "connected"};
    }

    public toString = () : string => {
        return 'Redis';
    }
}