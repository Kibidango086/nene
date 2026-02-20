# Nene

A lightweight AI agent that communicates via Telegram, implemented in Go.

## Features

- **Telegram Integration**: Interact via Telegram messaging
- **Streaming Responses**: Real-time streaming with live updates
- **Tool Execution Display**: Visual display of tool calls
- **Session Management**: Separate conversation contexts per chat
- **Multiple Providers**: OpenAI, Anthropic Claude, Azure OpenAI, and OpenAI-compatible APIs
- **Long-term Memory**: SQLite + FTS5 powered memory system
- **Parallel Subagents**: Spawn multiple subagents for parallel task execution
- **Proxy Support**: HTTP/HTTPS proxy for Telegram API
- **Access Control**: Allow-list based user permission

## Quick Start

```bash
# Build
go build -o nene ./cmd/nene

# Initialize configuration
./nene init

# Edit config file
vim ~/.nene/config.json

# Run
./nene
```

## Configuration

All data is stored in `~/.nene/`:
- `config.json` - Configuration file
- `memory.db` - Long-term memory database

### Initialize

Run `nene init` to create a default config at `~/.nene/config.json`:

```bash
./nene init
```

### Config File

```json
{
  "telegram": {
    "token": "your-telegram-bot-token",
    "proxy": "",
    "allow_from": [],
    "stream_mode": true
  },
  "provider": {
    "type": "openai",
    "api_key": "your-api-key",
    "base_url": "",
    "model": "gpt-4o"
  },
  "system_prompt": ""
}
```

### Environment Variables

Environment variables override config file:

```bash
export TELEGRAM_BOT_TOKEN="your-token"
export OPENAI_API_KEY="your-key"
export NENE_PROVIDER_MODEL="gpt-4o"
```

## Available Tools

| Tool | Description |
|------|-------------|
| `shell` | Execute shell commands |
| `read_file` | Read file contents |
| `write_file` | Write content to a file |
| `list_files` | List files in a directory |
| `websearch` | Search the web |
| `webfetch` | Fetch content from a URL |
| `message` | Send a message to the user |
| `think` | Internal reasoning |
| `spawn` | Spawn parallel subagents |
| `memory_store` | Store information in long-term memory |
| `memory_recall` | Search and retrieve memories |
| `memory_forget` | Delete a memory entry |

## Architecture

```
pkg/
├── agent/       # Session management
├── bus/         # Message bus (inbound/outbound/stream)
├── memory/      # Long-term memory (SQLite + FTS5)
├── model/       # LLM provider abstraction
├── telegram/    # Telegram bot integration
└── tool/        # Tool system
```

## License

MIT
