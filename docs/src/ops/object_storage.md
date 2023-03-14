# Object Storage

In Shibuya, we support two types of object storage:

1. Nexus(REST based)
2. GCP bucket

## Nexus

Nexus provides a REST API for resource CRUD. HTTP Basic authentication is used. If the storage solution you are using is following the same API spec, it will also work.
Specifically:

| HTTP method  | Resource operation |
| ------------ | ------------------ |
| GET | Get the resource     |
| PUT | Upload the resource  |
| DELETE  | DELETE the resource  |

All these method require HTTP basic authentication.

## GCP

