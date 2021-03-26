// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

const express = require('express');
const bodyParser = require('body-parser');

const port = 50051;

const app = express();
// Dapr publishes messages with the application/cloudevents+json content-type
app.use(bodyParser.json({ type: 'application/*+json' }));

var pubsubName = process.env.SB_PUBSUBNAME
var topic = process.env.SB_TOPIC
app.get('/dapr/subscribe', (_req, res) => {
    res.json([
        {
            pubsubname: pubsubName,
            topic: topic,
            route: "A"
        },
    ]);
});

app.post('/A', (req, res) => {
    console.log(topic, ": ", req.body.data.message);
    res.sendStatus(200);
});

app.listen(port, () => console.log(`Node App listening on port ${port}!`));
