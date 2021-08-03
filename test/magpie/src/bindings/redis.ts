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
        let cacheConnection = (this.password) ? redis.createClient(
                +this.port, 
                this.host, 
                {
                    auth_pass: this.password,
                    tls: {servername: this.host}
                }) 
                : redis.createClient(+this.port, this.host, {});

        cacheConnection.on("error", function(error) {
            console.error(error);
        });

        // Simple PING command
        console.log("\nCache command: PING");
        cacheConnection.ping(function(error, res) {
            if (error) throw error;
            console.log("Cache response : " + res);
        });

        return { ok: true, message: "connected"};
    }

    public toString = () : string => {
        return 'Redis';
    }
}