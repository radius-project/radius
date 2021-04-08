import express from 'express';
import path from 'path';
import * as routes from "./routes";

const app = express();
const port = process.env.PORT || 3000;

// Using the DB_CONNECTION env-var to pass the connection string
const connectionString = process.env.DB_CONNECTION;
if (connectionString) {
  app.set("connectionString", connectionString);
}

app.use(express.json());

app.set("views", path.join(__dirname, "views"));
app.set("view engine", "ejs");

app.use(express.static(path.join(__dirname, "www")));

routes.register(app);

function logError(err: any, req: any, res: any, next: any) {
  console.log(err)
  next()
}
app.use(logError)

process.on('SIGINT', function() {
  console.log( "\nGracefully shutting down from SIGINT (Ctrl-C)" );
  process.exit(1);
});

app.listen(port, () =>
  console.log(`App listening on port ${port}!`),
);
