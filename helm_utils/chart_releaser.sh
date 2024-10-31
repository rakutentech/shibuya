#!/bin/bash
chart_location=$1
chart_name=$(helm inspect chart $chart_location | awk -F': ' '/^name:/ {print $2}')
chart_version=$(helm inspect chart $chart_location | awk -F': ' '/^version:/ {print $2}')
chart_output=$chart_name-$chart_version
echo $chart_name
echo $chart_version
if gh release view $chart_output; then
    echo "Release already exists!"
else
    gh release create $chart_output --latest=false $chart_output.tgz --notes $chart_output -t $chart_output
fi
