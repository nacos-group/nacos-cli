# Nacos CLI

A powerful command-line tool for managing Nacos configuration center and AI skills, written in Go.

## Features

- 🚀 Fast and lightweight - single binary with no dependencies
- 💻 Interactive terminal mode with auto-completion
- 🎯 Skill management - full lifecycle: upload → review → release, plus get/list/describe/sync
- 🤖 AgentSpec management - full lifecycle: upload → review → release, plus get/list/describe
- 📝 Configuration management - list, get and set configurations
- 🔄 Real-time skill synchronization with Nacos
- 🌐 Namespace support for multi-environment management
- 📦 Batch operations - upload all skills and agent specs at once
- 🧾 Structured output - `--output json` on list/describe for scripting

## Installation

### npm / npx

Use `npx` to run directly without installation:

```bash
npx @nacos-group/cli --help
npx @nacos-group/cli skill-list --host 127.0.0.1 --port 8848 -u nacos -p nacos
```

Or install globally via npm:

```bash
npm install -g @nacos-group/cli
nacos-cli --help
```

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/nacos-group/nacos-cli/releases).

### Build from Source

```bash
# Clone the repository
git clone https://github.com/nacos-group/nacos-cli.git
cd nacos-cli

# Build
go build -o nacos-cli

# Or use make
make build
```

## Quick Start

### CLI Mode

Run commands directly:

```bash
# List all skills
nacos-cli skill-list -s 127.0.0.1:8848 -u nacos -p nacos

# Get a skill
nacos-cli skill-get skill-creator -s 127.0.0.1:8848 -u nacos -p nacos

# Upload a skill
nacos-cli skill-upload /path/to/skill -s 127.0.0.1:8848 -u nacos -p nacos
```

### Interactive Terminal Mode

Start an interactive session:

```bash
nacos-cli -s 127.0.0.1:8848 -u nacos -p nacos
```

Once in terminal mode, you can run commands interactively:

```
nacos> skill-list
nacos> skill-get skill-creator
nacos> config-list
nacos> help
```

## Commands

### AgentSpec Management

Agent specs follow a three-stage lifecycle aligned with the server:
`upload` (editing) → `review` (reviewing → reviewed) → `release` (online).

#### List AgentSpecs

```bash
# CLI mode (pretty output by default)
nacos-cli agentspec-list -s 127.0.0.1:8848 -u nacos -p nacos

# With filters
nacos-cli agentspec-list --name my-agentspec --page 1 --size 20

# Machine-readable output for scripts
nacos-cli agentspec-list --output json

# Terminal mode
nacos> agentspec-list
nacos> agentspec-list --name my-agentspec --page 2
nacos> agentspec-list --output json
```

#### Describe AgentSpec

Show detail + version history (latest / editing / reviewing / online, plus per-version status):

```bash
nacos-cli agentspec-describe my-agentspec
nacos-cli agentspec-describe my-agentspec --output json

# Terminal mode
nacos> agentspec-describe my-agentspec
```

#### Get/Download AgentSpec

Download an agent spec to local directory (default: `~/.agentspecs`):

```bash
# CLI mode
nacos-cli agentspec-get my-agentspec -s 127.0.0.1:8848 -u nacos -p nacos
nacos-cli agentspec-get my-agentspec -o /custom/path

# Download specific version
nacos-cli agentspec-get my-agentspec --version v1

# Download by route label
nacos-cli agentspec-get my-agentspec --label latest

# Download multiple agent specs
nacos-cli agentspec-get spec1 spec2 spec3

# Terminal mode
nacos> agentspec-get my-agentspec
```

#### Upload AgentSpec

Upload an agent spec from local directory (creates or updates the `editing` version):

```bash
# Upload single agent spec
nacos-cli agentspec-upload /path/to/agentspec -s 127.0.0.1:8848 -u nacos -p nacos

# Upload all agent specs in a directory
nacos-cli agentspec-upload --all /path/to/agentspecs/folder

# Terminal mode
nacos> agentspec-upload /path/to/agentspec
nacos> agentspec-upload --all /path/to/agentspecs
```

#### Review AgentSpec

Submit the current editing version for review (editing → reviewing). The server-side
review pipeline is asynchronous and eventually marks the version as `reviewed`.

```bash
nacos-cli agentspec-review my-agentspec

# Terminal mode
nacos> agentspec-review my-agentspec
```

#### Release AgentSpec

Publish an approved (reviewed) version online:

```bash
nacos-cli agentspec-release my-agentspec --version 0.0.2
nacos-cli agentspec-release my-agentspec --version 0.0.2 --update-latest=false

# Terminal mode
nacos> agentspec-release my-agentspec --version 0.0.2
```

> **Note**: if `agentspec-release` fails with `HTTP 400 parameter validate error`
> right after `agentspec-review`, the async review pipeline probably hasn't
> marked the version as `reviewed` yet. The CLI will print a hint telling you
> to wait a few seconds and re-check status via `agentspec-describe`. Retry
> when `STATUS=reviewed`.

#### Publish AgentSpec (deprecated)

`agentspec-publish` is kept as a backward-compatible shortcut that runs
`upload` + `review` in sequence. It prints a deprecation warning and will
be removed in a future release — prefer the explicit lifecycle commands
above.

```bash
# Legacy shortcut (deprecated)
nacos-cli agentspec-publish /path/to/agentspec
nacos-cli agentspec-publish --all /path/to/agentspecs/folder
```

### Skill Management

Skills follow the same three-stage lifecycle as agent specs:
`upload` (editing) → `review` (reviewing → reviewed) → `release` (online).

#### List Skills

```bash
# CLI mode (pretty output by default)
nacos-cli skill-list -s 127.0.0.1:8848 -u nacos -p nacos

# With filters
nacos-cli skill-list --name skill-creator --page 1 --size 20

# Machine-readable output for scripts
nacos-cli skill-list --output json

# Terminal mode
nacos> skill-list
nacos> skill-list --name skill-creator --page 2
nacos> skill-list --output json
```

#### Describe Skill

```bash
nacos-cli skill-describe skill-creator
nacos-cli skill-describe skill-creator --output json

# Terminal mode
nacos> skill-describe skill-creator
```

#### Get/Download Skill

Download a skill to local directory (default: `~/.skills`):

```bash
# CLI mode
nacos-cli skill-get skill-creator -s 127.0.0.1:8848 -u nacos -p nacos
nacos-cli skill-get skill-creator -o /custom/path

# Terminal mode
nacos> skill-get skill-creator
```

#### Upload Skill

Upload a skill from local directory (creates or updates the `editing` version):

```bash
# Upload single skill
nacos-cli skill-upload /path/to/skill -s 127.0.0.1:8848 -u nacos -p nacos

# Upload all skills in a directory
nacos-cli skill-upload --all /path/to/skills/folder

# Terminal mode
nacos> skill-upload /path/to/skill
nacos> skill-upload --all /path/to/skills
```

#### Review Skill

Submit the current editing version for review (editing → reviewing):

```bash
nacos-cli skill-review skill-creator

# Terminal mode
nacos> skill-review skill-creator
```

#### Release Skill

Publish an approved (reviewed) version online:

```bash
nacos-cli skill-release skill-creator --version 0.0.2
nacos-cli skill-release skill-creator --version 0.0.2 --update-latest=false

# Terminal mode
nacos> skill-release skill-creator --version 0.0.2
```

> Same async-pipeline note as `agentspec-release`: if `skill-release` returns
> `HTTP 400 parameter validate error` just after `skill-review`, wait and retry
> when `skill-describe` shows the version as `reviewed`.

#### Publish Skill (deprecated)

`skill-publish` is kept as a backward-compatible shortcut that runs
`upload` + `review` in sequence. Prefer the explicit lifecycle commands.

```bash
# Legacy shortcut (deprecated)
nacos-cli skill-publish /path/to/skill
nacos-cli skill-publish --all /path/to/skills/folder
```

#### Sync Skill

Real-time synchronization - automatically syncs local skills when they change in Nacos:

```bash
# Sync single skill (CLI mode only)
nacos-cli skill-sync skill-creator -s 127.0.0.1:8848 -u nacos -p nacos

# Sync multiple skills
nacos-cli skill-sync skill-creator skill-analyzer

# Sync all skills
nacos-cli skill-sync --all

# Press Ctrl+C to stop synchronization
```

**Note**: `skill-sync` is only available in CLI mode, not in terminal mode.

### Configuration Management

#### List Configurations

```bash
# CLI mode
nacos-cli config-list -s 127.0.0.1:8848 -u nacos -p nacos

# With filters
nacos-cli config-list --data-id myconfig --group DEFAULT_GROUP

# With pagination
nacos-cli config-list --page 1 --size 20

# Terminal mode
nacos> config-list
nacos> config-list --data-id myconfig --page 2
```

#### Get Configuration

```bash
# CLI mode
nacos-cli config-get myconfig DEFAULT_GROUP -s 127.0.0.1:8848 -u nacos -p nacos

# Terminal mode
nacos> config-get myconfig DEFAULT_GROUP
```

### Terminal Commands

When in interactive terminal mode:

```bash
nacos> help           # Show all available commands
nacos> server         # Show server information
nacos> ns             # Show current namespace
nacos> ns production  # Switch to production namespace
nacos> clear          # Clear screen
nacos> quit           # Exit terminal
```

## Global Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| --host | | market.hiclaw.io when `--host` and `--port` are both omitted; otherwise 127.0.0.1 when only `--port` is provided | Nacos server host |
| --port | | 80 when `--host` and `--port` are both omitted; otherwise 8848 when omitted after `--host` | Nacos server port |
| --server | -s | market.hiclaw.io:80 when no host/port is provided | Nacos server address (deprecated, use --host and --port) |
| --username | -u | nacos | Nacos username |
| --password | -p | nacos | Nacos password |
| --namespace | -n | (empty/public) | Nacos namespace ID |
| --config | -c | | Path to configuration file |
| --help | -h | | Show help information |

## Configuration File

You can use a configuration file to avoid typing credentials every time:

```bash
# Create a config file
cat > local.conf << EOF
host: 127.0.0.1
port: 8848
username: nacos
password: nacos
namespace: ""
EOF

# Use the config file
nacos-cli --config ./local.conf skill-list
```

### Configuration File Format

The configuration file uses YAML format:

```yaml
# Nacos server host
host: 127.0.0.1

# Nacos server port
port: 8848

# Username for authentication
username: nacos

# Password for authentication
password: nacos

# Namespace ID (optional, leave empty for public namespace)
namespace: ""
```

### Configuration Priority

Configuration values are applied in the following priority order:
1. **Command line arguments** (highest priority)
2. **Configuration file**
3. **Environment variables**
4. **Default values** (lowest priority)

Supported environment variables:

```bash
export NACOS_HOST=127.0.0.1
export NACOS_PORT=8848
export NACOS_NAMESPACE=xxx
```

For example:
- `nacos-cli --config ./local.conf --host 10.0.0.1` - Uses `10.0.0.1` from command line, other values from config file
- `NACOS_HOST=127.0.0.1 NACOS_PORT=8848 NACOS_NAMESPACE=xxx nacos-cli skill-list` - Uses environment variables when command line and config file values are not provided
- `nacos-cli` - Uses default `market.hiclaw.io:80` when neither `--host` nor `--port` is provided
- `nacos-cli --host 127.0.0.1` - Uses `127.0.0.1:8848` because `--host` was provided without `--port`
- `nacos-cli --port 8849` - Uses `127.0.0.1:8849` because only `--port` was provided
- `nacos-cli --config ./local.conf` - Uses all values from config file

## Project Structure

```
nacos-cli/
├── cmd/                       # CLI commands
│   ├── root.go                # Root command / global flags
│   ├── list_skill.go          # skill-list
│   ├── describe_skill.go      # skill-describe
│   ├── get_skill.go           # skill-get
│   ├── upload_skill.go        # skill-upload
│   ├── review_skill.go        # skill-review
│   ├── release_skill.go       # skill-release
│   ├── publish_skill.go       # skill-publish (deprecated wrapper)
│   ├── sync_skill.go          # skill-sync
│   ├── list_agentspec.go      # agentspec-list
│   ├── describe_agentspec.go  # agentspec-describe
│   ├── get_agentspec.go       # agentspec-get
│   ├── upload_agentspec.go    # agentspec-upload
│   ├── review_agentspec.go    # agentspec-review
│   ├── release_agentspec.go   # agentspec-release
│   ├── publish_agentspec.go   # agentspec-publish (deprecated wrapper)
│   ├── list_config.go         # config-list
│   ├── get_config.go          # config-get
│   ├── set_config.go          # config-set
│   ├── profile.go             # profile / config file handling
│   └── interactive.go         # Interactive terminal entry
├── internal/
│   ├── client/                # Nacos client
│   ├── skill/                 # Skill service
│   ├── agentspec/             # AgentSpec service
│   ├── sync/                  # Sync service
│   ├── listener/              # Config listener
│   ├── terminal/              # Interactive terminal implementation
│   └── help/                  # Help system
├── main.go
├── go.mod
└── README.md
```

## Development

### Prerequisites

- Go 1.21 or higher
- Nacos server (2.x recommended)

### Build

```bash
# Build binary
make build

# Or manually
go build -o nacos-cli
```

### Run Tests

```bash
# Run test script
./test.sh

# Or test specific commands
go run main.go skill-list -s 127.0.0.1:8848 -u nacos -p nacos
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License

## Changelog

### v1.0.4 (2026-05-08)

- Aligned `agentspec-*` commands with `skill-*` around the full server lifecycle
  (`upload` → `review` → `release`), plus new `agentspec-describe` / `skill-describe`
- Added `--output pretty|json` on `*-list` and `*-describe` for scripting
- `agentspec-publish` / `skill-publish` are now deprecated wrappers that run
  `upload` + `review` and emit a deprecation warning
- `*-release` now prints a targeted hint when failing with `HTTP 400 parameter
  validate error`, pointing to the async review pipeline timing issue

### v0.2.0 (2026-01-28)

- Rewritten in Go for better performance and portability
- Added skill management commands (list, get, upload, sync)
- Added agent spec management commands (list, get, upload)
- Added real-time skill synchronization with Nacos
- Added interactive terminal mode with auto-completion
- Added batch upload support for multiple skills and agent specs
- Added configuration management commands
- Improved error handling and user experience
- Removed all emoji clutter from terminal output

### v0.1.0 (2026-01-27)

- Initial Python version release
- Basic configuration management
- Basic service discovery
