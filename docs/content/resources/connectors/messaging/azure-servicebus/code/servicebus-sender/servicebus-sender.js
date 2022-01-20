const { ServiceBusClient, delay } = require("@azure/service-bus");
const { sprintf } = require("sprintf-js");

// connection string to your Service Bus namespace
const connectionString = process.env.SB_CONNECTION
console.log(connectionString)
// name of the queue
const queueName = process.env.SB_QUEUE

async function main() {
	// create a Service Bus client using the connection string to the Service Bus namespace
	const sbClient = new ServiceBusClient(connectionString);

	// createSender() can also be used to create a sender for a topic.
	const sender = sbClient.createSender(queueName);
	var loop = true;

	try {
		var msgId = 1
		while (loop) {
			// create a batch object
			let batch = await sender.createMessageBatch();
			var msg = { body: sprintf("Cool Message %d", msgId) }
			if (!batch.tryAddMessage(msg)) {
				// if it fails to add the message to the current batch
				// send the current batch as it is full
				await sender.sendMessages(batch);

				// then, create a new batch 
				batch = await sender.createMessageBatch();

				// now, add the message failed to be added to the previous batch to this batch
				if (!batch.tryAddMessage(msg)) {
					// if it still can't be added to the batch, the message is probably too big to fit in a batch
					throw new Error("Message too big to fit in a batch");
				}
			}
			// }

			// Send the last created batch of messages to the queue
			await sender.sendMessages(batch);

			console.log(`Sent message Id: ${msgId} to servicebus queue: ${queueName}`);
			msgId++;
			await delay(2000)
		}
		// Close the sender
		await sender.close();
	} finally {
		await sbClient.close();
	}
}

// call the main function
main().catch((err) => {
	console.log("Error occurred: ", err);
	process.exit(1);
});
