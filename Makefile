PACKAGES := $(shell glide novendor)

export GO15VENDOREXPERIMENT=1


.PHONY: build
build:
	go build -i $(PACKAGES)


.PHONY: install
install:
	glide --version || go get github.com/Masterminds/glide
	glide install


.PHONY: test
test:
	go test -cover -race $(PACKAGES)


.PHONY: install_ci
install_ci: install
		go get github.com/wadey/gocovmerge
		go get github.com/mattn/goveralls
		go get golang.org/x/tools/cmd/cover


.PHONY: update_man
update_man:
	go install .
	yab --man-page > man/yab.1
	nroff -Tascii -man man/yab.1 | man2html -title yab > man/yab.html
	[[ -d ../yab_ghpages ]] && cp man/yab.html ../yab_ghpages/man.html
	@echo "Please update gh-pages"

.PHONY: test_ci
test_ci: install_ci build
	./scripts/cover.sh $(shell go list $(PACKAGES))

