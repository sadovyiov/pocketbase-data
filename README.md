# PocketBase Data (pbd)

A command-line tool for seeding and importing data into PocketBase databases.

## Overview

PocketBase Data (pbd) is a Go-based CLI tool that helps you:
- Generate and seed fake data into PocketBase collections based on schema definitions
- Import data from JSON or CSV files into PocketBase collections
- Establish relationships between collections during data generation

## Installation

### Prerequisites
- Go 1.22 or higher
- A running PocketBase instance

### Install from source
```bash
# Clone the repository
git clone https://github.com/sadovojav/pocketbase-data.git
cd pocketbase-data

# Build the binary
go build -o pbd
```

## Usage

### Configuration

Create a configuration file (e.g., `config.yml`) with your PocketBase connection details:

```yaml
url: http://127.0.0.1:8090
email: admin@example.com
password: your_password
```

You can also use environment variables:
```
URL=http://127.0.0.1:8090
EMAIL=admin@example.com
PASSWORD=your_password
```

### Seeding Data

Create a schema file (e.g., `posts.yml`) that defines the structure of your data:

```yaml
fields:
  - name: title
    type: fake
    value: '{sentence:3}'

  - name: content
    type: fake
    value: '{paragraph:2}'

  - name: category
    type: relation
    value: categories

  - name: status
    type: fake
    value: '{randomstring:[published,draft,archived]}'
```

Field types:
- `fake`: Generate fake data using gofakeit patterns
- `relation`: Establish a relationship with another collection
- `dependent`: Use the value from another field
- `custom`: Use a custom value

Then run the seed command:

```shell
pbd seed --config=config.yml --collection=posts --schema=posts.yml --count=10
```

### Importing Data

You can import data from JSON or CSV files:

#### JSON Import
```shell
pbd import --config=config.yml --collection=posts --file=posts.json
```

#### CSV Import
```shell
pbd import --config=config.yml --collection=posts --file=posts.csv
```

## Project Structure

- `examples/`: Example schema and configuration files
- `main.go`: Main application code

## License

This project is licensed under the MIT License.