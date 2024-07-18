#!/bin/bash

chart_name=$(helm inspect chart install/shibuya | awk -F': ' '/^name:/ {print $2}')
chart_version=$(helm inspect chart install/shibuya | awk -F': ' '/^version:/ {print $2}')
chart_output=$chart_name-$chart_version
if gh release view $chart_output; then
    echo "Release already exists!"
else
    gh release create $chart_output --latest=false $chart_output.tgz --notes $chart_output -t $chart_output
fi
