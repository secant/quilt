export GO15VENDOREXPERIMENT=1
PACKAGES=$(shell GO15VENDOREXPERIMENT=1 go list ./... | grep -v vendor)
NOVENDOR=$(shell find . -path ./vendor -prune -o -name '*.go' -print)
LINE_LENGTH_EXCLUDE=./constants/awsConstants.go \
		    ./constants/azureDescriptions.go \
		    ./constants/gceConstants.go \
		    ./cluster/provider/cloud_config.go \
		    ./minion/network/link_test.go \
		    ./minion/pb/pb.pb.go \

REPO = quilt
DOCKER = docker
SHELL := /bin/bash

.PHONY: all
all: quilt minion

.PHONY: quilt
quilt:
	cd -P . && go build .

.PHONY: minion
minion:
	cd -P . && go build -o ./minion/minion ./minion

install:
	cd -P . && go install . && go install ./inspect && go install ./minion

check: format-check
	go test $(PACKAGES)

COV_SKIP= /minion/pb /minion/pprofile /constants /scripts /quilt-tester \
		  /quilt-tester/tests/basic/src/docker_test \
		  /quilt-tester/tests/basic/src/log_test \
		  /quilt-tester/tests/spark/src/spark_test_monly

COV_PKG = $(subst github.com/NetSys/quilt,,$(PACKAGES))
coverage: $(addsuffix .cov, $(filter-out $(COV_SKIP), $(COV_PKG)))
	gover

%.cov:
	go test -coverprofile=.$@.coverprofile .$*
	go tool cover -html=.$@.coverprofile -o .$@.html

format: scripts/format
	gofmt -w -s $(NOVENDOR)
	scripts/format $(filter-out $(LINE_LENGTH_EXCLUDE),$(NOVENDOR))

scripts/format: scripts/format.go
	cd scripts && go build format.go

format-check:
	RESULT=`gofmt -s -l $(NOVENDOR)` && \
	if [[ -n "$$RESULT"  ]] ; then \
	    echo $$RESULT && \
	    exit 1 ; \
	fi

lint: format
	cd -P . && go vet $(PACKAGES)
	for package in $(PACKAGES) ; do \
		if [[ $$package != *minion/pb* ]] ; then \
			golint -min_confidence .25 $$package ; \
		fi \
	done

generate:
	go generate $(PACKAGES)

providers:
	python3 scripts/gce-descriptions > provider/gceConstants.go

# BUILD
docker-build-all: docker-build-tester docker-build-minion docker-build-ovs

docker-build-tester:
	cd -P quilt-tester && ${DOCKER} build -t ${REPO}/tester .

docker-build-minion:
	cd -P minion && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build . \
	 && ${DOCKER} build -t ${REPO}/minion .

docker-build-ovs:
	cd -P ovs && docker build -t ${REPO}/ovs .

# PUSH
#
docker-push-all: docker-push-tester docker-push-minion
	# We do not push the OVS container as it's built by the automated
	# docker hub system.

docker-push-tester:
	${DOCKER} push ${REPO}/tester

docker-push-minion:
	${DOCKER} push ${REPO}/minion

docker-push-ovs:
	${DOCKER} push ${REPO}/ovs

# Include all .mk files so you can have your own local configurations
include $(wildcard *.mk)
