## Overrides

Added

```json
"overrides": {
		"cookie": "^0.7.2"
	}
```

to `package.json` in response to Dependabot alert "cookie accepts cookie name, path, and domain with out of bounds characters" as latest version of SvelteKit is not yet compatible with the patched version.

## Scripts

Run `./docs/dev.sh` to launch Go server and Vite dev server.

Run `npm run db:apply-schema` to reset the database.

## Bugs

Get the chat text to disappear before the role menu items appear when switching from chat to roles view. Currently, the chat text is still there and so we see the roles appear underneath it, then jump up when the chat text goes.
