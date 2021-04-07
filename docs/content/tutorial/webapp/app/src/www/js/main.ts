import axios from "axios";
import * as M from "materialize-css";
import Vue from "vue";

class Item {
    id: string | undefined
    title: string | undefined
    done: boolean | undefined
}

class ItemResponse {
    message: string | undefined
    items: Item[] | undefined
}

const data = {
    message: "",
    items: <Item[]>[],
    isLoading: true,

    title: "",

    selectedItem: "",
    selectedItemId: "",
}
const vm = new Vue({
    data() {
        return data;
    },
    computed: {
        hasMessage(): boolean {
            return this.message !== null;
        },
        hasItems(): boolean {
            return this.isLoading === false && this.items.length > 0;
        },
        isEmpty(): boolean {
            return this.isLoading === false && this.items.length === 0;
        }
    },
    el: "#app",
    methods: {
        addItem() {
            const item = {
                title: this.title
            };
            axios
                .post("/api/todos", item)
                .then(() => {
                    this.title = ""
                    this.loadItems();
                })
                .catch((err: any) => {
                    // tslint:disable-next-line:no-console
                    console.log(err);
                });
        },
        completeItem(id: string) {
            const item = this.items.find((i) => i.id === id);
            if (!item) {
                return;
            }

            item.done = true;
            axios
                .put(`/api/todos/${id}`, item)
                .then(this.loadItems)
                .catch((err: any) => {
                    // tslint:disable-next-line:no-console
                    console.log(err);
                });
        },
        confirmDeleteItem(id: string) {
            const item = this.items.find((i) => i.id === id);
            this.selectedItem = item?.title!;
            this.selectedItemId = item?.id!;

            const dc = <Element>this.$refs.deleteConfirm;
            const modal = M.Modal.init(dc);
            modal.open();
        },
        deleteItem(id: string) {
            axios
                .delete(`/api/todos/${id}`)
                .then(this.loadItems)
                .catch((err: any) => {
                    // tslint:disable-next-line:no-console
                    console.log(err);
                });
        },
        loadItems() {
            axios
                .get("/api/todos")
                .then((res: any) => {
                    this.isLoading = false;

                    const response = Object.assign(new ItemResponse(), res.data)
                    this.message = response.message
                    this.items = response.items
                })
                .catch((err: any) => {
                    // tslint:disable-next-line:no-console
                    console.log(err);
                });
        }
    },
    mounted() {
        this.loadItems()
    }
});
