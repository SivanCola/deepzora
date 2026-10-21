#!/usr/bin/env bash
# Produce and verify a fresh image before replacing a previously built artifact.
set -euo pipefail

app="${1:?usage: package-desktop-dmg.sh <app> <output.dmg> <app-name>}"
output="${2:?missing output image}"
appname="${3:?missing app name}"
mkdir -p "$(dirname "$output")"
staging=$(mktemp -d "$(dirname "$output")/.desktop-dmg.XXXXXX")
staging=$(cd "$staging" && pwd -P)
scripts=$(cd "$(dirname "$0")" && pwd -P)
export DEEPZORA_HDIUTIL="$(command -v hdiutil)"
export DEEPZORA_DMG_STAGING="$staging"
export DEEPZORA_DMG_APP_NAME="$appname"
export DEEPZORA_DMG_FAILURE="$staging/cleanup-failed"
export DEEPZORA_DMG_GUARD="$scripts/desktop-dmg.mjs"
cleanup() {
	status=$?
	trap - EXIT
	if node "$scripts/desktop-dmg.mjs" cleanup-staging "$staging" "$appname"; then
		rm -rf "$staging"
	else
		echo "DMG cleanup failed; retained evidence and backing image at $staging" >&2
		status=1
	fi
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir "$staging/bin"
# Guard create-dmg's own detach calls, including its error paths. Scope the
# PATH override to this child so other applications/mounted images are untouched.
cat > "$staging/bin/hdiutil" <<'WRAPPER'
#!/usr/bin/env bash
exec node "$DEEPZORA_DMG_GUARD" hdiutil "$@"
WRAPPER
chmod +x "$staging/bin/hdiutil"
mkdir "$staging/input"
cp -R "$app" "$staging/input/${appname}.app"
image="$staging/candidate.dmg"

# Some create-dmg versions report Finder customization errors after producing
# a valid image. Verify that newly produced image, never the previous output.
PATH="$staging/bin:$PATH" create-dmg \
	--volname "$appname" \
	--window-size 540 380 \
	--icon-size 110 \
	--icon "${appname}.app" 150 190 \
	--app-drop-link 390 190 \
	--no-internet-enable \
	"$image" "$staging/input" || true
[ ! -e "$DEEPZORA_DMG_FAILURE" ] || { cat "$DEEPZORA_DMG_FAILURE" >&2; exit 1; }
node "$scripts/desktop-dmg.mjs" cleanup-staging "$staging" "$appname"
[ -s "$image" ] || { echo "create-dmg did not produce a fresh image" >&2; exit 1; }
hdiutil verify "$image"
mv -f "$image" "$output"
