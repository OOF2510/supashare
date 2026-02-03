# Supashare

A simple file sharing web app made with Go and HTMX.

## Features

- **File Uploads**: Upload files to Supabase Storage or any S3-compatible storage
- **Chunked Uploads**: Large files (>5MB) are uploaded in chunks for better reliability
- **ZIP Creation**: Create ZIP archives from multiple files
- **Media Compression**: Compress images and videos with configurable quality settings
- **Shareable Links**: Generate shareable links for uploaded files
- **My Shares Dashboard**: Track and manage your uploaded files

## Quick Start

### Prerequisites

- Go 1.25.5+
- PostgreSQL database
- S3-compatible storage
- FFmpeg (for video compression)

### Installation

1. Clone the repository:
```bash
git clone https://github.com/yourusername/supashare
cd supashare
```

2. Set up environment variables:
```bash
cp .env.example .env
# Edit .env with your credentials
```

3. Build the project:
```bash
./build.bash <version>
```
output binary will be in `./dist/<version>/supashare-<version>.x86_64`, pages/ copied into `./dist/<version>/pages/`, and .env symlinked to `./dist/<version>/.env`

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | 8080 |
| `BASE_URL` | Base URL for share links | http://localhost |
| `DATABASE_URL` | PostgreSQL connection string | - |
| `S3_ACCESS_KEY` | S3 access key | - |
| `S3_SECRET_KEY` | S3 secret key | - |
| `S3_STORAGE_ENDPOINT` | S3 endpoint URL | - |
| `S3_BUCKET_NAME` | S3 bucket name | fileshare |
| `LOG_LEVEL` | Logging level (debug, info, warn, error) | info |

## API Endpoints

- `GET /` - Main web interface
- `POST /upload` - Upload files
- `POST /upload/chunk` - Chunked upload
- `POST /create-zip` - Create ZIP archives
- `POST /compress-media` - Compress media files
- `GET /my-shares` - List user's uploads
- `GET /share/:id` - Download shared file
- `GET /health` - Health check with system stats

## Docker

Build and run with Docker:

```bash
docker build -t supashare .
docker run -p 8080:8080 --env-file .env supashare
```

## Tech Stack

- **Backend**: Go with Fiber
- **Frontend**: HTMX with Bulma CSS
- **Database**: PostgreSQL with GORM
- **Storage**: S3-compatible (Supabase Storage, AWS S3, etc) with AWS SDK for Go
- **Media Processing**: FFmpeg and imaging
- **Logging**: Logrus

## License

MPL-2.0
