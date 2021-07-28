import { Binding, BindingStatus} from '../binding'
import { ServiceBusClient, ServiceBusSender, ServiceBusMessage } from '@azure/service-bus'

// Use this with a values like:
// - BINDING_SERVICEBUS_CONNECTIONSTRING
// - BINDING_SERVICEBUS_QUEUE
export class ServiceBusBinding implements Binding {
    private connectionString: string;
    private queue: string;
    private sender: ServiceBusSender

    constructor(map: { [key: string]: string }) {
        this.connectionString = map['CONNECTIONSTRING'];
        if (!this.connectionString) {
            throw new Error('CONNECTIONSTRING is required');
        }

        this.queue = map['QUEUE']
        if (!this.connectionString) {
            throw new Error('QUEUE is required');
        }

        // These are safe to construct. It doesn't hit the network until you use it.
        let connection = new ServiceBusClient(this.connectionString);
        this.sender = connection.createSender(this.queue)
    }

    public async status(): Promise<BindingStatus> {
        // nothing complex, we just send a message.
        await this.sender.sendMessages({ body: 'hello, world!' })

        return { ok: true, message: "message sent"};
    }

    public toString = () : string => {
        return 'Azure ServiceBus';
    }
}