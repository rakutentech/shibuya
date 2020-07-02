var Plan = Vue.component("plans", {
    template: "#plan-tmpl",
    mixins: [DelimitorMixin, UploadMixin],
    data: function () {
        return {
            plan: {},
            upload_file_help: upload_file_help
        }
    },
    created: function () {
        this.fetchPlan();
        this.interval = setInterval(this.fetchPlan, SYNC_INTERVAL)
    },
    destroyed: function () {
        clearInterval(this.interval);
    },
    computed: {
        plan_id: function () {
            return this.$route.params.id
        },
        upload_url: function () {
            return "plans/" + this.plan.id + "/files"
        }
    },
    methods: {
        fetchPlan: function () {
            this.$http.get("plans/" + this.plan_id).then(
                function (resp) {
                    this.plan = resp.body;
                },
                function (resp) {
                    console.log(resp.body)
                }
            )
        },
        remove: function () {
            var r = confirm("You are going to delete the plan. Continue?");
            if (!r) return;
            var url = "plans/" + this.plan_id;
            this.$http.delete(url).then(
                function (resp) {
                    window.location.href = "/";
                },
                function (resp) {
                    alert(resp.body.message);
                }
            )
        },
        deletePlanFile: function (filename) {
            var url = encodeURI("plans/" + this.plan_id + "/files?filename=" + filename);
            this.$http.delete(url).then(
                function (resp) {
                    alert("File deleted successfully");
                },
                function (resp) {
                    alert(resp.body);
                }
            )
        }
    }
});