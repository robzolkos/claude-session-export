# claude-session-export

A fast, lightweight Go CLI tool that exports Claude Code sessions to GitHub Gist for sharing and viewing.

## Features

- **Zero external dependencies** - Built entirely with Go's standard library
- **Interactive session picker** - Browse and select from your local Claude Code sessions
- **GitHub Gist publishing** - One-command upload with built-in viewer
- **Session search** - Search across all sessions for specific terms
- **Claude API support** - Fetch sessions directly from the Claude web interface
- **Built-in viewer** - Modern, sophisticated session viewer with:
  - Collapsible conversation view (user messages as entry points)
  - Session statistics (duration, active time, tokens, message counts)
  - Tool visualization with icons
  - Markdown rendering
  - Copy URL button for sharing

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap robzolkos/tap
brew install claude-session-export
```

### Arch Linux (AUR)

```bash
yay -S claude-session-export-bin
```

### Using Go Install

```bash
go install github.com/robzolkos/claude-session-export@latest
```

### Building from Source

```bash
git clone https://github.com/robzolkos/claude-session-export
cd claude-session-export
go build -o claude-session-export .
```

### Verify Installation

```bash
claude-session-export --version
```

## Quick Start

```bash
# Interactive picker - uploads to Gist and opens viewer
claude-session-export

# Export a specific JSONL file to Gist
claude-session-export json my-session.jsonl

# Search across all your sessions
claude-session-export search "database migration"

# Save locally instead of uploading
claude-session-export -o ./output
```

## Commands

### `local` (default)

Browse and export sessions from your local Claude Code installation (`~/.claude/projects`). Uploads to GitHub Gist and opens the viewer by default.

```bash
claude-session-export                    # Interactive picker, upload to Gist
claude-session-export local              # Same as above
claude-session-export local --limit 50   # Show more sessions in picker
claude-session-export -o ./output        # Save locally instead
```

### `json`

Export a specific JSON or JSONL session file. Uploads to GitHub Gist by default.

```bash
# Export local file to Gist
claude-session-export json session.jsonl

# Export from URL
claude-session-export json https://example.com/session.jsonl

# Save locally instead of uploading
claude-session-export json session.jsonl -o ./output
```

### `web`

Fetch and export sessions from the Claude API (requires authentication). Uploads to GitHub Gist by default.

```bash
# Fetch a session by ID and upload to Gist
claude-session-export web abc123-session-id
```

### `search`

Search across all your Claude Code sessions for a specific term.

```bash
# Search for a term across all sessions
claude-session-export search "burrito"

# Example output:
# Found "burrito" in 3 sessions:
#
#  1. [Jan 05] -home-rzolkos-code-myapp (2 matches)
#     "...let's grab a burrito for lunch and then..."
#     "...the burrito API endpoint needs..."
#
#  2. [Jan 03] -home-rzolkos-code-backend (1 match)
#     "...like wrapping a burrito, we need to..."
#
# Enter number to export (or q to quit):

# Limit the number of snippet previews per session
claude-session-export search "refactor" --max-matches 5
```

### `open`

Open a gist URL in the session viewer.

```bash
claude-session-export open https://gist.github.com/user/gist-id
```

## Command Line Options

| Option | Short | Description |
|--------|-------|-------------|
| `--output DIR` | `-o` | Save JSONL locally instead of uploading to Gist |
| `--no-open` | | Don't open viewer after uploading |
| `--limit N` | | Maximum sessions to show in picker (default: 20) |
| `--max-matches N` | | Maximum snippets to show per session in search (default: 3) |
| `--help` | `-h` | Show help message |
| `--version` | `-v` | Show version number |

## Environment Variables

### Claude API Access

For the `web` command to fetch sessions from Claude's API:

| Variable | Description |
|----------|-------------|
| `CLAUDE_ACCESS_TOKEN` | Your Claude API access token |
| `CLAUDE_ORG_UUID` | Your Claude organization UUID |

These can also be read from `~/.claude.json` or (on macOS) from the system keychain.

### GitHub Gist

The `--gist` option requires the [GitHub CLI](https://cli.github.com/) (`gh`) to be installed and authenticated:

```bash
# Install gh (macOS)
brew install gh

# Authenticate
gh auth login
```

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o claude-session-export .
```

### Releasing

Releases are automated via GoReleaser and GitHub Actions. To create a new release:

```bash
git tag v1.0.0
git push origin v1.0.0
```

### Project Structure

```
claude-session-export/
├── main.go                     # Entry point
├── go.mod                      # Module definition
├── internal/
│   ├── cli/                    # Command-line interface
│   │   ├── cli.go              # Command handling
│   │   ├── cli_test.go
│   │   ├── embed.go            # Viewer embedding
│   │   └── viewer.html         # Session viewer
│   ├── session/                # Session parsing
│   │   ├── types.go            # Data structures
│   │   ├── parse.go            # JSON/JSONL parsing
│   │   ├── parse_test.go
│   │   └── discover.go         # Local session discovery
│   ├── gist/                   # GitHub Gist integration
│   │   └── gist.go
│   └── web/                    # Claude API client
│       └── web.go
└── README.md
```

## Troubleshooting

### "no sessions found"

Make sure Claude Code has been used and sessions exist in `~/.claude/projects/`.

### "gh CLI not found"

Install the GitHub CLI for `--gist` functionality:
- macOS: `brew install gh`
- Linux: See https://cli.github.com/

### "no access token found"

For the `web` command, set `CLAUDE_ACCESS_TOKEN` or authenticate Claude Code.

## License

Apache 2.0

## Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.
