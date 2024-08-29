#!/bin/bash

new_registry=asia-northeast1-docker.pkg.dev/shibuya-214807/shibuya
old_registry=gcr.io/shibuya-214807

component=$1
old_image=$old_registry/$component
new_image=$new_registry/$component
docker pull $old_image
docker tag $old_image $new_image
docker push $new_image
