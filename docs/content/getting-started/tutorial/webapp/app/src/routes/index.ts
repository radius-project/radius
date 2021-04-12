import * as express from "express";
import * as api from "./api";

export const register = (app: express.Application) => {
    app.get("/", (req: any, res) => {
        res.render("index");
    });

    api.register(app);
};
