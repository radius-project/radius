const { ServiceBusClient } = require("@azure/service-bus");

// connection string to your Service Bus namespace
const connectionString = process.env.SB_CONNECTION
// name of the queue
const queueName = process.env.SB_QUEUE

async function main() {
	var loop = true;
	const sbClient = new ServiceBusClient(connectionString);
	const receiver = sbClient.createReceiver(queueName);
	try {
		while (loop) {
			const myMessages = await receiver.receiveMessages(3);
			myMessages.forEach(element => {
				console.log("Messages: " + element.body)
			});
			console.log("\n\n");
		}
		await receiver.close()
	} finally {
		await sbClient.close();
	}
}

// call the main function
main().catch((err) => {
	console.log("Error occurred: ", err);
	process.exit(1);
});