% guhd(1) User Commands

# NAME

guhd - Google unified heads up display

# SYNOPSIS

**guhd** [**--config** *PATH*] [**--setup**]

**guhd** **--help** | **--version**

# DESCRIPTION

**guhd** displays upcoming Google Calendar events, Gmail messages, and optional recent Git projects in a terminal. It uses **gog** for Google access and leaves authentication and credentials with gog. Install gog and authorize an account for Gmail and Calendar before setup.

On first use, choose one account and OAuth client, calendar IDs, a Gmail query, and optional project directories. Missing default configuration starts setup. Later launches open the dashboard. The dashboard requires interactive standard input and output. Help and version work without a terminal or gog.

Events cover the next 30 days, including events already in progress. All-day dates use the calendar's timezone and exclusive end date. Overlapping timed events receive a conflict label. Each source refreshes independently and retains its last successful rows after a failed refresh.

The inbox loads up to 50 messages per page. The exact query **in:inbox** uses Gmail's whole-inbox message counts. Custom queries show unread counts among loaded messages, with a separate indication when another page exists. Opening internal message details fetches the body without modifying labels. Opening Gmail in a browser can cause Gmail itself to mark a message read.

Project directories are explicit. guhd reads each directory's latest Git commit without changing its working tree. Empty repositories and inaccessible directories appear as project errors.

# OPTIONS

**--help**, **-h**
: Print usage and exit.

**--version**
: Print version and exit.

**--config** *PATH*
: Use the selected JSON configuration. A missing explicit path is an error unless **--setup** is also supplied.

**--setup**
: Edit configuration interactively. A missing file can be created after settings validate. Escape cancels setup without saving.

# KEYS

**Up**, **Down**, **k**, **j**
: Select an overview row or scroll details.

**Tab**, **Shift-Tab**
: Change overview section.

**Enter**
: Open selected item details. Highlighting mail alone does not fetch its body.

**Escape**
: Return to the overview.

**r**
: Refresh available sources.

**n**
: Load the next inbox page when the inbox section is selected.

**o**
: Open the selected item with the system opener. Remote links must use HTTP or HTTPS.

**y**
: Request copying the item's link or project path through the terminal's OSC52 clipboard protocol. guhd cannot confirm clipboard success. The full link or path remains available in details if the terminal ignores the request.

**?**
: Toggle contextual help.

**q**, **Ctrl-C**
: Quit and cancel pending fetches.

# CONFIGURATION

The default file is **guhd/config.json** under the operating system's user configuration directory. On Linux, this follows **XDG_CONFIG_HOME** or **~/.config**. On macOS, it uses **~/Library/Application Support**. Saved files have mode 0600 and newly created directories have mode 0700. Configuration stores settings only, not credentials or fetched message/event bodies.

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
: Opaque Google Calendar IDs. An empty list disables event fetching.

**mail_query**
: Gmail search query. Default: **in:inbox**.

**projects**
: Absolute Git directory paths. An empty list hides the projects section.

**refresh_seconds**
: Positive refresh interval in seconds. Default: 300.

Unknown keys, malformed JSON, trailing JSON, and invalid settings cause a path-specific error. Set **NO_COLOR** to disable color. gog inherits the authentication environment used by your shell.

# EXIT STATUS

**0**
: Successful exit.

**1**
: Runtime or configuration error.

**2**
: Invalid command-line arguments.

# AUTHOR

Brian Cosgrove <cosgroveb@gmail.com>
