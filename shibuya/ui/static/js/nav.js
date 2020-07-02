var TopBar = new Vue({
    el: "#top-bar",
    methods: {
        logout: function () {
            this.$http.post("/logout").then(
                function (_) {
                    window.location.reload();
                },
                function (_) {
                    alert("Ooops, logout failed...");
                }
            );
        }
    }
})