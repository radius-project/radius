const express = require('express');
const bodyParser = require('body-parser');
const { response } = require('express');
require('isomorphic-fetch');

const app = express();
app.use(bodyParser.json());

// These ports are injected automatically into the container.
const daprPort = process.env.DAPR_HTTP_PORT; 
const daprGRPCPort = process.env.DAPR_GRPC_PORT;

const stateStoreName =  process.env.CONNECTION_ORDERS_STATESTORENAME;
const stateUrl = `http://localhost:${daprPort}/v1.0/state/${stateStoreName}`;
const port = 3000;

app.get('/order', (_req, res) => {
    if (!process.env.DAPR_HTTP_PORT) {
        res.status(400).send({message: "The container is running, but Dapr has not been configured."});
        return;
    }

    if (!process.env.CONNECTION_ORDERS_STATESTORENAME) {
        res.status(400).send({message: "The container is running, but the state store name is not set."});
        return;
    }

    fetch(`${stateUrl}/order`)
        .then((response) => {
            if (response.status == 204 || response.status == 404) {
                return Promise.resolve([]);
            }
            if (!response.ok) {
                throw "Could not get state.";
            }

            return response.json();
        }).then((orders) => {
            if (orders.length === 0) {
                res.send({  message: "no orders yet" })
            } else {
                res.send({ items: orders })
            }
        }).catch((error) => {
            console.log(error);
            res.status(500).send({message: error});
        });
});

app.post('/neworder', (req, res) => {
    const data = req.body;
    const orderId = data.orderId;
    console.log("Got a new order! Order ID: " + orderId);

    fetch(`${stateUrl}/order`, {
        method: "GET"
    }).then((response) => {
        if (!response.ok) {
            throw "Failed to read state.";
        }

        if (response.status == 204 || response.status == 404) {
            return Promise.resolve([]);
        }

        return response.json();
    }).then((orders) => {
        orders.push(data);
        const state = [{
            key: "order",
            value: orders
        }];

        return fetch(stateUrl, {
            method: "POST",
            body: JSON.stringify(state),
            headers: {
                "Content-Type": "application/json"
            }
        })
    }).then((response) => {
        if (!response.ok) {
            throw "Failed to persist state.";
        }

        console.log("Successfully persisted state.");
        res.status(200).send();
    }).catch((error) => {
        console.log(error);
        res.status(500).send({message: error});
    });
});

app.get('/ports', (_req, res) => {
    console.log("DAPR_HTTP_PORT: " + daprPort);
    console.log("DAPR_GRPC_PORT: " + daprGRPCPort);
    res.status(200).send({DAPR_HTTP_PORT: daprPort, DAPR_GRPC_PORT: daprGRPCPort })
});

app.listen(port, () => console.log(`Node App listening on port ${port}!`));