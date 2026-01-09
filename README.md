# ReadItLater

A simple command-line tool written in Go to quickly save URLs from your clipboard or command line into a local Org-mode file or Taskwarrior. It automatically fetches page titles and calculates reading times or video durations, adding them as metadata to the entry.

_This Go program was created by an AI, based on an initial bash script._

## Features

-   **Clipboard Integration:** Automatically reads a URL from the system clipboard if no argument is provided.
-   **Command-Line Input:** Accepts a URL directly as a command-line argument.
-   **Multiple Backend Support:** Choose between Org-mode files or Taskwarrior for storing your reading list.
-   **Org-mode Output:** Generates a new `LATER` heading in a specified Org-mode file with a link to the page.
-   **Taskwarrior Integration:** Stores URLs as tasks in Taskwarrior with custom UDA fields for URL, length, and creation date.
-   **Intelligent Tagging:**
    -   For webpages, it calculates the estimated reading time and adds a `:reading:` tag with a length category (`:short:`, `:mid:`, or `:long:`).
    -   For YouTube videos, it fetches the video duration and adds a `:video:` tag with a length category.
-   **Interactive Input:** Uses a terminal-based TUI with `tview` for adding custom tags and titles.
-   **Duplicate Checking:** Prevents adding the same URL twice by checking existing entries in your chosen backend.
-   **Mobile Compatibility:** The output file is in Org-mode format, which is supported by many applications on Android and iOS, making it convenient to read on mobile devices.
-   **Android/Termux Support:** The tool can be installed and used on Android devices via Termux, providing a consistent command-line experience for saving URLs on both desktop and mobile.
-   **Configuration:** Allows you to customize the backend and paths via a `config.json` file.


## Prerequisites

To use this tool, you need to have the following installed on your system:

-   **Go:** To build and install the program.
-   **A Clipboard Utility:** (optional) The program uses `wl-paste` by default for Wayland environments. You may need to adjust the `clipboardReader` function in the source code if you are using a different environment. Note that you can pass the URL as input, no need to access the clipboard.
-   **Taskwarrior:** (optional) Only required if you want to use the Taskwarrior backend instead of Org-mode files.



## Installation

1.  Clone the repository or download the source code.
2.  Build and install the program using the Go toolchain. This will place the `readitlater` executable in your `$GOPATH/bin` directory.
  ```
  go install
  ```
3.  Make sure your `$GOPATH/bin` is in your system's `PATH`.


## Configuration

The tool automatically generates a default configuration file located at `~/.config/readitlater/config.json`. You can modify this file to change the backend and paths.

**Default `config.json` (Org-mode backend):**

```json
{
  "backend": "org",
  "org_filepath": "~/.config/readitlater/ReadItLater.org",
  "org_archive_filepath": [
    "~/.config/readitlater/ReadItLater.org_archive",
    "~/.config/readitlater/Orgmode.org_archive"
  ]
}
```

-   `backend`: The backend to use for storing URLs. Options are `"org"` (default) or `"taskwarrior"`.
-   `org_filepath`: The path to the main Org-mode file where new entries will be saved (only used when backend is `"org"`).
-   `org_archive_filepath`: A list of Org-mode files to check for duplicate URLs (only used when backend is `"org"`).
-   `taskrc_path`: The path to the taskrc file for Taskwarrior (defaults to `$HOME/.taskrc`, only used when backend is `"taskwarrior"`).

**Taskwarrior Configuration:**

To use the Taskwarrior backend, change the `backend` field to `"taskwarrior"`:

```json
{
  "backend": "taskwarrior",
  "taskrc_path": "$HOME/.taskrc"
}
```

You also need to configure Taskwarrior with a custom UDA (User Defined Attribute) field. Add the following to your `~/.taskrc` file:

```
uda.url.type=string
uda.url.label=URL
```

When using the Taskwarrior backend:
- Tasks are created in the `readitlater` project
- The task description includes the duration and title in the format: `[Xm] Title`
- Tags include the duration category (`short`, `mid`, `long`), content type (`reading` or `video`), and any custom tags you provide
- The URL is stored in the custom `url` UDA field
- Taskwarrior's built-in creation date is used automatically


## Usage

### Saving a URL from the clipboard

Simply run the command without any arguments. The tool will automatically get the URL from your clipboard.

```
readitlater
```

### Saving a URL from the command line

Provide the URL as the first and only argument.

```
readitlater "https://your.favorite.website.com/article"
```


### Viewing Help

To see the usage instructions, use the `--help` flag.

```
readitlater --help
```


### Sample Entries

The tool will prompt you for custom tags (e.g., `tech` or `science` in the examples below) and will automatically append the appropriate `:reading:` or `:video:` tags.

**Org-mode Backend:**

After running the tool, a new entry will be appended to your `ReadItLater.org` file.

For a webpage:

    * LATER My Awesome Article :long:reading:tech:
      :PROPERTIES:
      :CREATED: 2023-10-27
      :LEN: 45
      :URL: https://your.favorite.website.com/article
      :COMMENT:
      :END:
      https://your.favorite.website.com/article


For a YouTube video:

    * LATER An interesting video - YouTube :mid:video:science:
      :PROPERTIES:
      :CREATED: 2023-10-27
      :LEN: 15
      :URL: https://www.youtube.com/watch?v=dQw4w9WgXcQ
      :COMMENT:
      :END:
      https://www.youtube.com/watch?v=dQw4w9WgXcQ


**Taskwarrior Backend:**

After running the tool, a new task will be created in Taskwarrior. You can view your reading list with:

```bash
task project:readitlater list
```

Example task output:
```
ID Project     Tags                    Description
1  readitlater long reading tech       [45m] My Awesome Article
2  readitlater mid video science       [15m] An interesting video - YouTube
```

The URL is stored in the custom `url` field. To view a specific task with all fields including the URL:
```bash
task 1 info
```



