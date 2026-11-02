package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"deepzora/desktop/internal/update"
)

// A stalled mirror must eventually yield to GitHub. This bounds the entire R2
// transfer, including its transport retries and signature fetch.
var mirrorDownloadTimeout = 5 * time.Minute

// updateAssetSources derives mirror paths only from an exact, validated official
// asset identity. The manifest and cached identity keep their canonical GitHub
// URLs so old clients and platform installers share the same signed bytes.
func updateAssetSources(selected, version string, asset update.Asset) ([]update.Asset, error) {
	if normalizeUpdateChannel(selected) != "stable" || !stableDesktopVersionRE.MatchString(version) {
		return nil, fmt.Errorf("update: invalid stable asset identity")
	}
	githubBase := githubReleaseAssetBase + "/desktop-" + version + "/"
	for _, required := range requiredDesktopAssets(version) {
		if required.group != "downloads" || asset.URL != githubBase+required.filename {
			continue
		}
		if _, err := validateManifestAsset("stable", version, required.filename, asset); err != nil {
			return nil, err
		}
		mirror := asset
		mirror.URL = r2DesktopAssetBase + "/desktop-" + version + "/" + required.filename
		mirror.Sig = mirror.URL + ".minisig"
		return []update.Asset{mirror, asset}, nil
	}
	return nil, fmt.Errorf("update: asset is not an official download for %s", version)
}

// downloadVerifiedAsset switches origins only between complete attempts: no
// partial bytes or detached signature are carried from R2 into GitHub. Every
// successful source must pass size, signature and SHA-256 checks. Production
// supplies update.Verify, backed by the embedded Deepzora public key.
func downloadVerifiedAsset(
	ctx context.Context, c, ipv4 *http.Client, selected, version string, asset update.Asset,
	progress func(received, total int64), verifying func(), verify func(data, sig []byte) error,
) ([]byte, []byte, error) {
	sources, err := updateAssetSources(selected, version, asset)
	if err != nil {
		return nil, nil, err
	}
	if verify == nil {
		return nil, nil, fmt.Errorf("update: signature verifier is required")
	}
	var errs []error
	for i, source := range sources {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		sourceCtx := ctx
		cancel := func() {}
		if i == 0 {
			sourceCtx, cancel = context.WithTimeout(ctx, mirrorDownloadTimeout)
		}
		if progress != nil {
			progress(0, asset.Size)
		}
		data, sig, err := downloadVerifiedSource(sourceCtx, c, ipv4, selected, source, progress, verifying, verify)
		cancel()
		if err == nil {
			return data, sig, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", source.URL, err))
	}
	return nil, nil, fmt.Errorf("update: no verified download source: %w", errors.Join(errs...))
}

func downloadVerifiedSource(
	ctx context.Context, c, ipv4 *http.Client, selected string, asset update.Asset,
	progress func(received, total int64), verifying func(), verify func(data, sig []byte) error,
) ([]byte, []byte, error) {
	data, err := downloadForChannel(ctx, c, ipv4, selected, asset.URL, asset.Size, progress)
	if err != nil {
		return nil, nil, err
	}
	if verifying != nil {
		verifying()
	}
	sig, err := fetchBytesFallbackForChannelSized(ctx, c, ipv4, selected, asset.Sig, maxDesktopSignatureSize)
	if err != nil {
		return nil, nil, err
	}
	if err := verify(data, sig); err != nil {
		return nil, nil, err
	}
	if err := checkSHA256(data, asset.SHA256); err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return data, sig, nil
}
