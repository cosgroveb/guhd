# guhd

guhd (Google unified heads up display) shows Calendar events and Gmail messages in a terminal. Add Git directories to see each project's latest commit. Google authentication stays in [gog](https://github.com/openclaw/gogcli).

## Build and setup

Install Go 1.26 or later, make, and gog. Authorize your Google account for Gmail and Calendar with gog, then build guhd from this checkout. Setup saves your choices and opens the dashboard.

```sh
make build
./guhd --setup
```

## Use

Run `./guhd` from this checkout to reopen the dashboard. Press Enter for details, `r` to refresh, `?` for help, and `q` to quit.

See [guhd(1)](doc/guhd.1.md) for setup, keys, and configuration. Developer instructions are in [AGENTS.md](AGENTS.md).
