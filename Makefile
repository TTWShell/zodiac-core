MKDOCS ?= .venv-docs/bin/mkdocs

.PHONY: docs-sync docs-serve docs-build

docs-sync:
	cp CHANGELOG.md docs/changelog.md

docs-serve: docs-sync
	$(MKDOCS) serve

docs-build: docs-sync
	$(MKDOCS) build --strict
