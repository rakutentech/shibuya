# How to build the docs

## deps

mdbook: please install [here](https://github.com/rust-lang/mdBook)

## Build step

1. `cd docs`
2. `mdbook build`
3. `mdbook serve`

You can also `watch` resources and let mdbook automatically build for you

1. `mdbook watch`
2. `mdbook serve`

## Use GitHub action

If you want to enable the GitHub pages in your fork, please go to this [doc](https://github.com/peaceiris/actions-mdbook)

After syncing with the upstream master(that will include the `.github` folder which includes the action configuration), Basically you only need to configure the GitHub pages to be built from the `gh-pages` branch.