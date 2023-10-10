var PlanAttrs = {
    data: function () {
        return {
            "name": {
                label: "Plan Name"
            }
        }
    }
}
var Collection = Vue.component("collection", {
    mixins: [DelimitorMixin, UploadMixin, TZMixin],
    template: "#collection-tmpl",
    data: function () {
        return {
            cache: {},
            collection_status: {},
            plan_status: {},
            collection: {},
            launched: false,
            triggered: false,
            internal: null,
            collection_file: null,
            trigger_in_progress: false,
            stop_in_progress: false,
            purge_in_progress: false,
            on_demand_cluster: on_demand_cluster,
            upload_file_help: upload_file_help,
            showing_log: false,
            showing_engines_detail: false,
            log_content: "",
            log_modal_title: "",
            engines_detail: {},
            upload_url: ""
        }
    },
    computed: {
        nodes_plural: function () {
            if (this.collection_status.pool_size > 1) {
                return "nodes";
            }
            return "node";
        },
        verb: function () {
            if (this.collection_status.pool_size > 1) {
                return "are";
            }
            return "is";
        },
        running_context: function () {
            return running_context;
        },
        collection_id: function () {
            return this.$route.params.id;
        },
        stopped: function () {
            return !this.triggered;
        },
        show_runs: function () {
            if (!this.collection.hasOwnProperty("run_history")) return false;
            if (this.collection.run_history === null) return false;
            return this.collection.run_history.length > 0;
        },
        can_be_launched: function () {
            var that = this;
            var everything_is_launched = true;
            _.all(this.collection_status.status, function (plan) {
                if (plan.engines_deployed === 0) {
                    that.launched = false;
                    everything_is_launched = false;
                    return;
                }
            });
            if (!everything_is_launched) {
                return true;
            }
            if (this.can_be_triggered) {
                this.launched = true;
            }
            if (!this.collection.hasOwnProperty("execution_plans")) {
                return false;
            }
            return this.collection.execution_plans.length > 0 && !this.launched;
        },
        can_be_triggered: function () {
            var collection_status = this.collection_status.status;
            if (collection_status == null) {
                return false;
            }
            var t = collection_status.length > 0;
            _.all(collection_status, function (plan) {
                t = t && plan.engines_deployed === plan.engines && plan.engines_reachable
            });
            return t;
        },
        launchable: function () {
            if (this.can_be_triggered) {
                this.launched = true;
            }
            return this.can_be_launched && !this.launched;
        },
        triggerable: function () {
            return this.can_be_triggered && this.launched && this.stopped;
        },
        stoppable: function () {
            return this.triggered;
        },
        purge_tip: function () {
            var t = true;
            _.all(this.collection_status.status, function (plan) {
                t = t && (plan.engines_deployed === plan.engines);
            });
            if (!t) {
                this.purge_in_progress = false;
            }
            return this.purge_in_progress && t;
        },
        collectionConfigDownloadUrl: function () {
            return "api/collections/" + this.collection_id + "/config";
        },
        engine_remaining_time: function () {
            var engines_detail = this.engines_detail;
            var engine_life_span = gcDuration; // value we get from the ui handler
            if (engines_detail.engines.length > 0) {
                var e = engines_detail.engines[0],
                    now = Date.now(),
                    created_time = new Date(e.created_time)
                var running_time = (now - created_time) / 1000 / 60;
                return Math.ceil(engine_life_span - running_time);
            }
            return engine_life_span;
        },
        total_engines: function () {
            var total = 0;
            _.each(this.collection_status.status, function (plan) {
                total += plan.engines;
            })
            return total
        }
    },
    created: function () {
        this.fetchCollection();
        this.interval = setInterval(this.fetchCollection, SYNC_INTERVAL);
    },
    destroyed: function () {
        clearInterval(this.interval);
    },
    methods: {
        updateCache: function (collection_status) {
            var self = this,
                stopped = true;
            _.each(collection_status, function (plan_status) {
                plan_status.started_time = new Date(plan_status.started_time);
                stopped = stopped && !plan_status.in_progress;
                Vue.set(self.cache, plan_status.plan_id, plan_status);
            });
            this.triggered = !stopped;
        },
        fetchCollection: function () {
            this.$http.get("collections/" + this.collection_id).then(
                function (resp) {
                    this.collection = resp.body;
                },
                function (resp) {
                    alert(resp.body.message);
                }
            );
            this.$http.get("collections/" + this.collection_id + '/status').then(
                function (resp) {
                    this.collection_status = resp.body;
                    this.updateCache(this.collection_status.status);
                },
                function (resp) {
                    alert(resp.body.message);
                }
            );
        },
        plan_url: function (plan_id) {
            return "#plans/" + plan_id;
        },
        launch: function () {
            var url = "collections/" + this.collection_id + "/deploy"
            this.$http.post(url).then(
                function (resp) {
                    this.launched = true;
                    this.purged = false;
                },
                function (resp) {
                    alert(resp.body.message);
                }
            )
        },
        trigger: function () {
            this.trigger_in_progress = true;
            var url = "collections/" + this.collection_id + "/trigger"
            this.$http.post(url).then(
                function (resp) {
                    this.triggered = true;
                    this.trigger_in_progress = false;
                },
                function (resp) {
                    alert(resp.body.message);
                    this.trigger_in_progress = false;
                }
            );
        },
        stop: function () {
            this.stop_in_progress = true;
            var url = "collections/" + this.collection_id + "/stop"
            this.$http.post(url).then(
                function (resp) {
                    this.triggered = false;
                    this.stop_in_progress = false;
                },
                function (resp) {
                    console.log(resp.body);
                    this.stop_in_progress = false;
                }
            );
        },
        purge: function () {
            var url = "collections/" + this.collection_id + "/purge"
            this.purge_in_progress = true;
            this.$http.post(url).then(
                function (resp) {
                    this.launched = false;
                    this.triggered = false;
                },
                function (resp) {
                    console.log(resp.body);
                }
            );
        },
        remove: function () {
            var r = confirm("You are going to delete the collection. Continue?");
            if (!r) return;
            var url = "collections/" + this.collection_id;
            this.$http.delete(url, {
                collection_id: this.collection_id
            }).then(
                function (resp) {
                    window.location.href = "/";
                },
                function (resp) {
                    alert(resp.body.message);
                }
            )
        },
        calPlanLaunchProgress: function (plan_id) {
            var status = this.cache[plan_id];
            if (status === undefined) {
                return "0%";
            }
            var progress = status.engines_deployed / status.engines;
            return progress.toFixed(2) * 100;
        },
        progressBarStyle: function (plan_id) {
            var p = this.calPlanLaunchProgress(plan_id);
            return {
                width: p * 0.5 + "%"
            }
        },
        isPlanReachable: function (plan_id) {
            var status = this.cache[plan_id];
            if (status == undefined) {
                return false;
            }
            return status.engines_reachable;
        },
        reachableText: function (plan_id) {
            return this.isPlanReachable(plan_id) ? "Reachable" : "Unreachable"
        },
        reachableClass: function (plan_id) {
            return this.isPlanReachable(plan_id) ? "progress-bar bg-success" : "progress-bar bg-danger";
        },
        reachableStyle: function (plan_id) {
            var status = this.cache[plan_id];
            var style = {
                width: "100%"
            };
            if (status == undefined) {
                return style
            }
            if (!status.engines_deployed) {
                return style
            }
            style.width = "50%";
            return style;
        },
        planStarted: function (plan) {
            var plan_status = this.cache[plan.plan_id];
            if (plan_status === undefined) {
                return false
            }
            return plan_status.in_progress;
        },
        runningProgress: function (plan) {
            if (!this.planStarted(plan)) {
                return "0%";
            }
            var plan_status = this.cache[plan.plan_id];
            var started_time = plan_status.started_time,
                now = new Date(),
                delta = Math.abs(now - started_time),
                duration = plan.duration * 60 * 1000,
                progress = Math.min(100, delta / duration * 100); // we can have overflow
            return progress.toFixed(0) + "%";
        },
        runningProgressStyle: function (plan) {
            var p = this.runningProgress(plan);
            return {
                width: p
            }
        },
        showEnginesDetail: function (e) {
            e.preventDefault();
            var url = "collections/" + this.collection.id + "/engines_detail";
            this.$http.get(url).then(
                function (resp) {
                    this.showing_engines_detail = true;
                    this.engines_detail = resp.body;
                },
                function (resp) {
                    alert(resp.body.message);
                }
            );
        },
        viewPlanLog: function (e, plan_id) {
            e.preventDefault();
            var url = "collections/" + this.collection.id + "/logs/" + plan_id;
            this.log_modal_title = this.collection.name + "/" + plan_id;
            this.$http.get(url).then(
                function (resp) {
                    this.showing_log = true;
                    this.log_content = resp.body.c;
                },
                function (resp) {
                    if (!this.triggered) {
                        alert("The collection has not been triggered!");
                        return
                    }
                    console.log(resp.body.message);
                }
            )
        },
        runGrafanaUrl: function (run) {
            //buffer 1 minute before and after because of time lag in shipping of results
            var start = new Date(run.started_time);
            start.setMinutes(start.getMinutes() - 1);
            var end = new Date(run.end_time);
            if (end.getTime() <= 0) {
                return result_dashboard + "?var-runID=" + run.id + "&from=" + start.getTime() + "&to=now" + "&refresh=3s";
            }
            end.setMinutes(end.getMinutes() + 1);
            return result_dashboard + "?var-runID=" + run.id + "&from=" + start.getTime() + "&to=" + end.getTime();
        },
        hasEngineDashboard: function () {
            return engine_health_dashboard !== "";
        },
        engineHealthGrafanaUrl: function () {
            return engine_health_dashboard + "?var-collectionID=" + this.collection_id;
        },
        purgeNodes: function () {
            var url = "collections/" + this.collection_id + "/nodes";
            this.$http.delete(url).then(
                function (resp) {
                    alert("Deleting nodes in process...This will take some time.");
                },
                function (resp) {
                    alert(resp.body.message);
                }
            );
        },
        makeUploadURL: function (path) {
            switch (path) {
                case "yaml":
                    this.upload_url = "collections/" + this.collection.id + "/config";
                    break;
                case "data":
                    this.upload_url = "collections/" + this.collection.id + "/files"
                    break;
                default:
                    console.log("Wrong upload type selection for making collection upload url");
            }
        },
        deleteCollectionFile: function (filename) {
            var url = encodeURI("collections/" + this.collection_id + "/files?filename=" + filename);
            this.$http.delete(url).then(
                function (resp) {
                    alert("File deleted successfully");
                },
                function (resp) {
                    alert(resp.body);
                }
            );
        }
    }
});
