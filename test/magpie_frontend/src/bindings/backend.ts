import { Binding, BindingStatus} from '../binding'
import { Response, Request, HttpClient, newHttpClient } from 'typescript-http-client'

// Use this with a values like: BINDING_KEYVAULT_URI
export class BackendBinding implements Binding {
    private uri: string;
    private client: HttpClient;

    constructor(map: { [key: string]: string }) {
        this.uri = map['URI'];
        if (!this.uri) {
            throw new Error('URI is required');
        }

        // This is safe to construct. It doesn't hit the network until you use it.
        this.client = newHttpClient()
    }

    public async status(): Promise<BindingStatus> {
        const request = new Request('http://' + process.env.SERVICE__BACKEND__HOST + ':' +  process.env.SERVICE__BACKEND__PORT + '/backend', { responseType: 'text' })
        let response = await this.client.execute<string>(request)
        let healthy = false
        if (response != "") {
            healthy = true
        }
        return { ok: healthy, message: "healthz"};
    }

    public toString = () : string => {
        return 'Backend';
    }
}