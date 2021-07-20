import express from 'express';
import * as http from 'http';

const app: express.Application = express();
const server: http.Server = http.createServer(app);
const port = 3000;

server.listen(port, () => {
    console.log(`Server running at http://localhost:${port}`);
});