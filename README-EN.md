# GO RUNCARNATION

![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)
![GitHub License](https://img.shields.io/github/license/quadrans-muralis/runcarnation?style=flat-square&color=blue)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey?style=flat-square)
![Build Status](https://img.shields.io/github/actions/workflow/status/quadrans-muralis/runcarnation/ci.yml?branch=main&style=flat-square&logo=github)
![Last Commit](https://img.shields.io/github/last-commit/quadrans-muralis/runcarnation?style=flat-square&color=blue)

**"Always-on apps, reborn the moment a remote update is detected."**

GO RUNCARNATION is a CLI tool that monitors the state of a Git repository. When a new commit arrives on GitHub, it automatically executes **pull → rebuild → restart**. The name originates from combining `run` and `reincarnation`. It frees you from the manual effort of running `git pull` and restarting development servers, bots, or any always-on background processes.

It supports both an interactive prompt interface and a non-interactive CLI mode suitable for CI scripts and automation.

---

## Table of Contents

* [Features](#features)
* [Prerequisites](#prerequisites)
* [Installation](#installation)
* [Usage](#usage)
* [Interactive Mode](#interactive-mode)
* [Non-Interactive Mode (CLI Flags)](#non-interactive-mode-cli-flags)

* [Commands](#commands)
* [How `runcarnation` Works](#how-runcarnation-works)
* [Compiled Binary Location](#compiled-binary-location)
* [Language Switching](#language-switching)
* [License](#license)

---

## Features

* **Local ⇔ GitHub Status Check**
Instantly inspect branch status (ahead / behind), working tree cleanliness, remote URL, and diff stats with a single command.
* **`run` — Update once and execute**
Synchronizes with GitHub (pulling if necessary) prior to launching the application once.
* **`runcarnation` — Continuous monitoring and execution**
Runs the application while continuously watching the remote repository. Upon detecting a new commit, it automatically performs **Kill → pull → rebuild → restart**. Ideal for active development servers and persistent bots.
* **Clean execution based on `go build**`
Instead of `go run .`, it compiles an executable binary each time and manages the process directly, ensuring clean and reliable process termination.
* **Persistent binary storage & bulk cleanup**
Compiled binaries are stored in OS-standard configuration directories and can be cleared at any time using the `demolition` command.
* **Multilingual Support**
Switch between English, Japanese, and Chinese seamlessly with a single command.
* **Command Aliases**
Longer command names can be executed using short aliases (e.g., `check` → `chk`).
* **Interactive and Non-Interactive Modes**
Launch without arguments to use the interactive prompt, or pass arguments directly for automation and single-shot execution.

---

## Prerequisites

While this tool runs as a precompiled binary, **it invokes the following external utilities at runtime**. Ensure they are installed and available in your `PATH`:

| Command | Purpose |
| --- | --- |
| `git` | Used for status checks, fetching, pulling, diffing, and core Git operations. |
| `go` | Used for compiling binaries (`go build`) during `run` and `runcarnation`. |

The target repository directory must meet the following criteria:

* Must be a valid Git repository (`git init` executed).
* Must have an `origin` remote configured.
* For `run` and `runcarnation`, the root directory must contain a Go project with a `main` package.

> **Note:** Git 2.22 or higher is required as branch detection relies on `git branch --show-current`.

---

## Installation

### Building from Source

```bash
git clone https://github.com/yourname/go-runcarnation.git
cd go-runcarnation
go build -o go-runcarnation .

```

### Using Pre-built Binaries

Download the binary matching your OS from the GitHub Releases page, extract it, and place it in a directory included in your system `PATH`.

---

## Usage

### Interactive Mode

Running the tool without arguments launches the interactive prompt `GO RUNCARNATION>`. Enter commands at the prompt:

```bash
./go-runcarnation

```

```
GO RUNCARNATION> help

```

Commands such as `check`, `status`, `remote`, `fetch`, `diff`, `run`, and `runcarnation` will prompt for the target repository path:

```
GO RUNCARNATION> check
Repository path: /path/to/your/repo

```

### Non-Interactive Mode (CLI Flags)

When calling from scripts or CI pipelines, pass the subcommand and the `--path` flag directly. This executes the command without waiting for user input.

```bash
./go-runcarnation check --path=/path/to/your/repo
./go-runcarnation run --path=/path/to/your/repo

```

The display language can also be set on the fly using `--lang`:

```bash
./go-runcarnation check --path=/path/to/your/repo --lang=en

```

Commands that do not require a repository path (such as `help`, `demolition`, or `language`) can be executed without `--path`:

```bash
./go-runcarnation demolition

```

---

## Commands

| Command | Short Alias | Description |
| --- | --- | --- |
| `check` | `chk` | Scans the repository and compares it against GitHub (ahead / behind status). |
| `status` | `stat` | Checks the local working tree for uncommitted changes. |
| `remote` | `rmt` | Displays the `origin` remote URL. |
| `fetch` | `fch` | Fetches the latest updates from GitHub (does not pull). |
| `diff` | - | Displays diff statistics (`--stat`) between local and GitHub. |
| `ignored` | `ign` | Lists all files ignored by `.gitignore` with absolute paths. |
| `changed` | `chg` | Lists all modified, uncommitted files with absolute paths. |
| `run` | - | Syncs with GitHub (pulls if behind), then executes the application once. |
| `runcarnation` | `rcn` | Runs the app while polling GitHub; auto-restarts upon detecting updates. |
| `demolition` | `demo` | Deletes all saved pre-compiled binaries. |
| `language` | `lang` | Switches interface language (`en` / `jp` / `cn`). |
| `help` | - | Displays the list of available commands. |
| `exit` | - | Exits the CLI. |

---

## How `runcarnation` Works

```
1. Fetch remote → pull changes if local is behind
2. Generate temporary binary using `go build`
3. Launch the binary
4. Monitor GitHub every 5 seconds (pollInterval)
     ├─ New commit detected → Kill process → Return to step 1 (pull, rebuild, restart)
     └─ App exits naturally (success or crash) → Loop terminates

```

Remote-triggered restarts are explicitly distinguished from normal application exits. **If the application terminates on its own, the monitoring loop stops**, preventing unintended auto-restarts.

---

## Compiled Binary Location

Temporary binaries generated by `run` and `runcarnation` are stored in standard OS configuration paths:

| OS | Directory Path |
| --- | --- |
| Windows | `%APPDATA%\GO_RUNCARNATION\runcarnation` |
| macOS | `~/Library/Application Support/GO_RUNCARNATION/runcarnation` |
| Linux | `$XDG_CONFIG_HOME/GO_RUNCARNATION/runcarnation` (typically `~/.config/...`) |

Binaries are automatically cleaned up after exit. Any lingering files caused by crashes can be purged manually using the `demolition` command:

```
GO RUNCARNATION> demolition

```

Execution logs are stored in the same directory structure under `GO_RUNCARNATION/log`. Logs older than 14 days are automatically purged upon application startup.

---

## Language Switching

```
GO RUNCARNATION> language
  en. English
  jp. 日本語
  cn. 中文

Select language: en
Language changed to English.

```

---

## License

See the `LICENSE` file for details.
