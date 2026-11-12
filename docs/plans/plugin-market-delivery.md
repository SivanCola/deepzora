# Community plugin market delivery plan

1. Define one immutable release per catalog ID and semantic version. The review input identifies a GitHub repo, package subdirectory, manifest identity and a full SHA; the index is server-generated. Separate reviewed releases from topic candidates. Withdraw a release without recycling its version.
2. Bootstrap the independent community repository with contribution instructions, CODEOWNERS, read-only PR validation, a protected main branch and a dist branch. Build the review report from the actual pinned source without running the plugin. Publish only from main. Seed an intentionally resource-only plugin through a real PR; leave the human review visible.
3. Implement one Go resolver for search, provenance, stable version ordering and withdrawal. Only browse or install on user request. Cache the last good catalog for offline browsing; revalidate before writes. Keep custom catalogs visibly separate.
4. Feed a direct repo/tree/SHA/path snapshot to the existing install planner. Bind the preview, installed target, release identity and actual source to confirmation. Reject different-source collisions and preserve disabled state. Install and update share the same preview.
5. Expose search/info/updates and explicit preview/apply in CLI. In Desktop offer Discover, Installed and manual source installation, show author/channel, version, capability changes and the actual plan. Regenerate the host contract from Go.
6. Generate the unreviewed topic index on a schedule. Merge by canonical repository and package path, never display name. Preserve the reviewed default when upstream moves.
7. Exercise the full pipeline with temporary Git repos, isolated Deepzora home and the actual CLI; move a tag between preview/apply, withdraw a version, change the approval ID and try a conflicting source. Validate desktop contracts and UI interactions separately. Report local acceptance, remote PR checks and maintainer approval as distinct results.

