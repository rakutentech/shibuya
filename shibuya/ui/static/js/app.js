var routes = [
    {
        path: "/",
        component: Projects
    },
    {
        path: "/collections/:id",
        component: Collection
    },
    {
        path: "/plans/:id",
        component: Plan
    },

];

var router = new VueRouter(
    {
        routes: routes
    }
)
var shibuya = new Vue({
    router: router,
})

shibuya.$mount(".shibuya")
