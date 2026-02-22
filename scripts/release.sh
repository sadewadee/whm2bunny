#!/bin/bash
# Release script for whm2bunny
# Usage: ./scripts/release.sh [version]

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Get version from argument or prompt
VERSION=${1:-}
if [ -z "$VERSION" ]; then
    echo -e "${YELLOW}Enter version (e.g., v1.0.0):${NC}"
    read -r VERSION
    if [ -z "$VERSION" ]; then
        echo -e "${RED}Version is required${NC}"
        exit 1
    fi
fi

# Validate version format
if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-z0-9.]+)?$ ]]; then
    echo -e "${RED}Invalid version format. Use: v1.0.0 or v1.0.0-beta.1${NC}"
    exit 1
fi

echo -e "${GREEN}=== whm2bunny Release $VERSION ===${NC}"
echo ""

# Check if git is clean
if [ -n "$(git status --porcelain)" ]; then
    echo -e "${RED}Error: Working directory is not clean${NC}"
    echo "Please commit or stash your changes first"
    exit 1
fi

# Check if tag already exists
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    echo -e "${RED}Error: Tag $VERSION already exists${NC}"
    exit 1
fi

# Build for all platforms
echo -e "${YELLOW}Building for all platforms...${NC}"
make release

# Create git tag
echo -e "${YELLOW}Creating git tag...${NC}"
git tag -a "$VERSION" -m "Release $VERSION"

# Show release info
echo ""
echo -e "${GREEN}=== Release Ready ===${NC}"
echo "Version: $VERSION"
echo "Tag: $VERSION"
echo ""
echo "Built artifacts:"
ls -lh dist/
echo ""
echo -e "${YELLOW}To publish:${NC}"
echo "  git push origin $VERSION"
echo "  gh release create $VERSION ./dist/*"
