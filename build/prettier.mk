.PHONY: prettier-install prettier-check prettier-format me prettier
prettier-install:
	npm install --global prettier

prettier-check:
	npx prettier --check "*/**/*.{ts,js,mjs,json}"

prettier-format:
	npx prettier --write "*/**/*.{ts,js,mjs,json}"

me:
	@echo "🪄💄🪄💄🪄💄"

prettier:
	@npx prettier --write "*/**/*.{ts,js,mjs,json}"