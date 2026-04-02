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
