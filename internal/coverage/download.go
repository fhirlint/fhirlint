package coverage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1" //nolint:gosec // the registry publishes SHA-1; see publishedSHA
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultRegistry is the canonical FHIR package registry.
const DefaultRegistry = "https://packages.fhir.org"

// Limits on what a package tarball may expand to. A FHIR package is a few
// hundred files of JSON; these are far above anything legitimate and exist so
// that a malformed or hostile archive cannot fill the disk.
const (
	tarMaxFiles      = 100_000
	tarMaxTotalBytes = 512 << 20
	tarMaxFileBytes  = 64 << 20
)

const downloadTimeout = 5 * time.Minute

// Downloader fetches IG packages from a FHIR package registry into the local
// package cache.
type Downloader struct {
	Registry string
	HTTP     *http.Client
}

// NewDownloader returns a Downloader pointed at the default registry.
func NewDownloader() *Downloader {
	return &Downloader{
		Registry: DefaultRegistry,
		HTTP:     &http.Client{Timeout: downloadTimeout},
	}
}

// Fetch downloads name#version and installs it into cacheRoot, so that the
// package's files end up where PackageDir expects them.
//
// The returned warnings are conditions the caller must surface but which do not
// invalidate the download — an unobtainable checksum, most of all.
func (d *Downloader) Fetch(ctx context.Context, cacheRoot, name, version string) ([]string, error) {
	var warnings []string

	// Ask for the published checksum first. A mismatch later is fatal, so it is
	// better to discover the registry is unreachable before spending the
	// download.
	want, err := d.publishedSHA(ctx, name, version)
	if err != nil {
		// Matching how the validator JAR is handled: a checksum that cannot be
		// obtained is a loud warning rather than a hard failure, because the
		// alternative is that a registry hiccup blocks work entirely. What it
		// must never be is silent (#260).
		warnings = append(warnings, fmt.Sprintf(
			"could not verify the checksum of %s#%s: %v — the package was installed unverified",
			name, version, err))
	}

	body, err := d.download(ctx, name, version)
	if err != nil {
		return warnings, err
	}

	if want != "" {
		got := sha1.Sum(body) //nolint:gosec // matching the registry's own published digest
		if hex.EncodeToString(got[:]) != want {
			return warnings, fmt.Errorf(
				"checksum mismatch for %s#%s: registry published %s, downloaded archive is %s — "+
					"the download may be corrupted or tampered with, and it was not installed",
				name, version, want, hex.EncodeToString(got[:]))
		}
	}

	return warnings, install(cacheRoot, name, version, body)
}

// publishedSHA reads dist.shasum for a version from the registry's packument.
//
// The digest is SHA-1 because that is what the npm packument format the
// registry follows carries. It is worth checking anyway: it catches a truncated
// or corrupted transfer and a tampering CDN. It is not a defence against the
// registry itself, since the digest and the archive come from the same source —
// stating that plainly is more useful than implying a guarantee it cannot give.
func (d *Downloader) publishedSHA(ctx context.Context, name, version string) (string, error) {
	endpoint := strings.TrimSuffix(d.registry(), "/") + "/" + url.PathEscape(name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := d.client().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching package metadata", resp.StatusCode)
	}

	var doc struct {
		Versions map[string]struct {
			Dist struct {
				Shasum string `json:"shasum"`
			} `json:"dist"`
		} `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", fmt.Errorf("parsing package metadata: %w", err)
	}
	v, ok := doc.Versions[version]
	if !ok || v.Dist.Shasum == "" {
		return "", errors.New("no checksum published for this version")
	}
	return strings.ToLower(v.Dist.Shasum), nil
}

func (d *Downloader) download(ctx context.Context, name, version string) ([]byte, error) {
	endpoint := strings.TrimSuffix(d.registry(), "/") + "/" +
		url.PathEscape(name) + "/" + url.PathEscape(version)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s#%s: %w", name, version, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("IG package %s#%s does not exist in the registry — check the name and version", name, version)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s#%s: HTTP %d", name, version, resp.StatusCode)
	}

	// Bounded so a hostile or broken response cannot be read without limit. The
	// extra byte distinguishes "exactly at the limit" from "over it".
	body, err := io.ReadAll(io.LimitReader(resp.Body, tarMaxTotalBytes+1))
	if err != nil {
		return nil, fmt.Errorf("downloading %s#%s: %w", name, version, err)
	}
	if len(body) > tarMaxTotalBytes {
		return nil, fmt.Errorf("package %s#%s exceeds the %d MiB download limit", name, version, tarMaxTotalBytes>>20)
	}
	return body, nil
}

func (d *Downloader) registry() string {
	if d.Registry == "" {
		return DefaultRegistry
	}
	return d.Registry
}

func (d *Downloader) client() *http.Client {
	if d.HTTP == nil {
		return &http.Client{Timeout: downloadTimeout}
	}
	return d.HTTP
}

// install unpacks the archive into the cache.
//
// Extraction goes to a temporary directory and is renamed into place only once
// it has completed. A half-written package directory would otherwise look
// exactly like a cached one on the next run, and every later coverage report
// would quietly be missing profiles.
func install(cacheRoot, name, version string, archive []byte) error {
	if err := os.MkdirAll(cacheRoot, 0o750); err != nil {
		return err
	}

	tmp, err := os.MkdirTemp(cacheRoot, ".fhirlint-download-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := extractTarGz(archive, tmp); err != nil {
		return fmt.Errorf("unpacking %s#%s: %w", name, version, err)
	}

	final := filepath.Join(cacheRoot, name+"#"+version)
	if err := os.Rename(tmp, final); err != nil {
		// Another process may have installed the same package meanwhile, which
		// is a race worth losing rather than failing on.
		if _, statErr := os.Stat(final); statErr == nil {
			return nil
		}
		return fmt.Errorf("installing %s#%s: %w", name, version, err)
	}
	return nil
}

// extractTarGz unpacks a gzipped tar into dest.
//
// Only regular files and directories are written. Symlinks and hard links are
// skipped rather than followed: a link is the standard way to make an archive
// write outside its destination, and a FHIR package has no legitimate use for
// one. Entry paths are resolved against dest and rejected if they escape it.
func extractTarGz(archive []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("not a gzip archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var files int
	var total int64

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		files++
		if files > tarMaxFiles {
			return fmt.Errorf("archive contains more than %d entries", tarMaxFiles)
		}

		target, err := safeJoin(dest, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if hdr.Size > tarMaxFileBytes {
				return fmt.Errorf("archive entry %q exceeds the %d MiB per-file limit", hdr.Name, tarMaxFileBytes>>20)
			}
			total += hdr.Size
			if total > tarMaxTotalBytes {
				return fmt.Errorf("archive expands to more than %d MiB", tarMaxTotalBytes>>20)
			}
			if err := writeFile(target, tr, hdr.Size); err != nil {
				return err
			}
		default:
			// Symlinks, devices, FIFOs: skipped deliberately.
			continue
		}
	}
	return nil
}

func writeFile(target string, r io.Reader, size int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // path validated by safeJoin
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	// Copy bounded by the declared size, so a header that understates the
	// payload cannot be used to slip past the running total above.
	if _, err := io.Copy(f, io.LimitReader(r, size)); err != nil {
		return err
	}
	return f.Close()
}

// safeJoin resolves an archive entry path against dest and refuses anything
// that would land outside it.
//
// A traversal is rejected rather than neutralised. Cleaning "../../x" into
// "dest/x" would be safe in the sense that nothing escapes, but it silently
// turns a malformed or hostile archive into a successful install in which an
// entry has quietly taken another entry's filename. An archive containing ".."
// has no legitimate reading, so the install stops.
func safeJoin(dest, name string) (string, error) {
	if name == "" {
		return "", errors.New("archive entry with an empty name")
	}

	// Backslashes are not path separators in a tar entry, but they are on
	// Windows once the name reaches the filesystem, so they are normalised
	// before the checks rather than after.
	slashed := strings.ReplaceAll(filepath.ToSlash(name), `\`, "/")

	if strings.HasPrefix(slashed, "/") || filepath.IsAbs(name) {
		return "", fmt.Errorf("archive entry %q has an absolute path", name)
	}
	for _, part := range strings.Split(slashed, "/") {
		if part == ".." {
			return "", fmt.Errorf("archive entry %q escapes the destination", name)
		}
	}

	target := filepath.Join(dest, filepath.FromSlash(slashed))

	// Defence in depth: whatever the checks above missed, the result still has
	// to sit inside dest.
	root := filepath.Clean(dest) + string(os.PathSeparator)
	if target != filepath.Clean(dest) && !strings.HasPrefix(target, root) {
		return "", fmt.Errorf("archive entry %q would be written outside the destination", name)
	}
	return target, nil
}
