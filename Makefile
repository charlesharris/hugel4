.PHONY: build test vet fmt clean install

BIN     ?= $(HOME)/.local/bin/hugel
SKILLS  ?= $(HOME)/.claude/skills
COMPDIR ?= $(HOME)/.local/share/zsh/site-functions

build:
	go build -o bin/hugel ./cmd/hugel

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# Installs as hugel. The name was held by a generation archived on 2026-05-04,
# whose binary now sits beside this one as hugel-archived with its hooks
# repointed there -- archived is not dead, and eight of them still run. This
# source has always called itself hugel: every error prefix, both TUI titles and
# the tender's brief say so, and none of them ever said hugel4.
# The skill is symlinked so edits here take effect without reinstalling.
# The completion script is generated at install time rather than checked in,
# so it can never describe a flag this binary does not have.
install: build
	mkdir -p $(dir $(BIN)) $(SKILLS) $(COMPDIR)
	cp bin/hugel $(BIN)
	./bin/hugel completion zsh > $(COMPDIR)/_hugel
	ln -sfn $(CURDIR)/skills/hugel-soil $(SKILLS)/hugel-soil
	ln -sfn $(CURDIR)/skills/hugel-beads $(SKILLS)/hugel-beads
	@echo "installed $(BIN) and skills into $(SKILLS)"
	@echo "completion  $(COMPDIR)/_hugel"
	@case ":$$FPATH:" in *":$(COMPDIR):"*) ;; *) \
	  echo ""; \
	  echo "  $(COMPDIR) is not on \$$fpath. Add this above compinit in ~/.zshrc:"; \
	  echo "    fpath=($(COMPDIR) \$$fpath)";; \
	esac

clean:
	rm -rf bin
