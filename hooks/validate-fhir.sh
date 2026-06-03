#!/usr/bin/env bash
#
# fhirlint PostToolUse hook — validate a FHIR resource right after Claude edits it.
#
# Fires after Edit/Write/MultiEdit. When it finds validation issues it feeds them
# back into the session (PostToolUse additionalContext) so Claude can fix the
# resource it just touched.
#
# Opt-in: this hook does nothing unless FHIRLINT_AUTOVALIDATE is set to a truthy
# value (1/true/yes/on). Installing the plugin therefore does not change your
# editing behaviour until you explicitly enable it.
#
# It also no-ops silently when:
#   - jq or the fhirlint binary is not on PATH,
#   - the edited file is not a FHIR resource (.json/.xml with a resourceType /
#     the FHIR namespace),
#   - the validator JAR is not cached yet — so an automatic edit never triggers a
#     surprise ~250 MB download. Run `fhirlint validate` once to prime it.

set -uo pipefail

# --- opt-in gate --------------------------------------------------------------
case "${FHIRLINT_AUTOVALIDATE:-}" in
  1 | true | TRUE | yes | on) ;;
  *) exit 0 ;;
esac

# --- prerequisites (degrade quietly) -----------------------------------------
command -v jq >/dev/null 2>&1 || exit 0
command -v fhirlint >/dev/null 2>&1 || exit 0

input="$(cat)"
file="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty')"
cwd="$(printf '%s' "$input" | jq -r '.cwd // empty')"
[ -n "$file" ] || exit 0

# --- only FHIR-ish files ------------------------------------------------------
ext="$(printf '%s' "$file" | tr '[:upper:]' '[:lower:]')"
case "$ext" in
  *.json | *.xml) ;;
  *) exit 0 ;;
esac
[ -f "$file" ] || exit 0

# A FHIR resource has a resourceType (JSON) or the FHIR namespace (XML). This
# keeps the hook from validating unrelated json/xml files.
grep -qE '"resourceType"|hl7\.org/fhir' "$file" 2>/dev/null || exit 0

# --- stay idle until the JAR is cached ---------------------------------------
jar="${FHIRLINT_JAR:-$HOME/.fhirlint/validator_cli.jar}"
[ -f "$jar" ] || exit 0

# Run from the project directory so fhirlint.yml, .fhirlintignore and suppression
# rules apply exactly as they do on the CLI.
if [ -n "$cwd" ]; then
  cd "$cwd" 2>/dev/null || true
fi

# Validate only the file that was just touched (no validation storms on
# multi-file edits). A non-zero exit means issues were found — that is expected.
report="$(fhirlint validate "$file" --format json 2>/dev/null)" || true
[ -n "$report" ] || exit 0

# Only errors/fatals — the cases where this edit actually broke validation.
# Advisory warnings (e.g. dom-6 best-practice) would nag on every edit; use the
# `validate` skill or `fhirlint validate` directly to see those.
findings="$(printf '%s' "$report" | jq -r '
  [ .files[]?.issues[]?
    | select(.severity == "error" or .severity == "fatal") ]
  | map("  [" + .severity + "] " + .message
        + (if (.location // "") != "" then " @ " + .location else "" end))
  | .[]
' 2>/dev/null)"

# Nothing actionable: stay silent so clean edits add no noise.
[ -n "$findings" ] || exit 0

context="fhirlint found validation errors in $(basename "$file") after this edit:
$findings

Fix these so the resource validates against FHIR."

jq -n --arg ctx "$context" '{
  hookSpecificOutput: {
    hookEventName: "PostToolUse",
    additionalContext: $ctx
  }
}'
exit 0
