# Image Processing Service

A lightweight, concurrent image processing API built with Go. This service accepts image processing jobs (like resizing or creating thumbnails) and processes them asynchronously using a worker pool pattern.

## What This Does

You upload an image, tell it what you want done (resize to 800x600, make a thumbnail, etc.), and get back a job ID. Check the status endpoint to see when it's done, then download your processed image. Simple.

The worker pool handles multiple concurrent jobs efficiently, so if you send 50 resize requests, they'll be processed in parallel instead of one-by-one.

## Quick Start

```bash
# Install dependencies
go mod tidy

# Run the server
go run main.go

# Server starts on port 8080 by default
```

That's it. No Docker, no complex setup. It creates the necessary directories (uploads, outputs, data) automatically.

## Configuration

Set these environment variables if you want to customize things:

```bash
PORT=8080                    # HTTP port
DB_PATH=./data/jobs.db       # SQLite database location
UPLOAD_DIR=./uploads         # Where uploaded images go
OUTPUT_DIR=./outputs         # Where processed images go
WORKER_POOL_SIZE=10          # Number of concurrent workers
MAX_UPLOAD_SIZE=10485760     # Max file size in bytes (10MB default)
```

## API Usage

### Submit a Job

```bash
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "image_data": "<base64-encoded-image-data>",
    "operation": "resize",
    "width": 800,
    "height": 600,
    "output_format": "jpeg"
  }'
```

Response:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending",
  "message": "Image processing job submitted"
}
```

### Check Job Status

```bash
curl http://localhost:8080/jobs/550e8400-e29b-41d4-a716-446655440000
```

Response:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "input_path": "./uploads/...",
  "output_path": "./outputs/...",
  "operation": "resize",
  "width": 800,
  "height": 600,
  "created_at": "2024-...",
  "updated_at": "2024-..."
}
```

### Download Processed Image

```bash
curl -O http://localhost:8080/jobs/550e8400-e29b-41d4-a716-446655440000/download
```

### Health Check

```bash
curl http://localhost:8080/health
```

## Supported Operations

- `resize` - Resize to exact dimensions (may distort aspect ratio)
- `thumbnail` - Resize to fit within dimensions while maintaining aspect ratio

Output formats: `jpeg`, `png` (optional, defaults to input format)

## Why Worker Pools?

Image processing is CPU-intensive. Without a worker pool:
- Each request would spawn a goroutine
- 1000 concurrent uploads = 1000 goroutines fighting for CPU
- System thrashes, performance tanks

With the worker pool:
- Fixed number of workers (configurable, default 10)
- Jobs queue up if workers are busy
- Controlled concurrency, predictable resource usage
- Better throughput under load

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│  HTTP API   │────▶│  Job Queue  │
└─────────────┘     └─────────────┘     └──────┬──────┘
                                               │
                       ┌───────────────────────┘
                       ▼
              ┌─────────────────┐
              │   Worker Pool   │
              │  ┌───┐ ┌───┐    │
              │  │ W │ │ W │... │
              │  └───┘ └───┘    │
              └───────┬─────────┘
                      ▼
              ┌───────────────┐
              │  Image Proc.  │
              └───────┬───────┘
                      ▼
              ┌───────────────┐
              │  SQLite / FS  │
              └───────────────┘
```

## Testing

```bash
go test ./workerpool -v
```

Includes tests for:
- Multiple concurrent jobs
- Worker parallelism
- Graceful shutdown
- Pool resizing

## Production Considerations

What's already handled:
- Graceful shutdown (waits for in-progress jobs)
- Request validation
- Structured logging
- Configurable via environment variables
- SQLite for persistence

What you'd want to add for real production:
- S3/MinIO instead of local filesystem for storage
- Redis for job queue (instead of in-memory)
- Authentication/authorization
- Rate limiting
- Metrics (Prometheus)
- Circuit breakers for external services

## Tech Stack

- **Go** - Language of choice for concurrency
- **Gin** - HTTP framework (fast, minimal)
- **GORM** - ORM for database operations
- **SQLite** - Zero-config database
- **Worker Pool** - Custom implementation (see `workerpool/`)

## Why This Project?

I wanted to demonstrate:
1. **Concurrency patterns** - Worker pools, channels, goroutines
2. **Clean architecture** - Separation of concerns, dependency injection
3. **Production awareness** - Graceful shutdown, logging, config management
4. **Async job processing** - Real-world pattern for CPU-intensive tasks

The code is intentionally straightforward. No unnecessary abstractions, no over-engineering. Just solid, working Go.
