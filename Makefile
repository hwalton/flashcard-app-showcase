# Project-wide vars
TAILWIND = npx @tailwindcss/cli
CONFIG    = src/tailwind.config.js
INPUT     = src/frontend/static/tailwind/input.css
OUTPUT    = src/frontend/static/tailwind/output.css

.PHONY: install watch-css build-css

install:
	npm i

watch-css:
	$(TAILWIND) \
	  -c $(CONFIG) \
	  -i $(INPUT) \
	  -o $(OUTPUT) \
	  --watch

build-css:
	$(TAILWIND) \
	  -c $(CONFIG) \
	  -i $(INPUT) \
	  -o $(OUTPUT) \
	  --minify

