#!/bin/bash
set -euo pipefail

VERSION=${1:-v0.1.0}
FORCE=${2:-}

echo "Creating release $VERSION..."

if git rev-parse "$VERSION" >/dev/null 2>&1; then
  if [[ "${FORCE}" != "--force" ]]; then
    echo "Tag ${VERSION} already exists. Delete it locally or re-run with: $0 ${VERSION} --force" >&2
    exit 1
  fi
  echo "Replacing existing tag ${VERSION} (--force)..."
  git tag -d "${VERSION}"
  git push origin ":refs/tags/${VERSION}" 2>/dev/null || true
fi

git tag -a "${VERSION}" -m "Release ${VERSION}"
git push origin "${VERSION}"

echo "Tag ${VERSION} created and pushed."

origin_url=$(git remote get-url origin 2>/dev/null || true)
repo_path=""
if [[ -n "${origin_url}" ]]; then
  repo_path=$(echo "${origin_url}" | sed -E 's#^(git@github.com:|https://github.com/)##; s#\.git$##')
fi
if [[ -n "${repo_path}" ]]; then
  echo "Actions: https://github.com/${repo_path}/actions"
else
  echo "Open the Actions tab for this repository on GitHub."
fi
