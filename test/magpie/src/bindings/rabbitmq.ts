import { Binding, BindingStatus} from '../binding'
import { amqp } from 'amqplib/callback_api'

// Use this with a values like:
// - BINDING_AMQP_CONNECTIONSTRING
// - BINDING_AMQP_QUEUE
export class AMQPBinding implements Binding {
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
        amqp.connect(this.connectionString, {}, (err, conn) => {
            if (err) {
                throw err;
            }
            conn.createChannel((err, ch) => {
                if (err) {
                    throw err;
                }
                var msg = 'Hello World!';

                ch.assertQueue(this.queue, {
                    durable: false
                });
                ch.sendToQueue(this.queue, Buffer.from(msg));
        
                console.log(" [x] Sent %s", msg);
            });
        });
        return { ok: true, message: "message sent"};
    }

    public toString = () : string => {
        return 'AMQP';
    }
}