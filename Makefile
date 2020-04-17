GOTFLATS ?=
SHELL = /bin/bash

define eachmod
	@echo '$(1)'
	@find . -type f -name go.mod -print0 | xargs -I '{}' -n1 -0 bash -c 'dir="$$(dirname {})" && echo "$${dir}" && cd "$${dir}" && $(1)'
endef

.PHONY: install tidy mod-download lint build-all docker install test

install: goinstall docker

goinstall:
	go install .

pre-commit:
	python -m pip install pre-commit --upgrade --user
	pre-commit install --install-hooks

tidy:
	$(call eachmod,go mod tidy)

mod-download:
	$(call eachmod,go mod download)

lint:
	$(call eachmod,GOGC=75 golangci-lint run --build-tags balsam --concurrency 32 --deadline 4m ./...)

build-all:
	$(call eachmod,go build -tags balsam -o /dev/null ./...)

docker:
	docker build -t ipfs/testground .

test:
	$(call eachmod,go test -tags balsam -p 1 -v $(GOTFLAGS) ./...)
