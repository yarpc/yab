#!/bin/bash -e

if [ -z "$GITHUB_TOKEN" ]; then
  echo "GITHUB_TOKEN is not set"
  exit 1
fi

VERSION="$1"
if [ -z "$VERSION" ]; then
  echo "USAGE: $0 VERSION"
  exit 1
fi

./scripts/cross_build.sh "$VERSION"

CHANGELOG=$(go run scripts/extract_changelog.go "$VERSION")

echo "Releasing $VERSION"
echo ""
echo "CHANGELOG:"
echo "$CHANGELOG"
echo ""

# ghr needs either a directory or a single file so we'll move the files we
# want to release into their own folder.
RELEASEDIR="release-$VERSION"
mkdir -p "$RELEASEDIR"
mv build/yab-"$VERSION"-*.zip "$RELEASEDIR"

ghr \
  -owner yarpc \
  -token "$GITHUB_TOKEN" \
  -body "$CHANGELOG" \
  "$VERSION" "$RELEASEDIR"
rm -r "$RELEASEDIR"
