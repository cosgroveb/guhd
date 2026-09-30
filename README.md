# guhd

guhd (Google unified heads up display) shows Calendar events and Gmail messages in a terminal. Add Git directories to see each project's latest commit. Google authentication stays in [gog](https://github.com/openclaw/gogcli).

## Install

On macOS, install with Homebrew. On Debian or Ubuntu, download the matching `.deb` from [v0.1.0](https://github.com/cosgroveb/guhd/releases/tag/v0.1.0), then install it with apt. Debian and Ubuntu also need gog installed.

```sh
# macOS
brew install cosgroveb/tap/guhd
# Debian or Ubuntu, from the download directory
sudo apt install ./guhd_0.1.0-1_$(dpkg --print-architecture).deb
```

## Use

Authorize Gmail and Calendar with gog, then run setup. Later launches use `guhd`. See [guhd(1)](doc/guhd.1.md) for keys, configuration, and source builds, or [AGENTS.md](AGENTS.md) for development.

```sh
guhd --setup
```
