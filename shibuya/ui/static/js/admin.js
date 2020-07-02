var AdminRoot = Vue.component("admin_root", {
    template: "#admin-route"
});

var CollectionAdmin = Vue.component("collection-admin", {
    template: "#admin-collection-tmpl",
    mixins: [DelimitorMixin, TZMixin],
    data: function () {
        return {
            running_collections: [],
            node_pools: {}
        }
    },
    created: function () {
        this.fetchRunningCollections();
        this.interval = setInterval(this.fetchRunningCollections, SYNC_INTERVAL);
    },
    destroyed: function () {
        clearInterval(this.internal);
    },
    methods: {
        fetchRunningCollections: function () {
            this.$http.get("admin/collections").then(
                function (resp) {
                    this.running_collections = resp.body.running_collections;
                    this.node_pools = resp.body.node_pools;
                },
                function (resp) {
                    console.log(resp.body);
                }
            )
        },
        collection_url: function (collection_id) {
            return "#collections/" + collection_id;
        }
    }
});

var admin_routes = [
    {
        path: "/admin/",
        component: AdminRoot
    },
    {
        path: "/admin/collections",
        component: CollectionAdmin
    }
]
var admin_router = new VueRouter({
    routes: admin_routes
});
var AdminComponent = new Vue({
    router: admin_router
});
AdminComponent.$mount(".shibuya-admin")



