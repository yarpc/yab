.DEFAULT_GOAL:=build


.PHONY: build
build:
	go build ./...

.PHONY: install
install:
	go mod download


.PHONY: test
test:
	go test -cover -race ./...


.PHONY: docs
docs:
	go build
	# Automatically update the Usage section of README.md with --help (wrapped to 80 characters).
	screen -d -m bash -c 'stty cols 80 && $(CURDIR)/yab --help | python -c "import re; import sys; f = open(\"README.md\"); r = re.compile(r\"\`\`\`\nUsage:.*?\`\`\`\", re.MULTILINE|re.DOTALL); print r.sub(\"\`\`\`\n\" + sys.stdin.read() + \"\`\`\`\", f.read().strip())" | sponge README.md'
	# Update our manpage output and HTML pages.
	$(CURDIR)/yab --man-page > man/yab.1
	groff -man -T html man/yab.1 > man/yab.html
	[[ -d ../yab_ghpages ]] && cp man/yab.html ../yab_ghpages/man.html
	@echo "Please update gh-pages"

.PHONY: test_ci
test_ci: install build
	go test -race -coverprofile=cover.out -coverpkg=./... ./...
	go tool cover -html=cover.out -o cover.html
