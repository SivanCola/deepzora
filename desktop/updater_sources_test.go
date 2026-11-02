package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"aead.dev/minisign"
	"deepzora/desktop/internal/update"
)

func updateResponse(status int, body []byte) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), ContentLength: int64(len(body)), Body: io.NopCloser(bytes.NewReader(body))}
}

func TestFetchManifestMirrorPreferenceAndFallback(t *testing.T) {
	valid := validDesktopManifest(t, "stable", "v0.0.3")
	validBytes, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"healthy", "404", "403", "500", "timeout", "html", "oversized", "invalid identity", "wrong origin", "incomplete", "prerelease", "both failed"} {
		t.Run(mode, func(t *testing.T) {
			var calls []string
			client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
				calls = append(calls, req.URL.String())
				if req.URL.String() == githubManifestEndpoint {
					if mode == "both failed" {
						return updateResponse(404, nil), nil
					}
					return updateResponse(200, validBytes), nil
				}
				if req.URL.String() != r2ManifestEndpoint {
					t.Fatalf("unexpected endpoint %s", req.URL)
				}
				switch mode {
				case "healthy":
					return updateResponse(200, validBytes), nil
				case "404", "403", "500", "both failed":
					status := map[string]int{"404": 404, "403": 403, "500": 500, "both failed": 503}[mode]
					return updateResponse(status, nil), nil
				case "timeout":
					return nil, context.DeadlineExceeded
				case "html":
					return updateResponse(200, []byte("<html>challenge</html>")), nil
				case "oversized":
					return updateResponse(200, bytes.Repeat([]byte("x"), int(maxDesktopManifestSize)+1)), nil
				}
				var broken update.Manifest
				if err := json.Unmarshal(validBytes, &broken); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "invalid identity":
					broken.DownloadPage = "https://example.com/releases"
				case "wrong origin":
					a := broken.Platforms["darwin-arm64"]
					a.URL = "https://downloads.deepzora.app/evil.zip"
					broken.Platforms["darwin-arm64"] = a
				case "incomplete":
					delete(broken.Platforms, "darwin-arm64")
				case "prerelease":
					broken.Version = "v0.0.3-rc.1"
				}
				body, err := json.Marshal(broken)
				if err != nil {
					t.Fatal(err)
				}
				return updateResponse(200, body), nil
			})}
			got, err := fetchManifest(context.Background(), client, nil, "stable")
			if mode == "both failed" {
				if err == nil || !strings.Contains(err.Error(), r2ManifestEndpoint) || !strings.Contains(err.Error(), githubManifestEndpoint) {
					t.Fatalf("lost source errors: %v", err)
				}
			} else if err != nil || !reflect.DeepEqual(got, &valid) {
				t.Fatalf("manifest = %+v, error = %v", got, err)
			}
			want := []string{r2ManifestEndpoint, githubManifestEndpoint}
			if mode == "healthy" {
				want = want[:1]
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls = %v, want %v", calls, want)
			}
		})
	}
}

func TestFetchManifestCancellationAndVersionBinding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != r2ManifestEndpoint {
			t.Fatal("cancelled check reached GitHub")
		}
		cancel()
		return nil, context.Canceled
	})}
	if _, err := fetchManifest(ctx, client, nil, "stable"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	// A changed pointer must still force a new check, never silently install a
	// different version because the mirror or fallback advanced meanwhile.
	if err := ensureExpectedUpdateVersion("stable", "v0.0.3", "v0.0.4"); err == nil {
		t.Fatal("fallback changed the version bound to the request")
	}
}

func TestFetchManifestStalledMirrorYieldsToGitHub(t *testing.T) {
	m := validDesktopManifest(t, "stable", "v0.0.3")
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.String())
		if req.URL.String() == r2ManifestEndpoint {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}
		return updateResponse(200, body), nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	manifest, err := fetchManifest(ctx, client, nil, "stable")
	if err != nil || manifest.Version != m.Version || !reflect.DeepEqual(calls, []string{r2ManifestEndpoint, githubManifestEndpoint}) {
		t.Fatalf("stalled mirror did not yield: manifest=%+v err=%v calls=%v", manifest, err, calls)
	}
}

func TestUpdateAssetSourcesRequireExactOfficialIdentity(t *testing.T) {
	m := validDesktopManifest(t, "stable", "v0.0.3")
	for _, required := range requiredDesktopAssets(m.Version) {
		if required.group != "downloads" {
			continue
		}
		asset := m.Downloads[required.filename]
		sources, err := updateAssetSources("stable", m.Version, asset)
		if err != nil || len(sources) != 2 || sources[0].URL != r2DesktopAssetBase+"/desktop-v0.0.3/"+required.filename || sources[0].Sig != sources[0].URL+".minisig" || sources[1] != asset || sources[0].SHA256 != asset.SHA256 || sources[0].Size != asset.Size || sources[0].InstallLayout != asset.InstallLayout {
			t.Fatalf("sources = %+v, error = %v", sources, err)
		}
	}
	asset, ok := m.Asset()
	if !ok {
		t.Fatal("fixture has no current platform")
	}
	for _, mutate := range []func(*update.Asset){
		func(a *update.Asset) { a.URL = strings.ReplaceAll(a.URL, "github.com", "github.com.evil.invalid") },
		func(a *update.Asset) { a.URL += "?mirror=true" },
		func(a *update.Asset) { a.URL = strings.ReplaceAll(a.URL, "desktop-v0.0.3", "desktop-v0.0.2") },
		func(a *update.Asset) { a.URL = githubReleaseAssetBase + "/desktop-v0.0.3/../evil.zip" },
		func(a *update.Asset) { a.Sig = "https://example.com/signature" },
		func(a *update.Asset) { a.SHA256 = "" },
		func(a *update.Asset) { a.Size = 0 },
		func(a *update.Asset) { a.InstallLayout = "unknown" },
	} {
		bad := asset
		mutate(&bad)
		if _, err := updateAssetSources("stable", m.Version, bad); err == nil {
			t.Fatalf("accepted unsafe source: %+v", bad)
		}
	}
}

func TestDownloadVerifiedAssetMirrorFallbackAndIntegrity(t *testing.T) {
	fastRetry(t)
	body := []byte("complete official signed update archive")
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := pub.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	signature := minisign.Sign(priv, body)
	otherBody := bytes.Repeat([]byte("x"), len(body))
	otherSignature := minisign.Sign(priv, otherBody)
	digest := sha256.Sum256(body)
	verify := func(data, sig []byte) error { return update.VerifyWithKey(string(key), data, sig) }
	m := validDesktopManifest(t, "stable", "v0.0.3")
	asset, ok := m.Asset()
	if !ok {
		t.Fatal("fixture has no current platform")
	}
	asset.Size, asset.SHA256 = int64(len(body)), hex.EncodeToString(digest[:])
	for _, mode := range []string{"healthy", "404", "403", "500", "timeout", "reset", "short", "oversized", "corrupt bytes", "other signed payload", "signature 404", "oversized signature", "corrupt signature", "both corrupt", "wrong hash"} {
		t.Run(mode, func(t *testing.T) {
			var calls []string
			var resets int
			client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
				calls = append(calls, req.URL.String())
				mirror := req.URL.Host == "downloads.deepzora.app"
				isSignature := strings.HasSuffix(req.URL.Path, ".minisig")
				if !mirror && req.Header.Get("Range") != "" {
					t.Fatal("partial mirror bytes leaked into GitHub request")
				}
				if mirror {
					switch mode {
					case "404", "403", "500":
						return updateResponse(map[string]int{"404": 404, "403": 403, "500": 500}[mode], nil), nil
					case "timeout":
						return nil, context.DeadlineExceeded
					case "reset":
						if resets++; resets <= downloadAttempts {
							return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: int64(len(body)), Body: &updateBrokenBody{data: body[:5]}}, nil
						}
					case "short":
						return updateResponse(200, body[:5]), nil
					case "oversized":
						return updateResponse(200, append(append([]byte{}, body...), 'x')), nil
					}
				}
				if isSignature {
					if mirror && mode == "other signed payload" {
						return updateResponse(200, otherSignature), nil
					}
					if mirror && mode == "oversized signature" {
						return updateResponse(200, bytes.Repeat([]byte("x"), int(maxDesktopSignatureSize)+1)), nil
					}
					if mirror && mode == "signature 404" {
						return updateResponse(404, nil), nil
					}
					if (mirror && mode == "corrupt signature") || mode == "both corrupt" {
						return updateResponse(200, []byte("invalid signature")), nil
					}
					return updateResponse(200, signature), nil
				}
				if mirror && (mode == "corrupt bytes" || mode == "other signed payload") {
					return updateResponse(200, otherBody), nil
				}
				return updateResponse(200, body), nil
			})}
			candidate := asset
			if mode == "wrong hash" {
				candidate.SHA256 = strings.Repeat("a", 64)
			}
			var progressResets int
			data, sig, err := downloadVerifiedAsset(context.Background(), client, nil, "stable", m.Version, candidate, func(received, _ int64) {
				if received == 0 {
					progressResets++
				}
			}, nil, verify)
			if mode == "both corrupt" || mode == "wrong hash" {
				if err == nil || data != nil || sig != nil {
					t.Fatalf("invalid update accepted: %q, %q, %v", data, sig, err)
				}
				if !strings.Contains(err.Error(), r2DesktopAssetBase) || !strings.Contains(err.Error(), asset.URL) {
					t.Fatalf("lost source failure details: %v", err)
				}
			} else if err != nil || !bytes.Equal(data, body) || !bytes.Equal(sig, signature) {
				t.Fatalf("download = %q, %q, %v", data, sig, err)
			}
			if len(calls) == 0 || !strings.HasPrefix(calls[0], r2DesktopAssetBase+"/") {
				t.Fatalf("mirror was not first: %v", calls)
			}
			githubCalls := 0
			for _, call := range calls {
				if strings.HasPrefix(call, githubReleaseAssetBase+"/") {
					githubCalls++
					if call != asset.URL && call != asset.Sig {
						t.Fatalf("fallback changed artifact identity: %s", call)
					}
				}
			}
			if mode == "healthy" {
				if githubCalls != 0 || progressResets != 1 {
					t.Fatalf("healthy mirror reached GitHub or restarted: %v", calls)
				}
			} else if githubCalls != 2 || progressResets != 2 {
				t.Fatalf("fallback did not retry the complete pair: calls=%v resets=%d", calls, progressResets)
			}
		})
	}
}

func TestDownloadVerifiedAssetIPv4RecoveryAndUntrustedRedirect(t *testing.T) {
	fastRetry(t)
	body := []byte("signed payload served by the fallback transport")
	pub, privateKey, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := pub.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	sig := minisign.Sign(privateKey, body)
	asset, ok := validDesktopManifest(t, "stable", "v0.0.3").Asset()
	if !ok {
		t.Fatal("fixture has no current platform")
	}
	digest := sha256.Sum256(body)
	asset.Size, asset.SHA256 = int64(len(body)), hex.EncodeToString(digest[:])
	verify := func(data, signature []byte) error { return update.VerifyWithKey(string(key), data, signature) }
	for _, redirect := range []bool{false, true} {
		var githubCalls, ipv4Calls int
		serve := func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "evil.invalid" {
				t.Fatal("followed an untrusted update redirect")
			}
			if req.URL.Host == "github.com" {
				githubCalls++
			}
			if strings.HasSuffix(req.URL.Path, ".minisig") {
				return updateResponse(200, sig), nil
			}
			return updateResponse(200, body), nil
		}
		primary := &http.Client{CheckRedirect: validateUpdateRedirect, Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "downloads.deepzora.app" {
				if redirect {
					response := updateResponse(302, nil)
					response.Header.Set("Location", "https://evil.invalid/stolen.zip")
					return response, nil
				}
				return nil, io.ErrUnexpectedEOF
			}
			return serve(req)
		})}
		ipv4 := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			ipv4Calls++
			if redirect && req.URL.Host == "downloads.deepzora.app" {
				return updateResponse(404, nil), nil
			}
			return serve(req)
		})}
		data, signature, err := downloadVerifiedAsset(context.Background(), primary, ipv4, "stable", "v0.0.3", asset, nil, nil, verify)
		if err != nil || !bytes.Equal(data, body) || !bytes.Equal(signature, sig) || ipv4Calls == 0 {
			t.Fatalf("redirect=%v: err=%v ipv4=%d", redirect, err, ipv4Calls)
		}
		if (redirect && githubCalls != 2) || (!redirect && githubCalls != 0) {
			t.Fatalf("redirect=%v: GitHub calls=%d", redirect, githubCalls)
		}
	}
}

type updateBrokenBody struct{ data []byte }

func (b *updateBrokenBody) Read(p []byte) (int, error) {
	if len(b.data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, b.data)
	b.data = b.data[n:]
	return n, nil
}
func (*updateBrokenBody) Close() error { return nil }

func TestDownloadVerifiedAssetCancellationAndMirrorDeadline(t *testing.T) {
	fastRetry(t)
	previous := mirrorDownloadTimeout
	mirrorDownloadTimeout = 10 * time.Millisecond
	t.Cleanup(func() { mirrorDownloadTimeout = previous })
	asset, ok := validDesktopManifest(t, "stable", "v0.0.3").Asset()
	if !ok {
		t.Fatal("fixture has no current platform")
	}
	for _, cancelParent := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		githubCalls := 0
		client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "downloads.deepzora.app" {
				if cancelParent {
					cancel()
				}
				<-req.Context().Done()
				return nil, req.Context().Err()
			}
			githubCalls++
			return updateResponse(404, nil), nil
		})}
		_, _, err := downloadVerifiedAsset(ctx, client, nil, "stable", "v0.0.3", asset, nil, nil, update.Verify)
		cancel()
		if err == nil || (cancelParent && (githubCalls != 0 || !errors.Is(err, context.Canceled))) || (!cancelParent && githubCalls != 1) {
			t.Fatalf("cancelParent=%v: calls=%d err=%v", cancelParent, githubCalls, err)
		}
	}
}

func TestAppDownloadVerifyRejectsUntrustedPublisherOnBothSources(t *testing.T) {
	fastRetry(t)
	body := []byte("untrusted release")
	_, privateKey, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sig := minisign.Sign(privateKey, body)
	asset, ok := validDesktopManifest(t, "stable", "v0.0.3").Asset()
	if !ok {
		t.Fatal("fixture has no current platform")
	}
	digest := sha256.Sum256(body)
	asset.Size, asset.SHA256 = int64(len(body)), hex.EncodeToString(digest[:])
	var calls []string
	client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.String())
		if strings.HasSuffix(req.URL.Path, ".minisig") {
			return updateResponse(200, sig), nil
		}
		return updateResponse(200, body), nil
	})}
	original, originalIPv4 := updaterHTTPClient, updaterHTTPClientIPv4
	updaterHTTPClient = func() (*http.Client, error) { return client, nil }
	updaterHTTPClientIPv4 = func() (*http.Client, error) { return nil, nil }
	t.Cleanup(func() { updaterHTTPClient, updaterHTTPClientIPv4 = original, originalIPv4 })
	app := NewApp()
	data, signature, err := app.downloadVerify("test-request", "stable", "v0.0.3", asset)
	if err == nil || data != nil || signature != nil || !strings.Contains(err.Error(), "signature verification failed") || len(calls) != 4 {
		t.Fatalf("untrusted signed release accepted: data=%q sig=%q err=%v calls=%v", data, signature, err, calls)
	}
}
