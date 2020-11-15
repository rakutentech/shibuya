# Configuration Explanation

Below is a sample configuration of Shibuya. We will explain them by sections.

```
{
    "bg_color": "#fff",
    "project_home": "",
    "upload_file_help": "",
    "auth_config": {
        "admin_users": [],
        "ldap_server": "",
        "ldap_port": "",
        "system_user": "",
        "system_password": "",
        "base_dn": "",
        "no_auth": true
    },
    "http_config": {
        "proxy": ""
    },
    "db": {
        "host": "db",
        "user": "root",
        "password": "root",
        "database": "shibuya"
    },
    "executors": {
        "cluster": {
            "on_demand": false
        },
        "in_cluster": true,
        "namespace": "shibuya-executors",
        "jmeter": {
            "image": "shibuya:jmeter",
            "cpu": "1",
            "mem": "512Mi"
        },
        "pull_secret": "",
        "pull_policy": "IfNotPresent"
    },
    "dashboard": {
        "url": "http://localhost:3000",
        "run_dashboard": "/d/RXY8nM1mk2/shibuya",
        "engine_dashboard": "/d/9EH6xqTZz/shibuya-engine-health"
    },
    "object_storage": {
        "provider": "local",
        "url": "http://storage:8080",
        "user": "",
        "password": ""
    },
    "log_format": {
        "json": false
    }
}
```

## General Configs

```
    "bg_color": "#fff",  # UI bg colour. Could be useful when you are using Shibuya in multiple networking environments.
    "project_home": "",
    "upload_file_help": "", # Document link for uploading the file
```

## Auth related

All authentication related logic is configured by this block

```
    "auth_config": {
        "admin_users": [], # admin mailing list. A admin will have a dedicated page to view all the running collections
        "ldap_server": "", 
        "ldap_port": "",
        "system_user": "", # ldap system user
        "system_password": "", # ldap system pwd
        "base_dn": "",
        "no_auth": true # Turn off auth completely
    }
```

## HTTP client 

Once this is configured, all the traffic will pass through proxy. Including metrics streaming and requests to k8s cluster.

```
    "http_config": {
        "proxy": ""
    }
```

## DB configurations

```
    "db": {
        "host": "db",
        "user": "root",
        "password": "root",
        "database": "shibuya"
    }
```

## Executor configurations

Shibuya supports two types of clusters:

1. on demand, specifically, GKE in Google Cloud Platform. 
2. on-premise cluster.

With on demand mode, Shibuya is able to automatically create nodes and clean resources after usage. In most cases, the GKE cluster used by Shibuya has 0 worker nodes(to save money). 

Shibuya controller can be run outside of a k8s cluster, which usually is the cluster where the generators are deployed. If this is the case, `in_cluster` should be set to `false`, `true` for otherwise.

```
    "executors": {
        "cluster": {
            "on_demand": false
        },
        "in_cluster": true,
        "namespace": "shibuya-executors", # this is the namespace where generators are deployed
        "jmeter": {
            "image": "shibuya:jmeter", 
            "cpu": "1", # resoures(requests) for the generator pod in a k8s cluster.
            "mem": "512Mi"
        },
        "pull_secret": "",
        "pull_policy": "IfNotPresent"
    }
```

## Metrics dashboard

Shibuya uses external Grafana dashboard to visualise the metrics. 

```
    "dashboard": {
        "url": "http://localhost:3000", # root of the dashboard url
        "run_dashboard": "/d/RXY8nM1mk2/shibuya", # link to the metrics of all the runs.
        "engine_dashboard": "/d/9EH6xqTZz/shibuya-engine-health" # link to the health of the engines.
    }
```

## Object storage

Shibuya uses object storage to store all the test plans. It supports two types storage:

1. HTTP based storage service, like, Nexus. 
2. GCP bucket. 

```
    "object_storage": {
        "provider": "local", # either gcp, local, or Nexus
        "url": "http://storage:8080",
        "user": "", # HTTP basic authentication user and password
        "password": ""
    },
```

Please bear in mind, `local` should be only used by Shibuya developers. 

## Logging support

```
    "log_format": {
        "json": false
    }
```

If you require logs to be in JSON format, you can set `json: true`.

## GCP

TODO