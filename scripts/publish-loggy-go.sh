#!/bin/bash

# Publish loggy-go to GitHub (Go modules are published via git tags)
# Usage: ./scripts/publish-loggy-go.sh <version>
# Example: ./scripts/publish-loggy-go.sh v0.1.0

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
SDK_DIR="$PROJECT_ROOT"

cd "$SDK_DIR"

# Check if version argument is provided
if [ -z "$1" ]; then
    echo "Error: Version argument required"
    echo "Usage: ./scripts/publish-loggy-go.sh <version>"
    echo "Example: ./scripts/publish-loggy-go.sh v0.1.0"
    exit 1
fi

VERSION="$1"

# Ensure version starts with 'v'
if [[ ! "$VERSION" =~ ^v ]]; then
    VERSION="v$VERSION"
fi

echo "Publishing loggy-go version $VERSION..."

# Run tests
echo "Running tests..."
go test -v ./...

# Ensure go.mod is tidy
echo "Tidying go.mod..."
go mod tidy

# Check if there are uncommitted changes
cd "$PROJECT_ROOT"
if [[ -n $(git status --porcelain loggy-go/) ]]; then
    echo "Warning: There are uncommitted changes in loggy-go/"
    echo "Please commit your changes before publishing."
    git status --porcelain loggy-go/
    exit 1
fi

# Create and push tag for the loggy-go module
# Since this is the root of its own repo, use plain version tags
TAG="$VERSION"

echo "Creating tag $TAG..."
git tag "$TAG"

echo "Pushing tag to origin..."
git push origin "$TAG"

echo ""
echo "✅ Successfully published loggy-go@$VERSION"
echo ""
echo "The module will be available at:"
echo "  github.com/loggy-dev/loggy-go@$VERSION"
echo ""
echo "Install with:"
echo "  go get github.com/loggy-dev/loggy-go@$VERSION"
