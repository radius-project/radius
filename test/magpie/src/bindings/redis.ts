import { Binding, BindingStatus} from '../binding'
import redis from 'redis';

// Use this with the three following values:
// BINDING_REDIS_HOST: the host string
// BINDING_REDIS_PORT: the port string
// BINDING_REDIS_PASSWORD: the password string, for azure it's the primary key
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
        var success = cacheConnection.ping(function(error, res) {
            if (error != null) {
                console.error("Error: " + error);
            }
            console.log("Cache response : " + res);
        });
        if (!success) {
            throw new Error("Could not ping redis cache");
        }

        // Simple get and put of integral data types into the cache
        success = cacheConnection.get("Message");
        console.log("\nCache command: GET Message");
        console.log("Cache response : " + success);    
        if (!success) {
            throw new Error("Could not get on redis cache");
        }

        success = cacheConnection.set("Message", "Hello! The cache is working from Node.js!")
        console.log("\nCache command: SET Message");
        console.log("Cache response : " + success);    
        if (!success) {
            throw new Error("Could not set on redis cache");
        }

        // Demonstrate "SET Message" executed as expected...
        success = cacheConnection.get("Message", function(error, res) {
            if (error != null) {
                console.error("Error: " + error);
            }
            console.log("Cache response : " + res);
        });
        console.log("\nCache command: GET Message");
        console.log("Cache response : " + cacheConnection.get("Message"));    
        if (!success) {
            throw new Error("Could not get on redis cache");
        }

        // Get the client list, useful to see if connection list is growing...
        success = cacheConnection.client("LIST")
        console.log("\nCache command: CLIENT LIST");
        console.log("Cache response : " + cacheConnection.client("LIST")); 
        if (!success) {
            throw new Error("Could not list on redis cache");
        }

        return { ok: true, message: "connected"};
    }

    public toString = () : string => {
        return 'Redis';
    }
}