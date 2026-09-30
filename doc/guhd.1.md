% guhd(1) User Commands

# NAME

guhd - Google unified heads up display

# SYNOPSIS

**guhd** [**--config** *PATH*] [**--setup**]

**guhd** **--help** | **--version**

# DESCRIPTION

**guhd** displays upcoming Google Calendar events, Gmail messages, and the latest commits from optional Git directories in a terminal. It uses **gog** for Google access and leaves authentication and credentials with gog. Install gog and authorize an account for Gmail and Calendar before setup.

On first use, choose one account and OAuth client, calendar IDs, a Gmail query, and optional project directories. Missing default configuration starts setup. Later launches open the dashboard. The dashboard requires interactive standard input and output. Help and version work without a terminal or gog.

Events cover the next 30 days, including events already in progress. All-day dates use the calendar's timezone and exclusive end date. Overlapping timed events receive a conflict label unless Google marks them as free. All-day events do not receive conflict labels. Each source has its own refresh schedule. Failed fetches keep the previous results and display an error.

The inbox loads up to 50 messages per page. The exact query **in:inbox** uses Gmail's whole-inbox message counts. Custom queries show unread counts among loaded messages. A separate indicator shows whether another page exists. Refresh replaces loaded inbox pages with the first page. Opening internal message details fetches the body without modifying labels.

Choose each project directory in setup or configuration. guhd reads each directory's latest Git commit and sorts projects by commit time. Empty repositories and inaccessible directories appear as project errors.

# INSTALLATION

## macOS

Install with Homebrew:

```sh
brew install openclaw/tap/gogcli
brew install cosgroveb/tap/guhd
guhd --version
man guhd
```

The formula installs gog from **openclaw/tap/gogcli** and Git. Authorize Gmail and Calendar with gog before running setup.

## Debian and Ubuntu

Download **guhd_0.1.0-1_amd64.deb** or **guhd_0.1.0-1_arm64.deb** from <https://github.com/cosgroveb/guhd/releases/tag/v0.1.0>. Choose the architecture reported by **dpkg --print-architecture**. Run these commands from the download directory:

```sh
sudo apt install ./guhd_0.1.0-1_$(dpkg --print-architecture).deb
guhd --version
man guhd
```

The package depends on Git and timezone data and recommends xdg-utils for opening items. Install and authorize gog before running setup. Minimal systems also need **man-db** to read the installed manual.

Release assets include **SHA256SUMS** for checksum verification.

## Source build

Install Go 1.26 or later and make. Install gog and authorize Gmail and Calendar before setup. Run these commands from the source checkout:

```sh
make build
./guhd --version
./guhd --setup
```

Project fetching requires Git. To install the binary under **/usr/local**, run **sudo make install**.

Use **PREFIX** to choose another installation prefix and **DESTDIR** to stage an installation. The install target copies the binary. To generate the manual, install pandoc and run **make man**.

## Debian package build

Build packages in Debian sid with the dependencies declared in **debian/control**, plus ca-certificates and lintian:

```sh
sudo apt-get update
sudo apt-get install -y ca-certificates git lintian
sudo apt-get build-dep -y .
make deb VERSION=0.1.0
```

The helper downloads Go modules, vendors them, and writes binary and source packages to **dist/**. The source package includes **guhd_0.1.0-1.dsc**, **guhd_0.1.0.orig.tar.gz**, and **guhd_0.1.0-1.debian.tar.xz**. With build dependencies installed, rebuild the extracted source without network access using **dpkg-buildpackage -us -uc**.

# SETUP

Put **gog** on **PATH** and authorize an account for Gmail and Calendar using gog's setup instructions. guhd uses gog's JSON commands and its **--readonly** flag. Install **git** to display projects.

Run **guhd --setup** to choose an account and its OAuth client. Use Up/Down or j/k to select a row, then Enter to continue. On the calendar screen, Space toggles each calendar.

Enter a Gmail search query, then optional project paths separated by semicolons. Setup resolves relative paths against the current directory. Enter on the project screen saves settings and opens the dashboard.

Shift-Tab returns to the previous step. Press r on the account or calendar screen to retry discovery. Escape or Ctrl-C exits without saving.

Setup does not edit the refresh interval. Edit **refresh_seconds** in the configuration file and restart guhd to change it.

# OPTIONS

**--help**, **-h**
: Print usage and exit.

**--version**
: Print version and exit.

**--config** *PATH*
: Use the selected JSON configuration. A missing explicit path is an error unless you also pass **--setup**.

**--setup**
: Open setup to edit configuration. Setup can create a missing file after validating settings. Escape cancels setup without saving.

# DASHBOARD KEYS

**Up**, **Down**, **k**, **j**
: Select an overview row or scroll details.

**PageDown**, **Space**, **f**
: Scroll details down one page.

**PageUp**, **b**
: Scroll details up one page.

**u**, **Ctrl-U**
: Scroll details up half a page.

**d**, **Ctrl-D**
: Scroll details down half a page.

**Tab**, **Shift-Tab**
: Change overview section.

**Enter**
: Open selected item details. Highlighting mail alone does not fetch its body.

**Escape**
: Return to the overview.

**r**
: Refresh sources that have no fetch in progress. Open details retain their current content.

**n**
: Append the next inbox page from the overview with the inbox section selected.

**o**
: Open the selected item with **open** on macOS or **xdg-open** on Linux. Remote links must use HTTP or HTTPS.

**y**
: Request copying the item's link or project path through the terminal's OSC52 clipboard protocol. guhd cannot confirm clipboard success. The full link or path remains available in details if the terminal ignores the request.

**?**
: Toggle help.

**q**, **Ctrl-C**
: Quit and cancel pending fetches.

# CONFIGURATION

The default file is **guhd/config.json** under the operating system's user configuration directory. On Linux, this follows **XDG_CONFIG_HOME** or **~/.config**. On macOS, it uses **~/Library/Application Support**. Saved files have mode 0600 and newly created directories have mode 0700. Configuration stores the settings below.

```json
{
  "account": "alex@example.com",
  "client": "default",
  "calendars": ["alex@example.com"],
  "mail_query": "in:inbox",
  "projects": ["/home/alex/code/example"],
  "refresh_seconds": 300
}
```

**account**
: One authorized Google email address.

**client**
: The gog OAuth client paired with that account. Default: **default**.

**calendars**
: Google Calendar IDs, preserved as supplied. An empty list disables event fetching.

**mail_query**
: Gmail search query. Default: **in:inbox**.

**projects**
: Absolute Git directory paths. An empty list hides the projects section.

**refresh_seconds**
: Positive integer refresh interval in seconds, measured from each fetch completion. Default: 300. The value must fit a Go time duration.

Unknown keys, malformed JSON, trailing JSON, and invalid settings cause a path-specific error. guhd reads configuration at startup.

# DATA AND EXTERNAL ACTIONS

Can:

- Read Calendar events and Gmail messages through gog.
- Read the latest commit in each configured Git directory.
- Save configuration, open links or directories, and request clipboard copies.

Cannot:

- Send mail, modify labels, or change Calendar events through guhd.
- Manage Google credentials or authorize accounts.

Opening Gmail in a browser can cause Gmail itself to mark a message read. guhd fetches message bodies on Enter and keeps details in memory. Detail previews truncate content after 128 KiB.

# ENVIRONMENT

**PATH**
: Must contain gog. Project fetching requires git, and opening items on Linux requires xdg-open.

**XDG_CONFIG_HOME**
: Sets the base configuration directory on Linux. The default is **~/.config**.

Child processes inherit guhd's environment, including gog authentication settings. Configure credentials through gog before launching guhd.

# EXIT STATUS

**0**
: Successful exit.

**1**
: Runtime or configuration error.

**2**
: Invalid command-line arguments.

# AUTHOR

Brian Cosgrove <cosgroveb@gmail.com>
