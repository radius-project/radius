import { Binding, BindingStatus} from '../binding'
import redis from 'redis';

// Use this with a value like BINDING_REDIS_CONNECTIONSTRING
export class RedisBinding implements Binding {
    private host: string;
    private port: string;
    private password: string;

    constructor(map: { [key: string]: string }) {
        this.host = map['HOST'];
        if (!this.host) {
            throw new Error('HOST is required');
        }

        this.port = map['PORT'];
        if (!this.port) {
            throw new Error('PORT is required');
        }

        this.password = map['PASSWORD'];
        if (!this.password) {
            throw new Error('PORT is required');
        }
    }

    public async status(): Promise<BindingStatus> {
        // from https://docs.microsoft.com/en-us/azure/azure-cache-for-redis/cache-nodejs-get-started
        var cacheConnection = redis.createClient(+this.port, this.host, 
            {
                auth_pass: this.password, 
                tls: {servername: this.host}
            });

        cacheConnection.on("error", function(error) {
            console.error(error);
        });
        // Simple PING command
        console.log("\nCache command: PING");
        cacheConnection.ping(function(error, res) {
            if (error != null) {
                console.error("Error: " + error);
            }
            console.log("Cache response : " + res);
        });

        // Simple get and put of integral data types into the cache
        console.log("\nCache command: GET Message");
        console.log("Cache response : " + cacheConnection.get("Message"));    

        console.log("\nCache command: SET Message");
        console.log("Cache response : " + cacheConnection.set("Message",
            "Hello! The cache is working from Node.js!"));    

        // Demonstrate "SET Message" executed as expected...
        console.log("\nCache command: GET Message");
        console.log("Cache response : " + cacheConnection.get("Message"));    

        // Get the client list, useful to see if connection list is growing...
        var canList = cacheConnection.client("LIST")
        console.log("\nCache command: CLIENT LIST");
        console.log("Cache response : " + cacheConnection.client("LIST")); 

        return { ok: canList, message: "connected"};
    }

    public toString = () : string => {
        return 'Redis';
    }
}