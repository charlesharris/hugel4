.PHONY: build test vet fmt clean install

BIN     ?= $(HOME)/.local/bin/hugel4
SKILLS  ?= $(HOME)/.claude/skills

build:
	go build -o bin/hugel ./cmd/hugel

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Installs as hugel4, not hugel: an earlier generation still owns that name on
# PATH and is wired into live hooks. The skill is symlinked so edits here take
# effect without reinstalling.
install: build
	mkdir -p $(dir $(BIN)) $(SKILLS)
	cp bin/hugel $(BIN)
	ln -sfn $(CURDIR)/skills/hugel-soil $(SKILLS)/hugel-soil
	ln -sfn $(CURDIR)/skills/hugel-beads $(SKILLS)/hugel-beads
	@echo "installed $(BIN) and skills into $(SKILLS)"

clean:
	rm -rf bin
