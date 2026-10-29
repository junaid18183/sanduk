# Demo project for sanduk.
#
# Run `make` on its own to list the targets below. The entry name comes from
# DEMO_ENTRY (exported by .envrc) and falls back to "demo".
#
# Nothing in this directory is secret: the values in demo.env are dummies, and
# no recipe ever prints a value.

SANDUK ?= sanduk
ENTRY  ?= $(or $(DEMO_ENTRY),demo)

.PHONY: help setup doctor show dev list add-legacy clean

help: ## list the targets below
	@grep -hE '^[a-z][a-z-]*:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  %-11s %s\n", $$1, $$2}'
	@echo
	@echo "entry: $(ENTRY)   project file: .sanduk.toml   sanduk: $(SANDUK)"

setup: ## load the dummy values from demo.env into the vault
	./setup.sh

doctor: ## compare the vault entry against .env.example
	$(SANDUK) doctor $(ENTRY) --template .env.example

show: ## print the entry's field names (never its values)
	$(SANDUK) group show -g $(ENTRY)

dev: ## run ./check.sh with the secrets injected into that process only
	$(SANDUK) exec $(ENTRY) -- ./check.sh

list: ## list the entries in the vault
	$(SANDUK) list

add-legacy: ## add the key `make doctor` reports as missing
	@echo "$(SANDUK) group add -g $(ENTRY) LEGACY_TOKEN --from-env LEGACY_TOKEN --replace"
	@LEGACY_TOKEN=demo-legacy-token-not-secret $(SANDUK) group add -g $(ENTRY) LEGACY_TOKEN --from-env LEGACY_TOKEN --replace

clean: ## remove the demo entry from the vault
	$(SANDUK) remove $(ENTRY) --yes
