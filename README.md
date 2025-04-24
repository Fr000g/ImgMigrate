# Image Migration Tool

⚠️This is a a toy project, not for industrial usage, skopeo is a more suitable way

## Background

A simple tool to migrate multi-arch images to local storage or private registry after tagging.

## Features

- Pull multi-architecture Docker images from DockerHub
- Tag images with architecture-specific tags
- Save images to local filesystem as .tar or .tar.gz files
- Push images to private registries with custom tags
- Support for specific architecture selection or all available architectures
- Filter images by operating system (e.g., linux, windows)
- Create multi-architecture manifest files
- YAML-based configuration for batch processing
- Gzip compression support for saved images

## Requirements

- Docker CLI installed and properly configured
- Docker experimental features enabled (for manifest inspection)
- Docker daemon must be running
- Go 1.16 or higher (for building from source)

Before using this tool, make sure Docker is installed and running with the command:
```bash
docker --version
```

Ensure Docker's experimental features are enabled by adding the following to your Docker daemon configuration file (usually at `/etc/docker/daemon.json`):
```json
{
  "experimental": true
}
```

## Installation

```bash
# Clone the repository
git clone https://github.com/Fr000g/ImageMigrate.git
cd ImageMigrate

# Build the tool
go build -o imgMigrate
```

## Usage

### Save all architectures of an image to local filesystem

```bash
# Pull and save all architectures of the nginx image to local
./imgMigrate pull --source nginx:latest --all-arch --output ./output

# With compression
./imgMigrate pull --source nginx:latest --all-arch --output ./output --compress

# Specify operating system(s)
./imgMigrate pull --source nginx:latest --all-arch --output ./output --os linux,windows
```

### Save specific architectures of an image to local filesystem

```bash
# Pull and save only amd64 and arm64 architectures of the nginx image
./imgMigrate pull --source nginx:latest --arch amd64,arm64 --output ./output
```

### Pull and push to private registry

```bash
# Pull and push all architectures to private registry
./imgMigrate push --source nginx:latest --target registry.example.com/nginx:v1 --all-arch --username user --password pass

# Pull and push specific architectures to private registry
./imgMigrate push --source nginx:latest --target registry.example.com/nginx:v1 --arch amd64,arm64 --username user --password pass

# Specify operating system(s) and disable multi-arch manifest creation
./imgMigrate push --source nginx:latest --target registry.example.com/nginx:v1 --all-arch --os linux --create-multi-arch=false

# Using insecure registry
./imgMigrate push --source nginx:latest --target registry.example.com/nginx:v1 --all-arch --insecure
```

### Using YAML configuration

YAML configuration allows you to define multiple tasks in a single file, making it easier to process batches of images.

#### Generate a sample configuration file:

```bash
./imgMigrate from-config --generate sample-config.yaml
```

#### Example configuration file:

```yaml
registry:
  url: registry.example.com
  username: username
  password: password
  insecure: false
images:
  - source: nginx:latest
    all_architectures: true
    save: true
    output_dir: ./output
    compress: true
    operating_systems:
      - linux
    create_multi_arch: true
  - source: ubuntu:latest
    target: registry.example.com/ubuntu:v1
    architectures:
      - amd64
      - arm64
    operating_systems:
      - linux
    create_multi_arch: true
```

#### YAML Configuration Fields:

**Registry**:
- `url`: Private registry URL
- `username`: Username for registry authentication
- `password`: Password for registry authentication
- `insecure`: Allow insecure registry connections if true

**Images**:
- `source` (required): Source image to pull from DockerHub (e.g., nginx:latest)
- `target` (optional): Target image for pushing to registry
- `architectures` (optional): List of architectures to process (e.g., amd64, arm64, arm/v7)
- `all_architectures` (optional): Process all available architectures if true
- `save` (optional): Save images to local filesystem if true
- `output_dir` (optional): Directory where images will be saved (defaults to current directory)
- `compress` (optional): Use gzip compression for saved images if true
- `operating_systems` (optional): List of operating systems to filter (e.g., linux, windows)
- `create_multi_arch` (optional): Create a multi-architecture manifest if true

Either `all_architectures` must be true or `architectures` must be specified.
Either `target` must be specified or `save` must be true.

#### Run with configuration file:

```bash
./imgMigrate from-config --file config.yaml
```

## Examples

### Example 1: Save all architectures of Nginx with compression

```bash
./imgMigrate pull --source nginx:latest --all-arch --output ./nginx-images --compress
```

### Example 2: Push Ubuntu images to private registry using YAML

Create a config file `ubuntu-config.yaml`:
```yaml
registry:
  url: registry.internal.com
  username: admin
  password: password123
images:
  - source: ubuntu:20.04
    target: registry.internal.com/ubuntu:20.04
    architectures: 
      - amd64
      - arm64
    operating_systems:
      - linux
```

Run with the config file:
```bash
./imgMigrate from-config --file ubuntu-config.yaml
```

### Example 3: Save locally and push to registry with specific OS filter

```yaml
registry:
  url: registry.internal.com
  username: admin
  password: password123
images:
  - source: redis:6
    target: registry.internal.com/redis:6
    all_architectures: true
    save: true
    output_dir: ./redis-images
    compress: true
    operating_systems:
      - linux
    create_multi_arch: true
```