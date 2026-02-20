#!/bin/bash
set -e

VERSION=${1:-v0.1.0}

echo "Creating release $VERSION..."

# Check if tag exists
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    echo "Tag $VERSION already exists. Deleting it..."
    git tag -d "$VERSION"
    git push origin ":refs/tags/$VERSION" 2>/dev/null || true
fi

# Create and push tag
git tag -a "$VERSION" -m "Release $VERSION"
git push origin "$VERSION"

echo "✅ Tag $VERSION created and pushed!"
echo "GitHub Actions will now build and create the release automatically."
echo "Check: https://github.com/$(git remote get-url origin | sed 's/.*github.com[:/]\(.*\)\.git/\1/')/actions"

