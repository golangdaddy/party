# Minecraft Bedrock Server Manager

A Go application that polls a public GitHub repository for configuration and manages multiple Minecraft Bedrock Edition server instances automatically.

## Features

- **Public GitHub Integration**: Polls a public GitHub repository for server configurations (no authentication required)
- **Flexible Branch Configuration**: Use a `branch` file to specify which branch to monitor for configuration
- **Automatic Server Management**: Starts, stops, and updates Bedrock servers based on configuration changes
- **Multiple Server Support**: Manages up to 5 Minecraft Bedrock server instances simultaneously
- **HTTP API**: Provides health checks and server status endpoints
- **Graceful Shutdown**: Properly stops all servers when the application is terminated
- **Bedrock Edition Support**: Works with official Minecraft Bedrock Dedicated Server

## Directory Structure

```
minecraft-server-manager/
├── cmd/
│   └── client/
│       └── main.go              # Main application entry point
├── internal/
│   ├── config/
│   │   └── config.go            # Configuration management
│   ├── github/
│   │   └── client.go            # GitHub API client (public repos only)
│   └── server/
│       └── manager.go           # Minecraft Bedrock server management
├── config.yaml                  # Application configuration
├── branch                       # Branch specification (optional)
├── example-servers.yaml         # Example server configuration
├── go.mod                       # Go module dependencies
└── README.md                    # This file
```

## Prerequisites

- Go 1.21 or later
- Minecraft Bedrock Dedicated Server executable
- A public GitHub repository for server configurations

## Installation

1. Clone the repository:
```bash
git clone <your-repo-url>
cd minecraft-server-manager
```

2. Install dependencies:
```bash
go mod tidy
```

3. Download Minecraft Bedrock Dedicated Server:
   - Visit [Minecraft Bedrock Dedicated Server](https://www.minecraft.net/en-us/download/server/bedrock)
   - Download the appropriate version for your platform
   - Extract and place the `bedrock_server` executable in your project directory

4. Configure the application by editing `config.yaml`:
```yaml
github:
  repo_owner: "your-username"
  repo_name: "minecraft-servers-config"
  branch: "main"  # Default branch (can be overridden by branch file)
  config_path: "servers.yaml"
  poll_interval: 60

http:
  port: 8080

server:
  base_dir: "./servers"
  max_instances: 5
  bedrock_path: "./bedrock_server"  # Path to Bedrock server executable
  memory_limit: "1G"
```

5. (Optional) Create a `branch` file to specify which branch to monitor:
```bash
echo "production" > branch
```

## Branch Configuration

The application supports flexible branch configuration through a `branch` file in the root directory:

### Using the Branch File

Create a `branch` file in the root directory with the branch name:
```bash
echo "production" > branch
```

This will make the application monitor the `production` branch instead of the default `main` branch.

### Branch Priority

The branch is determined in this order:
1. **Branch file** (highest priority) - if `branch` file exists, its contents are used
2. **config.yaml** - if no branch file exists, uses `branch` field from config
3. **Default** - if neither exists, defaults to `main`

### Examples

**Using a development branch:**
```bash
echo "dev" > branch
```

**Using a staging branch:**
```bash
echo "staging" > branch
```

**Using main branch (remove branch file):**
```bash
rm branch
```

## GitHub Repository Setup

Create a **public** GitHub repository with a `servers.yaml` file containing your server configurations. Use `example-servers.yaml` as a template.

**Important**: The repository must be public since this application doesn't use authentication.

Example `servers.yaml`:
```yaml
servers:
  - name: "survival-world"
    port: 19132
    version: "1.20.50"
    world_name: "survival"
    level_type: "DEFAULT"
    gamemode: "survival"
    difficulty: "normal"
    max_players: 20
    online_mode: true
    pvp: true
    allow_flight: false
    motd: "Welcome to Survival World!"
    whitelist:
      - "player1"
      - "player2"
    ops:
      - "admin1"
    default_player_permission_level: "member"
```

## Running the Application

1. Build the application:
```bash
go build -o minecraft-manager cmd/client/main.go
```

2. Run the application:
```bash
./minecraft-manager
```

Or run directly with Go:
```bash
go run cmd/client/main.go
```

The application will log which branch it's using:
```
time="2024-01-01T12:00:00Z" level=info msg="Using branch 'production' for configuration"
```

## Configuration Options

### GitHub Configuration
- `repo_owner`: GitHub username or organization
- `repo_name`: Repository name (must be public)
- `branch`: Default branch to monitor (can be overridden by `branch` file)
- `config_path`: Path to the configuration file in the repo (default: "servers.yaml")
- `poll_interval`: How often to check for changes in seconds (default: 60)

### Server Configuration
- `base_dir`: Directory where server files will be stored
- `max_instances`: Maximum number of servers to run simultaneously
- `bedrock_path`: Path to Bedrock server executable
- `memory_limit`: Memory limit for servers

### Minecraft Bedrock Server Properties
Each server in the configuration supports the following properties:
- `name`: Unique server name
- `port`: Server port (must be unique, default Bedrock port is 19132)
- `version`: Minecraft Bedrock version
- `world_name`: World directory name
- `level_seed`: World seed (optional)
- `level_type`: World type (DEFAULT, FLAT, LEGACY)
- `gamemode`: Game mode (survival, creative, adventure)
- `difficulty`: Difficulty level (peaceful, easy, normal, hard)
- `max_players`: Maximum number of players
- `online_mode`: Enable online mode (authentication)
- `pvp`: Enable PvP
- `allow_flight`: Allow players to fly
- `motd`: Message of the day
- `whitelist`: List of whitelisted players
- `ops`: List of server operators
- `default_player_permission_level`: Default permission level (visitor, member, operator)
- `content_log_file_enabled`: Enable content logging
- `enable_scripts`: Enable scripting
- `enable_command_blocking`: Enable command blocking
- `max_threads`: Maximum number of threads
- `player_idle_timeout`: Player idle timeout in minutes
- `max_world_size`: Maximum world size in chunks
- `properties`: Additional server.properties settings

## API Endpoints

The application provides HTTP endpoints for monitoring:

- `GET /health`: Health check endpoint
- `GET /status`: Server status information

Example status response:
```json
{
  "total_servers": 3,
  "running": 2,
  "stopped": 1,
  "servers": [
    {
      "name": "survival-world",
      "status": "running",
      "port": 19132,
      "start_time": "2024-01-01T12:00:00Z",
      "uptime": "2h30m15s",
      "player_count": 0
    }
  ],
  "last_update": "2024-01-01T14:30:00Z"
}
```

## Server Lifecycle

1. **Configuration Polling**: The application polls the public GitHub repository every `poll_interval` seconds
2. **Change Detection**: When changes are detected, the application updates server configurations
3. **Server Management**:
   - Starts new servers defined in the configuration
   - Stops servers no longer in the configuration
   - Restarts servers when their configuration changes
4. **Process Monitoring**: Monitors server processes and logs crashes

## Bedrock Server Files

For each server, the application creates:
- `server.properties`: Server configuration file
- `permissions.json`: Player permissions and operator list
- `whitelist.json`: Whitelisted players
- `worlds/`: Directory containing world data
- `logs/`: Server log files

## Security Considerations

- **Public Repository Only**: This application only works with public GitHub repositories
- Configure firewalls to only allow necessary ports (19132-19136 for Bedrock)
- Use whitelists and operator lists to control access
- Consider using a dedicated user account for running the application
- Bedrock servers require proper authentication for online mode

## Rate Limiting

Since this application uses the GitHub API without authentication:
- **Rate Limit**: 60 requests per hour for unauthenticated requests
- **Polling Interval**: Default is 60 seconds, which allows for 60 requests per hour
- **Recommendation**: Don't set `poll_interval` lower than 60 seconds to avoid rate limiting

## Troubleshooting

### Common Issues

1. **Bedrock server not found**: Ensure the Bedrock server executable is in the correct path
2. **Port conflicts**: Make sure each server has a unique port (19132-19136 recommended)
3. **Permission errors**: Ensure the application has write permissions to the server directory
4. **GitHub API rate limiting**: If you see rate limit errors, increase the `poll_interval`
5. **Bedrock server crashes**: Check server logs in the `logs/` directory
6. **Repository not found**: Ensure the GitHub repository is public and the path is correct
7. **Branch not found**: Ensure the branch specified in the `branch` file exists in the repository

### Bedrock-Specific Notes

- Bedrock servers use different ports than Java Edition (19132 vs 25565)
- Bedrock servers require the official dedicated server software
- World formats are different from Java Edition
- Some features like plugins work differently in Bedrock

### Logs

The application logs all activities to stdout. Check the logs for:
- Configuration changes
- Server start/stop events
- Error messages
- GitHub API responses
- Branch information

## Development

To contribute to the project:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details. 