# guhd

Google unified heads up display shows upcoming Calendar events, Gmail messages, and recent commits from selected Git directories in a terminal pane. Google authentication stays in [gog](https://github.com/openclaw/gogcli).

## Build and setup

Install gog and authorize a Google account for Gmail and Calendar. Build guhd with Go 1.26 or later, then run setup in a terminal.

```sh
make build
./guhd --setup
```

## Use

Run `guhd` to open the dashboard. Press `?` for help, Enter for details, and `q` to quit. Details keep their place during refresh, and failed sources retain their last successful rows.

See [guhd(1)](doc/guhd.1.md) for keys, configuration, and prerequisites. Run `make check` for formatting, lint, tests, documentation, and build checks.
