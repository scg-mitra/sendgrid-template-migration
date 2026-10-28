<img src="assets/app-icon.png" width="64" height="64" alt="">

# SendGrid Template Migration

Copy SendGrid templates, including every version, from one account to another. For example, from a parent account to a subuser.

![Choosing templates to copy](assets/screenshot.png)

## Download & install

Get the latest build from the [Releases](../../releases) page:

| OS | File | Install |
| --- | --- | --- |
| Windows 10/11 | `..._windows_amd64_setup.exe` | Run it. Adds the app to the Start menu and desktop. No admin rights needed. Uninstall from **Settings → Apps**. |
| Windows (no install) | `..._windows_amd64_portable.zip` | Unzip and double-click `sendgrid-template-migration.exe`. |
| macOS (Apple Silicon) | `..._macos_arm64.dmg` | Open it and drag the app into **Applications**. |
| Fedora | `....x86_64.rpm` | `sudo dnf install ./sendgrid-template-migration-*.x86_64.rpm` |
| Debian / Ubuntu | `..._amd64.deb` | `sudo apt install ./sendgrid-template-migration_*_amd64.deb` |

On Linux, the app then appears in your app launcher as **SendGrid Template Migration**. It needs glibc 2.34 or newer and WebKit2GTK 4.1; the package pulls in WebKit2GTK automatically. Tested on Fedora 44 and Ubuntu 24.04. It should also work on Ubuntu 22.04, the system it is built on.

On Windows it uses the Microsoft Edge WebView2 runtime, which comes with Windows 11 and up-to-date Windows 10.

The builds aren't code-signed, so the OS warns the first time you open them:

- **Windows:** SmartScreen says "Windows protected your PC". Click **More info** → **Run anyway**.
- **macOS:** It says the app "can't be opened" or "is damaged". Run `xattr -dr com.apple.quarantine "/Applications/SendGrid Template Migration.app"` once in Terminal. Or open **System Settings → Privacy & Security** and click **Open Anyway**.

## Usage

Open the app from your app launcher, Start menu or Applications folder. It walks you through three steps:

1. **Connect:** enter the source (parent) API key, then choose how to reach the subuser:
   - **Subuser API key:** the subuser's own key, with full access to Template Engine.
   - **Parent key + subuser username:** the parent key acts as the subuser (SendGrid's `on-behalf-of`), so you don't need a separate subuser key. **Find subusers** lists the parent's active subusers to pick from (needs read access to Subuser Management).

   Both accounts are checked before you continue. Keys are never saved.
2. **Choose templates:** search, select and copy all versions or just the active one. Templates whose name already exists in the destination are flagged.
3. **Copy:** live progress, then each template's new ID with a copy button. **Copy ID list** puts `name, old ID, new ID` on the clipboard.

The copy gets a new template ID in the destination account, so update any code that references the old ID.

### Command line

Pass `-template` to copy without opening the window, e.g. from a script:

```sh
export SENDGRID_SRC_API_KEY=SG.parent...   # needs Template Engine read access

# Using the subuser's own key:
SENDGRID_DST_API_KEY=SG.subuser... sendgrid-template-migration -template d-1234567890abcdef
# Or using the parent key on behalf of a subuser:
sendgrid-template-migration -dst-subuser acme-marketing -template d-1234567890abcdef
```

| Flag | Description |
| --- | --- |
| `-template` | Template ID to copy. Repeat the flag or pass a comma-separated list. |
| `-src-key` | Source (parent) API key. Env: `SENDGRID_SRC_API_KEY`. |
| `-dst-key` | Destination subuser's API key. Env: `SENDGRID_DST_API_KEY`. |
| `-dst-subuser` | Copy to this subuser using the source key instead of `-dst-key`. Env: `SENDGRID_DST_SUBUSER`. |
| `-name` | Name for the copy. Works only with a single template. |
| `-active-only` | Copy only the active version. |
| `-base-url` | Defaults to `https://api.sendgrid.com`. Use `https://api.eu.sendgrid.com` for EU accounts. |
| `-version` | Print the version. |

Without `-template`, the window opens with the keys, subuser and `-base-url` pre-filled from these flags and environment variables.

## Development

The window is a [Wails v3](https://v3.wails.io) app. The UI is React + TypeScript + [Tailwind CSS](https://tailwindcss.com) + [shadcn/ui](https://ui.shadcn.com) in `frontend/`, built with Vite and embedded in the Go binary.

| Path | What |
| --- | --- |
| `main.go` | Flags; opens the window, or copies directly when `-template` is given |
| `sendgrid.go` | SendGrid API client |
| `connection.go` | Source/destination accounts, including the on-behalf-of mode |
| `app.go` | The backend service the UI calls (`Connect`, `CopyTemplates`, …) |
| `frontend/src/screens/` | The three screens |
| `frontend/src/bindings/` | TypeScript bindings generated from `app.go`. Run `make bindings` after changing it. |
| `packaging/` | Build scripts, Linux `.desktop` file and package config, Windows installer script |

Install the toolchain: Go, Node.js 20.19+ or 22.12+ (needed by Vite), and the system web view headers.

```sh
# Fedora (its nodejs package is new enough and includes npm)
sudo dnf install gcc make nodejs gtk3-devel webkit2gtk4.1-devel
# Ubuntu / Debian (their nodejs packages are too old: install Node from https://nodejs.org or nvm)
sudo apt install gcc make libgtk-3-dev libwebkit2gtk-4.1-dev
# Bindings generator, only needed for `make bindings`
CGO_ENABLED=0 go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
```

```sh
make dev       # same as F5 in VS Code, without the debugger
make test      # go test -race
make lint      # golangci-lint (install it separately) + oxlint
make bindings  # regenerate frontend/src/bindings after changing app.go
make build     # dist/sendgrid-template-migration
```

To use `go` directly, build the frontend first (`make frontend`), because the binary embeds `frontend/dist`. On Linux, also pass `-tags gtk3`; without it, Wails expects GTK4 and WebKitGTK 6.0.

### VS Code

The repo includes a VS Code setup in `.vscode/`. Open the project folder and install the recommended extensions when prompted: Go, Tailwind CSS IntelliSense and Oxc. The first time you debug, the Go extension also offers to install its debugger (Delve); accept it.

Press **F5** (or **Run and Debug → App (live UI)**). VS Code then:

1. Builds the frontend once if `frontend/dist` doesn't exist yet. The Go binary embeds it, so Go can't compile without it.
2. Starts the Vite dev server on port 34115 as a background task.
3. Runs the app under the Go debugger, with its UI served by Vite.

While it runs:

- **Go:** breakpoints in `app.go`, `connection.go` etc. work as usual. Restart (**Ctrl+Shift+F5**) after changing Go code.
- **React:** edits in `frontend/src` appear in the window immediately, with no restart.
- **Inspecting the UI:** right-click in the window → **Inspect Element** opens the web inspector for the page, its console and network requests.
- **Changing what `app.go` exposes to the UI:** run **Terminal → Run Task → bindings: regenerate**, then restart.

The Vite server keeps running between debug sessions; stop it with **Terminal → Terminate Task**. The other debug configurations are **App (built UI)**, which runs with the frontend embedded as in a release, and **Go tests**. Test and lint are also available as tasks.

`.vscode/settings.json` sets the `gtk3` build tag for the Go extension, so it doesn't report errors about missing GTK4 packages.

## Releases (GitHub Actions)

Push a version tag to build every platform and publish a GitHub Release:

```sh
git tag v1.0.0
git push origin v1.0.0
```

Every push to `main` and every pull request also builds everything. Those files are attached to the workflow run (**Actions** tab → run → **Artifacts**) and no release is published. See `.github/workflows/release.yml`.

| Job | Runner | Produces |
| --- | --- | --- |
| Linux + Windows | `ubuntu-22.04` | `.deb`, `.rpm`, `.tar.gz`; Windows installer and portable `.zip` (Wails on Windows needs no C compiler, so it cross-compiles) |
| macOS | `macos-latest` (Apple Silicon) | `.dmg` containing the `.app` |

The Linux job also checks that `frontend/src/bindings` matches `app.go`, lints the frontend, and runs the Go tests with the race detector.

To build installers locally: `make linux` (needs `nfpm`), `make windows` (needs `nsis`, `zip`, `go-winres`), or `make macos` (on a Mac).
