# Go Evaluation Mini App

This is a Go port of the Flask evaluation mini application. It provides the same functionality as the Python version but using Go with Gin web framework and GORM for database operations.

## Prerequisites

- Go 1.21 or higher
- SQLite (for database)

## Installation

1. Navigate to the application directory:
   ```bash
   cd source/evaluation/evaluation-mini
   ```

2. Initialize the Go module and download dependencies:
   ```bash
   go mod tidy
   ```

## Running the Application

1. Start the Go server:
   ```bash
   go run main.go
   ```

2. The server will start on port 8080. Access the application at:
   ```
   http://localhost:8080
   ```

## API Endpoints

The Go version provides the same API endpoints as the Flask version:

- `GET /` - Serves the main HTML page
- `GET /api/comments/by-model` - Get all comments grouped by model
- `POST /process` - Process reference and response JSONL files
- `POST /comments` - Add a new comment
- `GET /comments/:pmid` - Get comments by PMID
- `DELETE /comments/:id` - Delete a comment by ID

## Features

- **Database**: Uses SQLite with GORM for ORM functionality
- **Web Framework**: Gin for HTTP routing and middleware
- **File Processing**: Handles multipart form uploads for JSONL files
- **JSON API**: All endpoints return JSON responses
- **HTML Templates**: Serves HTML templates from the `templates/` directory
- **Static Files**: Serves static files from the `static/` directory

## Differences from Flask Version

1. **Port**: Runs on port 8080 instead of 5000 (Flask default)
2. **Database**: Creates `comments.db` in the same directory
3. **Performance**: Generally faster due to Go's compiled nature
4. **Concurrency**: Better handling of concurrent requests

## Building for Production

To build a standalone executable:

```bash
go build -o evaluation-mini main.go
```

This creates an executable that can be run without requiring Go to be installed on the target system.

## Dependencies

- **Gin**: Web framework for routing and HTTP handling
- **GORM**: ORM library for database operations
- **SQLite Driver**: Database driver for SQLite support

All dependencies are managed through `go.mod` and will be automatically downloaded when running `go mod tidy`. 