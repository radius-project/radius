import { Binding, BindingStatus} from '../binding'
import { MongoClient } from 'mongodb'

// Use this with a value like BINDING_MONGODB_CONNECTIONSTRING
export class MongoBinding implements Binding {
    private uri: string;
    private connection: Promise<MongoClient> | null;

    constructor(map: { [key: string]: string }) {
        this.uri = map['CONNECTIONSTRING'];
        if (!this.uri) {
            throw new Error('CONNECTIONSTRING is required');
        }
    }

    private async connect(): MongoClient {
        if (!this.connection) {
            this.connection = (async () : Promise<MongoClient> => {
                let client = new MongoClient(this.uri);
                await client.connect();
                return client;
            })();
        }

        return this.connection
    }

    public async status(): Promise<BindingStatus> {
        // nothing complex, we just connect.
        await this.connect();
        return { ok: true, message: "connected"};
    }

    public toString = () : string => {
        return 'MongoDB';
    }
}