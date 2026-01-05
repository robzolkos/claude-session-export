# claude-session-export

A fast, lightweight Go CLI tool that transforms Claude Code session files into shareable, searchable HTML documentation.

Inspired by [simonw/claude-code-transcripts](https://github.com/simonw/claude-code-transcripts). This project adds:

- Zero external dependencies (single Go binary)
- Built-in full-text search across transcript pages
- CLI search command to find terms across all sessions
- Session duration tracking
- Gists are private by default with `--public` flag option
- Synthwave 84 styling

## Features

- **Zero external dependencies** - Built entirely with Go's standard library
- **Multiple input formats** - Supports both JSON and JSONL session files
- **Paginated HTML output** - Clean, navigable pages (5 conversations per page)
- **Interactive index** - Timeline view of all prompts and git commits
- **Full-text search** - Search across all transcript pages with highlighted results
- **Mobile-friendly** - Responsive design that works on any device
- **Tool visualization** - Rich rendering for Bash, Write, Edit, Read, Grep, TodoWrite, and more
- **Git integration** - Automatically detects commits and links to GitHub repositories
- **GitHub Gist publishing** - One-command upload with shareable preview URLs
- **Batch processing** - Convert your entire session history into a browsable archive
- **Claude API support** - Fetch sessions directly from the Claude web interface

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
# Convert a session file to HTML
claude-session-export json my-session.jsonl

# Interactive picker for local Claude Code sessions
claude-session-export

# Convert all your sessions to a browsable archive
claude-session-export all --open
```

## Commands

### `local` (default)

Browse and convert sessions from your local Claude Code installation (`~/.claude/projects`).

```bash
claude-session-export                    # Interactive session picker
claude-session-export local              # Same as above
claude-session-export local --limit 50   # Show more sessions in picker
```

### `json`

Convert a specific JSON or JSONL session file.

```bash
# Convert local file
claude-session-export json session.jsonl

# Specify output directory
claude-session-export json session.json -o ./my-transcript

# Convert from URL
claude-session-export json https://example.com/session.json

# Upload result to GitHub Gist (private by default)
claude-session-export json session.jsonl --gist

# Upload as a public gist
claude-session-export json session.jsonl --gist --public

# Include original session file in output
claude-session-export json session.jsonl --include-json
```

### `web`

Fetch and convert sessions from the Claude API (requires authentication).

```bash
# Fetch a session by ID
claude-session-export web abc123-session-id

# Fetch and upload to Gist
claude-session-export web abc123-session-id --gist --open
```

### `all`

Convert all local sessions into a hierarchical, browsable archive organized by project.

```bash
# Generate archive in temp directory and open in browser
claude-session-export all --open

# Generate archive in specific directory
claude-session-export all -o ./my-archive

# Quiet mode (no output except errors)
claude-session-export all -o ./archive --quiet
```

### `search`

Search across all your Claude Code sessions for a specific term. Results show matching snippets and allow you to select a session to export.

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

# Search and immediately open the exported result
claude-session-export search "database migration" --open

# Limit the number of snippet previews per session
claude-session-export search "refactor" --max-matches 5
```

## Command Line Options

| Option | Short | Description |
|--------|-------|-------------|
| `--output DIR` | `-o` | Output directory (defaults to temp directory) |
| `--gist` | | Upload output to GitHub Gist (private by default) |
| `--public` | | Make gist public (use with --gist) |
| `--open` | | Open result in browser when complete |
| `--include-json` | | Include original JSON/JSONL file in output |
| `--quiet` | | Suppress all non-error output |
| `--limit N` | | Maximum sessions to show in picker (default: 20) |
| `--max-matches N` | | Maximum snippets to show per session in search (default: 3) |
| `--help` | `-h` | Show help message |
| `--version` | `-v` | Show version number |

## Output Structure

### Single Session

When converting a single session, the output directory contains:

```
output/
├── index.html           # Main index with search and timeline
├── page-001.html        # First page of conversations
├── page-002.html        # Second page
├── page-003.html        # And so on...
└── session.jsonl        # Original file (if --include-json)
```

### Batch Archive

When using the `all` command, sessions are organized by project:

```
archive/
├── index.html                              # Master index of all projects
├── my-project/
│   ├── index.html                          # Project index with session list
│   ├── abc123-session-1/
│   │   ├── index.html
│   │   ├── page-001.html
│   │   └── session.jsonl
│   └── def456-session-2/
│       ├── index.html
│       └── page-001.html
└── another-project/
    └── ...
```

## HTML Output Features

### Index Page

The index page provides:

- **Statistics** - Total prompts, messages, tool calls, commits, and pages
- **Search** - Full-text search across all pages with result highlighting
- **Timeline** - Chronological list of user prompts with:
  - First 200 characters of each prompt
  - Tool usage statistics (bash, read, write, edit counts)
  - Links to specific pages and messages
- **Commit Cards** - Git commits displayed inline with optional GitHub links

### Transcript Pages

Each page displays up to 5 conversations with:

- **User messages** - Blue-highlighted with timestamp
- **Assistant responses** - Gray background with full content
- **Thinking blocks** - Collapsible yellow-highlighted reasoning
- **Tool visualizations**:
  - **Bash** - Command with description, syntax-highlighted output
  - **Write** - File path with truncatable content preview
  - **Edit** - Side-by-side diff view (old vs new)
  - **Read/Glob/Grep** - File paths and patterns
  - **TodoWrite** - Styled task lists with status indicators
  - **Images** - Inline base64 image display
- **Commit cards** - Styled commit messages with GitHub links
- **Pagination** - Navigate between pages

### Search Functionality

The built-in search:

- Searches across all transcript pages
- Highlights matching text in results
- Shows which page each result is from
- Supports URL hash for shareable search links (`#search=query`)

## Tool Rendering Examples

### Bash Commands

```
┌─────────────────────────────────────┐
│ Bash  List files in directory       │
├─────────────────────────────────────┤
│ $ ls -la                            │
└─────────────────────────────────────┘
```

### File Edits

```
┌─────────────────────────────────────┐
│ Edit: /path/to/file.go              │
├─────────────────┬───────────────────┤
│ Old             │ New               │
├─────────────────┼───────────────────┤
│ func foo() {    │ func foo() error {│
│   return        │   return nil      │
│ }               │ }                 │
└─────────────────┴───────────────────┘
```

### Todo Lists

```
┌─────────────────────────────────────┐
│ TodoWrite                           │
├─────────────────────────────────────┤
│ ✓ Set up project structure          │
│ ◐ Implement core features           │
│ ○ Write tests                       │
└─────────────────────────────────────┘
```

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

## Session File Formats

### JSON Format

Standard JSON with a `messages` array:

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Hello, Claude!",
      "timestamp": "2024-01-15T10:30:00Z"
    },
    {
      "role": "assistant",
      "content": [
        {"type": "text", "text": "Hello! How can I help?"}
      ],
      "timestamp": "2024-01-15T10:30:05Z"
    }
  ]
}
```

### JSONL Format

One JSON object per line (used by Claude Code):

```jsonl
{"role": "user", "content": "Hello!", "timestamp": "2024-01-15T10:30:00Z"}
{"role": "assistant", "content": [{"type": "text", "text": "Hi there!"}], "timestamp": "2024-01-15T10:30:05Z"}
```

## Development

### Running Tests

```bash
go test ./...
```

### Running Tests with Verbose Output

```bash
go test ./... -v
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

This triggers a GitHub Action that builds binaries for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

Binaries are automatically attached to the GitHub Release.

### Project Structure

```
claude-session-export/
├── main.go                     # Entry point
├── go.mod                      # Module definition
├── internal/
│   ├── cli/                    # Command-line interface
│   │   ├── cli.go              # Command handling
│   │   └── cli_test.go
│   ├── session/                # Session parsing
│   │   ├── types.go            # Data structures
│   │   ├── parse.go            # JSON/JSONL parsing
│   │   ├── parse_test.go
│   │   └── discover.go         # Local session discovery
│   ├── render/                 # HTML generation
│   │   ├── render.go           # Message rendering
│   │   ├── render_test.go
│   │   ├── html.go             # Page generation
│   │   ├── html_test.go
│   │   ├── markdown.go         # Markdown to HTML
│   │   ├── markdown_test.go
│   │   └── batch.go            # Archive generation
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

### Search not working

Search requires JavaScript. If viewing files locally (`file://`), some browsers restrict JavaScript. Use `--open` to serve via a proper HTTP server, or upload to Gist.

## License

Apache 2.0

## Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.
