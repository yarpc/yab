PACKAGES := $(shell glide novendor | grep -v '/testdata/')

PROTOC_VERSION := 3.6.1
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
    PROTOC_OS := linux
endif
ifeq ($(UNAME_S),Darwin)
    PROTOC_OS := osx
endif
PROTOC_FILE := protoc-${PROTOC_VERSION}-${PROTOC_OS}-$(shell uname -m).zip

export GO15VENDOREXPERIMENT=1

.DEFAULT_GOAL:=build


.PHONY: build
build:
	go build -i $(PACKAGES)
	go build -i .

.PHONY: install
install:
	glide --version || go get github.com/Masterminds/glide
	glide install


.PHONY: test
test:
	go test -cover -race $(PACKAGES)

.PHONY: install_protobuf
install_protobuf:
	curl -L https://github.com/google/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_FILE} -o /tmp/protoc.zip
	unzip -o /tmp/protoc.zip -d "${HOME}"/protoc

.PHONY: install_ci
install_ci: install install_protobuf
		go get github.com/wadey/gocovmerge
		go get github.com/mattn/goveralls
		go get golang.org/x/tools/cmd/cover


.PHONY: docs
docs:
	go install .
	# Automatically update the Usage section of README.md with --help (wrapped to 80 characters).
	screen -d -m bash -c 'stty cols 80 && ${GOPATH}/bin/yab --help | python -c "import re; import sys; f = open(\"README.md\"); r = re.compile(r\"\`\`\`\nUsage:.*?\`\`\`\", re.MULTILINE|re.DOTALL); print r.sub(\"\`\`\`\n\" + sys.stdin.read() + \"\`\`\`\", f.read().strip())" | sponge README.md'
	# Update our manpage output and HTML pages.
	$$GOPATH/bin/yab --man-page > man/yab.1
	groff -man -T html man/yab.1 > man/yab.html
	[[ -d ../yab_ghpages ]] && cp man/yab.html ../yab_ghpages/man.html
	@echo "Please update gh-pages"

.PHONY: test_ci
test_ci: install_ci build
	./scripts/cover.sh $(shell go list $(PACKAGES))

