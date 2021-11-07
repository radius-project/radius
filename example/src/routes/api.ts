import * as express from "express";
import { connect, MongoClient } from "mongodb";
import { v4 as uuidv4 } from 'uuid';

interface Item {
    id: string | undefined
    title: string | undefined
    done: boolean | undefined
}

interface Repository {
    isRealDatabase(): boolean
    get(id: string): Promise<Item | null>
    list() : Promise<Item[]>
    update(item: Item) : Promise<Item | null>
    create(item: Item): Promise<Item>
    delete(id: string): Promise<void>
}

const items = <Item[]>[]

export const register = (app: express.Application) => {
    const respository = connectToDb(app.get("connectionString"))

    app.get(`/api/todos`, async (req, res) => {
        const repo = await respository;
        const items = await repo.list()

        let message : string | null = null;
        if (!repo.isRealDatabase()) {
            message = "No database is configured, items will be stored in memory.";
        }

        res.status(200);
        res.json({ items: items, message: message })
    });

    app.get(`/api/todos/:id`, async (req, res) => {
        const id = req.params.id;
        const item = await (await respository).get(id);
        if (!item) {
            res.sendStatus(404);
            return
        }

        res.status(200);
        res.json(item);
    });

    app.delete(`/api/todos/:id`, async (req, res) => {
        const id = req.params.id;
        await (await respository).delete(id);

        res.sendStatus(204);
    });

    app.put(`/api/todos/:id`, async (req, res) => {
        const item = req.body as Item;
        item.id = req.params.id

        const updated = await (await respository).update(item);
        if (!updated) {
            res.sendStatus(404);
            return
        }

        res.status(200);
        res.json(updated);
    });

    app.post(`/api/todos`, async (req, res) => {
        const item = req.body as Item;
        const updated = await (await respository).create(item);

        res.status(200);
        res.json(updated);
    });
};

function connectToDb(connectionString: string | null): Promise<Repository> {
    if (connectionString){
        console.log("initialized with a database connection");
        return connect(connectionString).then(value => new MongoRepository(value));
    } else {
        console.log("initialized without a database connection");
        return Promise.resolve(new InMemoryRepository());
    }
}

class InMemoryRepository implements Repository {
    items = <Item[]>[]

    isRealDatabase(): boolean {
        return false;
    }
    get(id: string): Promise<Item | null> {
        const item = items.find(i => i.id == id)
        return Promise.resolve<Item | null>(item ?? null);
    }
    list(): Promise<Item[]> {
        return Promise.resolve(items);
    }
    update(item: Item): Promise<Item | null> {
        const index = items.findIndex(i => i.id == item.id)
        if (index < 0) {
            return Promise.resolve(null);
        }

        items[index] = item;
        return Promise.resolve(item);
    }
    create(item: Item): Promise<Item> {
        const id = uuidv4();
        item.id = id;
        items.push(item)
        return Promise.resolve(item);
    }
    delete(id: string): Promise<void> {
        const index = items.findIndex(i => i.id == id)
        if (index < 0) {
            return Promise.resolve();
        }

        items.splice(index, 1);
        return Promise.resolve();
    }

}

class MongoRepository implements Repository {
    constructor(client: MongoClient) {
        this.client = client;
    }

    client: MongoClient

    isRealDatabase(): boolean {
        return true;
    }
    async get(id: string): Promise<Item | null> {
        const collection = this.client.db("todos").collection("todos");
        const item = await collection.findOne({ id: id });
        return item as Item | null;
    }
    async list(): Promise<Item[]> {
        const collection = this.client.db("todos").collection("todos");
        const items = await collection.find({}).toArray();
        return items as Item[];
    }
    async update(item: Item): Promise<Item | null> {
        const collection = this.client.db("todos").collection("todos");
        const result = await collection.findOneAndReplace(
            { id: item.id, }, 
            {
                id: item.id,
                title: item.title,
                done: item.done,
            })
        return result.value as Item | null;
    }
    async create(item: Item): Promise<Item> {
        const id = uuidv4();
        item.id = id;

        const collection = this.client.db("todos").collection("todos");
        const result = await collection.insertOne(item);
        return item;
    }
    async delete(id: string): Promise<void> {
        const collection = this.client.db("todos").collection("todos");
        await collection.findOneAndDelete({ id: id });
    }
}