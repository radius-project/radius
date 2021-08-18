import { Binding, BindingStatus} from '../binding'
const amqp = require('amqplib');

// Use this with a values like:
// - BINDING_AMQP_CONNECTIONSTRING
// - BINDING_AMQP_QUEUE
export class RabbitMQBinding implements Binding {
    private connectionString: string;
    private queue: string;

    constructor(map: { [key: string]: string }) {
        this.connectionString = map['CONNECTIONSTRING'];
        if (!this.connectionString) {
            throw new Error('CONNECTIONSTRING is required');
        }

        this.queue = map['QUEUE']
        if (!this.connectionString) {
            throw new Error('QUEUE is required');
        }
    }

    public async status(): Promise<BindingStatus> {
        // From https://github.com/rabbitmq/rabbitmq-tutorials/blob/master/javascript-nodejs/src/send.js
        let conn = await amqp.connect(this.connectionString);
        let channel = await conn.createChannel();
        var msg = 'Hello World!';

        await channel.assertQueue(this.queue, {
            durable: false
        });
        
        await channel.sendToQueue(this.queue, Buffer.from(msg));

        console.log("sent %s", msg);

        await channel.close()
        await conn.close()

        return { ok: true, message: "message sent"};
    }

    public toString = () : string => {
        return 'RabbitMQ';
    }
}