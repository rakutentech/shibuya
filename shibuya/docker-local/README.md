# Why we need a separate files for building images

It's mysteriously slow on my local Mac for building Go binaries that are depending on k8s libaries in Docker containers. As a result, in order to fastly create local environment, I moved the building process into Mac utilising Go's cross-platform compilation capability.

On the other hand, I could not break the exsiting Dockerfiles as they are required for CI/CD otherwise it will require our current CI/CD to have Go runtime support.

# How can we ensure the running environments are the same

Please remember to update two Dockerfiles when it's necessary.