var EventBus = new Vue();
var SYNC_INTERVAL = 5000;
Vue.http.options.root = "/api";
Vue.http.options.emulateJSON = true;

var DelimitorMixin = {
    delimiters: ['${', '}']
}

var Modal = Vue.component("modal", {
    template: "#modal-tmpl"
});

var UploadMixin = {
    methods: {
        upload: function (event) {
            var pending_files = event.target.files;
            if (pending_files.length > 0) {
                var file = pending_files[0];
                const formData = new FormData();
                formData.append(event.target.name, file, file.name);
                var self = this;
                var req = new XMLHttpRequest();
                req.open("put", "/api/" + self.upload_url);
                req.send(formData);
                req.addEventListener("loadend", function () {
                    switch (req.status) {
                        case 200:
                            alert("upload success!");
                            break;
                        default:
                            alert(req.responseText);
                    }
                    event.target.value = "";
                });
            };
        }
    }
}

var TZMixin = {
    methods: {
        toLocalTZ: function (isodate) {
            var d = new Date(isodate);
            if (d <= Date.UTC(1970)) {
                return "Running";
            }
            return Intl.DateTimeFormat('en-jp', {
                year: 'numeric', month: 'short', day: 'numeric', hour: 'numeric',
                minute: 'numeric', second: '2-digit', timeZoneName: 'short'
            }
            ).format(d);
        }
    }
}
var NewItem = Vue.component("new-item", {
    template: "#new-item-tmpl",
    mixins: [DelimitorMixin],
    props: ["attrs", "url", "event_name", "extra_attrs"],
    methods: {
        makePayload: function () {
            var payload = {}
            for (var key in this.attrs) {
                payload[key] = this.attrs[key].value;
            }
            if (!_.isEmpty(this.extra_attrs)) {
                for (var key in this.extra_attrs) {
                    payload[key] = this.extra_attrs[key];
                }
            }
            return payload
        },
        sendPayload: function (payload) {
            this.$http.post(this.url, payload).then(
                function (resp) {
                    EventBus.$emit(this.event_name);
                },
                function (resp) {
                    alert(resp.body.message);
                }
            )
        },
        handleSubmit: function () {
            var payload = this.makePayload()
            this.sendPayload(payload);
        }
    }
});
