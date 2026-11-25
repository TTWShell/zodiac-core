# Contributing

Requires Go 1.26.0 or newer.

## Check changes

```sh
go vet ./...
go test -race ./...
```

Format Go files with `gofmt` and run `go mod tidy` before submitting changes.
CI also generates and builds a temporary service, checks its health and OpenAPI
endpoints, and verifies that it shuts down successfully.
Tests and service checks run on Go 1.26.0 and the latest patches of Go 1.26 and
1.27, with automatic toolchain switching disabled.

## Preview documentation

```sh
python3 -m venv .venv-docs
.venv-docs/bin/python -m pip install -r docs/requirements.txt
.venv-docs/bin/mkdocs serve
```

Open the address printed by MkDocs. Validate documentation before submitting:

```sh
.venv-docs/bin/mkdocs build --strict
```

Write documentation in English for library users. Keep examples consistent with
the public API and update the relevant guide when behavior changes.

## Documentation publishing

In the GitHub repository's **Settings → Pages**, select **Deploy from a branch**,
then choose **gh-pages** and **/(root)**. The branch is created on the first
documentation publish. After changing the setting, rerun the publish job.

Pull requests build documentation without publishing. Pushes to `master` publish
`dev`. Publishing a non-prerelease GitHub Release builds documentation from its
tag and preserves previous versions. The release marked latest on GitHub updates
the `latest` alias and the site's default version.

The workflow pushes documentation to `gh-pages` and requests a Pages build.
Check the separate **pages build and deployment** run for the deployment result.
