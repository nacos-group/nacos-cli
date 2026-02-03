# Nacos CLI

A powerful command-line tool for managing Nacos configuration center and AI skills, written in Go.

## Features

- 🚀 Fast and lightweight - single binary with no dependencies
- 💻 Interactive terminal mode with auto-completion
- 🎯 Skill management - upload, download, list, and sync AI skills
- 📝 Configuration management - list and get configurations
- 🔄 Real-time skill synchronization with Nacos
- 🌐 Namespace support for multi-environment management
- 📦 Batch operations - upload all skills at once

## Installation

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/yourusername/nacos-cli/releases).

### Build from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/nacos-cli.git
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

### Skill Management

#### List Skills

```bash
# CLI mode
nacos-cli skill-list -s 127.0.0.1:8848 -u nacos -p nacos

# With filters
nacos-cli skill-list --name skill-creator --page 1 --size 20

# Terminal mode
nacos> skill-list
nacos> skill-list --name skill-creator --page 2
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

Upload a skill from local directory:

```bash
# Upload single skill
nacos-cli skill-upload /path/to/skill -s 127.0.0.1:8848 -u nacos -p nacos

# Upload all skills in a directory
nacos-cli skill-upload --all /path/to/skills/folder

# Terminal mode
nacos> skill-upload /path/to/skill
nacos> skill-upload --all /path/to/skills
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
| --server | -s | 127.0.0.1:8848 | Nacos server address |
| --username | -u | nacos | Nacos username |
| --password | -p | nacos | Nacos password |
| --namespace | -n | (empty/public) | Nacos namespace ID |
| --help | -h | | Show help information |

## Project Structure

```
nacos-cli/
├── cmd/                  # CLI commands
│   ├── root.go          # Root command
│   ├── list_skill.go    # skill-list command  
│   ├── get_skill.go     # skill-get command
│   ├── upload_skill.go  # skill-upload command
│   ├── sync_skill.go    # skill-sync command
│   ├── list_config.go   # config-list command
│   ├── get_config.go    # config-get command
│   └── interactive.go   # Interactive terminal
├── internal/
│   ├── client/          # Nacos client
│   ├── skill/           # Skill service
│   ├── sync/            # Sync service
│   ├── listener/        # Config listener
│   ├── terminal/        # Terminal implementation
│   └── help/            # Help system
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

### v0.2.0 (2026-01-28)

- Rewritten in Go for better performance and portability
- Added skill management commands (list, get, upload, sync)
- Added real-time skill synchronization with Nacos
- Added interactive terminal mode with auto-completion
- Added batch upload support for multiple skills
- Added configuration management commands
- Improved error handling and user experience
- Removed all emoji clutter from terminal output

### v0.1.0 (2026-01-27)

- Initial Python version release
- Basic configuration management
- Basic service discovery