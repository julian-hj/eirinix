all: test-unit build

export NAMESPACE ?= eirini

gen-fakes:
	bin/gen-fakes

generate: gen-fakes

vet:
	bin/vet

lint:
	bin/lint

test-unit:
	bin/test-unit

test-integration:
	bin/test-integration

test-e2e:
	bin/test-e2e

test: vet lint test-unit

tools:
	bin/tools

check-scripts:
	bin/check-scripts
