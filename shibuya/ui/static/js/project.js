var ProjectAttrs = {
    data: function () {
        var attributes = {
            attrs: {
                "name": {
                    label: "Project Name",
                },
                "owner": {
                    label: "Project Owner (mailing list)"
                }
            }
        }
        if (enable_sid) {
            attributes["attrs"]["sid"] = {
                label: "SID"
            } 
        }
        return attributes;
    }
}

var CollectionAttrs = {
    data: function () {
        return {
            collection_attrs: {
                "name": {
                    label: "Collection Name"
                }
            }
        }
    }
}

var PlanAttrs = {
    data: function () {
        return {
            plan_attrs: {
                "name": {
                    label: "Plan Name"
                }
            }
        }
    }
}

var Project = Vue.component("project", {
    mixins: [DelimitorMixin, CollectionAttrs, PlanAttrs],
    template: "#project-tmpl",
    props: ["project"],
    data: function () {
        return {
            creating_collection: false,
            creating_plan: false,
            new_collection_url: "collections",
            new_plan_url: "plans",
            collection_event_name: "new-collection",
            plan_event_name: "new-plan",
            extra_attrs: {
                project_id: this.project.id
            }
        }
    },
    created: function () {
        var self = this;
        EventBus.$on(this.collection_event_name, function () {
            self.creating_collection = false;
        }).$on(this.plan_event_name, function () {
            self.creating_plan = false;
        });
    },
    methods: {
        collection_url: function (c) {
            return "#/collections/" + c.id;
        },
        plan_url: function (p) {
            return "#/plans/" + p.id;
        },
        newCollection: function () {
            this.creating_collection = true;
        },
        newPlan: function () {
            this.creating_plan = true;
        },
        remove: function (e) {
            e.preventDefault();
            var r = confirm("You are going to delete the project " + this.project.name + ". Continue?");
            if (!r) return;
            var url = "projects/" + this.project.id;
            this.$http.delete(url).then(
                function () {
                },
                function (resp) {
                    alert(resp.body.message);
                }
            )
        }
    }
});

var Projects = Vue.component("projects", {
    mixins: [DelimitorMixin, ProjectAttrs],
    created: function () {
        this.fetchProjects();
        this.interval = setInterval(this.fetchProjects, SYNC_INTERVAL);
        var self = this;
        EventBus.$on(this.event_name, function () {
            self.creating = false;
        })
    },
    destroyed: function () {
        clearInterval(this.interval);
    },
    template: "#projects-tmpl",
    data: function () {
        return {
            projects: [],
            creating: false,
            interval: null,
            newProjectUrl: "projects",
            event_name: "new-project"
        }
    },
    methods: {
        fetchProjects: function () {
            this.$http.get("projects", {
                params: {
                    include_collections: true,
                    include_plans: true
                }
            }).then(
                function (resp) {
                    this.projects = resp.body;
                },
                function (resp) {
                    alert(resp.body);
                }
            )
        },
        create: function () {
            this.creating = true;
        }
    }
});
