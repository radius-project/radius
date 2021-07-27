import express, { json } from 'express';
import * as http from 'http';
import { loadBindings, BindingStatus, BindingProvider } from './binding'
import { DaprPubSubBinding } from './bindings/daprpubsub';
import { KeyVaultBinding } from './bindings/keyvault'
import { MongoBinding } from './bindings/mongo'
import { ServiceBusBinding } from './bindings/servicebus'

const app: express.Application = express();
const server: http.Server = http.createServer(app);
const port = 3000;

const providers: {[key: string]: BindingProvider }= {
    'DAPRPUBSUB': (map) => new DaprPubSubBinding(map),
    'KEYVAULT': (map) => new KeyVaultBinding(map),
    'MONGODB': (map) => new MongoBinding(map),
    'SERVICEBUS': (map) => new ServiceBusBinding(map),
};

let bindings = loadBindings(process.env, providers)
bindings.forEach(binding => {
    console.log(`loaded binding: ${binding}`);
})

// We check the health of bindings as a health check endpoint.
app.get('/healthz', async (_: express.Request, response: express.Response) => {
    let statuses: BindingStatus[] = [];

    let healthy = true;
    for (const binding of bindings) {
        let status: BindingStatus;

        try {
            status = await binding.status();
        } catch (err: unknown) {
            status = {
                ok: false,
                message: (err as Error).message,
            }
        }

        if (!status.ok) {
            healthy = false;
        }

        statuses.push(status);
    }

    let statusCode = healthy ? 200 : 500;
    response.status(statusCode).json(statuses);
})

server.listen(port, () => {
    console.log(`Server running at http://localhost:${port}`);
    console.log(`Check http://localhost:${port}/healthz for status`);
});