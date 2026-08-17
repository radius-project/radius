/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bicep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/setup"
	"github.com/radius-project/radius/pkg/version"
)

const (
	// remoteTemplateTimeout bounds the total time spent downloading a remote template.
	remoteTemplateTimeout = 60 * time.Second
	// maxDownloadAttempts is the number of times a transient download failure is retried.
	maxDownloadAttempts = 3
)

// maxRemoteTemplateSize bounds how many bytes are read from a remote template so a large or
// malicious response cannot exhaust memory. It is a variable so tests can lower it.
var maxRemoteTemplateSize int64 = 100 << 20 // 100 MiB

// retryBaseDelay is the base backoff between download attempts when no Retry-After is given. It is
// a variable so tests can lower it.
var retryBaseDelay = 500 * time.Millisecond

// Interface is the interface for interacting with Bicep.
type Interface interface {
	PrepareTemplate(ctx context.Context, filePath string) (map[string]any, error)
	Call(args ...string) ([]byte, error)
}

var _ Interface = (*Impl)(nil)

//go:generate go tool mockgen -typed -destination=./mock_bicep.go -package=bicep -self_package github.com/radius-project/radius/pkg/cli/bicep github.com/radius-project/radius/pkg/cli/bicep Interface

// Impl is the implementation of Interface.
type Impl struct {
	FileSystem filesystem.FileSystem
	Output     output.Interface
	// HTTPClient downloads remote templates. When nil, a default client that refuses to follow an
	// https->http redirect downgrade is used. Injectable so tests can control transport behavior.
	HTTPClient *http.Client
}

// PrepareTemplate checks if the file is a .json or .bicep file, downloads Bicep if it is not installed, checks if the file
// exists, and builds the template if it does. The file may be a local path or an http(s) URL; remote templates are
// downloaded to a temporary local file first. It returns a map of strings to any and an error if one occurs.
func (i *Impl) PrepareTemplate(ctx context.Context, filePath string) (map[string]any, error) {
	// A remote URL is downloaded to a temporary local file so it can be read or compiled like a
	// local template. This mirrors the behavior users expect from tools such as kubectl.
	// displayPath is the only form of the template argument that may be shown to the user, so a
	// credential embedded in a remote URL is never written to the terminal or to CI logs.
	displayPath := RedactTemplatePath(filePath)
	remote := isRemoteURL(filePath)
	if remote {
		localPath, cleanup, err := i.downloadTemplate(ctx, filePath)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		filePath = localPath
	}

	if strings.EqualFold(path.Ext(filePath), ".json") {
		template, err := ReadARMJSON(filePath)
		if err != nil && remote {
			return nil, fmt.Errorf("failed to read remote template %q: %w", displayPath, err)
		}
		return template, err
	} else if !strings.EqualFold(path.Ext(filePath), ".bicep") {
		return nil, fmt.Errorf("the provided file %q must be a .json or .bicep file", displayPath)
	}

	ok, err := IsBicepInstalled()
	if err != nil {
		return nil, fmt.Errorf("failed to find bicep: %w", err)
	}

	if !ok {
		i.Output.LogInfo("Downloading Bicep for channel %s...", version.Channel())
		err = DownloadBicep()
		if err != nil {
			return nil, fmt.Errorf("failed to download bicep: %w", err)
		}
	}

	// Check the file manually so we can control the error message.
	_, err = i.FileSystem.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not find file: %w", err)
	}

	step := i.Output.BeginStep("Building %s...", displayPath)
	bytes, err := i.Call("build", "--stdout", filePath)
	if err != nil {
		i.Output.CompleteStep(step)
		if remote {
			// The bicep compiler prints detailed diagnostics to stderr, so keep the wrapper
			// error focused on identifying the remote source rather than guessing the cause.
			return nil, fmt.Errorf("failed to build remote template %q: %w", displayPath, err)
		}
		return nil, fmt.Errorf("failed to build template: %w", err)
	}

	template := map[string]any{}
	err = json.Unmarshal(bytes, &template)
	if err != nil {
		return nil, err
	}

	i.Output.CompleteStep(step)
	return template, nil
}

// isRemoteURL reports whether filePath is intended as an http(s) URL. It classifies by scheme
// prefix (not by url.Parse) so that a malformed URL is still routed to remote handling and surfaced
// as a URL error, rather than being misread as a local file path. Local paths, including Windows
// paths such as C:\foo.bicep, are not treated as remote URLs.
func isRemoteURL(filePath string) bool {
	lower := strings.ToLower(filePath)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// RedactTemplatePath returns a display-safe form of a `rad` template argument. A local file path is
// returned unchanged; an http(s) URL has its userinfo and query-parameter values redacted so that
// credentials such as SAS tokens are never written to the terminal, to CI logs, or into generated
// output. Callers that display the template argument supplied by the user should format this value
// rather than the raw argument.
func RedactTemplatePath(templatePath string) string {
	if !isRemoteURL(templatePath) {
		return templatePath
	}
	return redactURL(templatePath)
}

// redactURL returns a display-safe copy of a URL with any userinfo and query-parameter values
// removed, so credentials embedded in the URL (basic-auth userinfo or signed query parameters such
// as SAS tokens) are never written to logs or error messages.
func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<redacted url>"
	}
	if parsed.User != nil {
		parsed.User = url.User("redacted")
	}
	if parsed.RawQuery != "" {
		query := parsed.Query()
		for key := range query {
			query[key] = []string{"redacted"}
		}
		parsed.RawQuery = query.Encode()
	}
	// A fragment is never needed to fetch a template, so drop it rather than risk displaying a
	// credential someone placed there.
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String()
}

// TemplateFileName returns the bare file name of a `rad` template argument, with any URL query
// string, fragment, and userinfo removed. Use it instead of filepath.Base when the result is
// written to disk or displayed, because filepath.Base of a URL retains the query string (and any
// credential in it).
func TemplateFileName(templatePath string) string {
	if !isRemoteURL(templatePath) {
		return filepath.Base(templatePath)
	}
	parsed, err := url.Parse(templatePath)
	if err != nil {
		return "template"
	}
	return path.Base(parsed.Path)
}

// urlParseReason extracts the underlying reason from a *url.Error, dropping the raw URL string it
// embeds (which may contain credentials). It applies to both url.Parse failures and http.Client
// transport failures, since both return *url.Error.
func urlParseReason(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// newHTTPClient returns the default client used for downloads. It refuses to follow a redirect that
// downgrades an https request to http so credentials and integrity are not silently lost.
func newHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return fmt.Errorf("refusing to follow redirect from https to %s", req.URL.Scheme)
			}
			return nil
		},
	}
}

// downloadTemplate retrieves a remote template referenced by an http(s) URL and writes it to a
// temporary local file so it can be read or compiled like a local template. It returns the local
// file path and a cleanup function that removes the temporary directory.
func (i *Impl) downloadTemplate(ctx context.Context, templateURL string) (string, func(), error) {
	parsed, err := url.Parse(templateURL)
	if err != nil {
		return "", nil, fmt.Errorf("invalid template URL %q: %w", redactURL(templateURL), urlParseReason(err))
	}
	if parsed.Host == "" {
		return "", nil, fmt.Errorf("invalid template URL %q: missing host", redactURL(templateURL))
	}

	ext := path.Ext(parsed.Path)
	if !strings.EqualFold(ext, ".bicep") && !strings.EqualFold(ext, ".json") {
		return "", nil, fmt.Errorf("the provided URL %q must reference a .json or .bicep file", redactURL(templateURL))
	}

	// display is used in all logs/errors so credentials in the URL are never surfaced.
	display := redactURL(templateURL)
	i.Output.LogInfo("Downloading template from %s...", display)

	body, err := i.fetchRemoteTemplate(ctx, templateURL, display)
	if err != nil {
		return "", nil, err
	}

	dir, err := i.FileSystem.MkdirTemp("", "rad-remote-template-")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary directory for remote template: %w", err)
	}
	cleanup := func() {
		_ = i.FileSystem.RemoveAll(dir)
	}

	// Preserve the original file name so compiler diagnostics reference a recognizable file.
	localPath := filepath.Join(dir, path.Base(parsed.Path))
	if err := i.FileSystem.WriteFile(localPath, body, 0600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to write remote template to temporary file: %w", err)
	}

	// Bicep discovers bicepconfig.json by walking up from the source file's directory, which for a
	// downloaded template is an isolated temp dir. Provide one so extension declarations such as
	// `extension radius` resolve, preferring the user's own config over a generated default.
	if strings.EqualFold(ext, ".bicep") {
		if err := i.writeBicepConfig(dir); err != nil {
			cleanup()
			return "", nil, err
		}
	}

	return localPath, cleanup, nil
}

// fetchRemoteTemplate downloads the template body, retrying transient failures (transport errors,
// 5xx responses, and 429) up to maxDownloadAttempts within an overall timeout. display is the
// redacted URL used in log and error messages.
func (i *Impl) fetchRemoteTemplate(ctx context.Context, templateURL, display string) ([]byte, error) {
	client := i.HTTPClient
	if client == nil {
		client = newHTTPClient()
	}

	ctx, cancel := context.WithTimeout(ctx, remoteTemplateTimeout)
	defer cancel()

	var lastErr error
	for attempt := 1; attempt <= maxDownloadAttempts; attempt++ {
		body, retryAfter, retryable, err := i.attemptDownload(ctx, client, templateURL, display)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable || attempt == maxDownloadAttempts {
			return nil, err
		}

		delay := retryAfter
		if delay <= 0 {
			delay = time.Duration(attempt) * retryBaseDelay
		}
		select {
		case <-ctx.Done():
			return nil, lastErr
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

// attemptDownload performs a single download attempt. It returns whether the failure is retryable
// and any server-provided Retry-After delay.
func (i *Impl) attemptDownload(ctx context.Context, client *http.Client, templateURL, display string) (_ []byte, retryAfter time.Duration, retryable bool, _ error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, templateURL, nil)
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to request template from %q: %w", display, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, false, fmt.Errorf("failed to download template from %q: %w", display, context.Cause(ctx))
		}
		// http.Client returns a *url.Error whose message embeds the raw request URL, so unwrap it
		// to the underlying reason rather than leaking credentials alongside the redacted display.
		return nil, 0, true, fmt.Errorf("failed to download template from %q: %w", display, urlParseReason(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		retryable = resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, parseRetryAfter(resp.Header.Get("Retry-After")), retryable,
			fmt.Errorf("failed to download template from %q: unexpected status %s", display, resp.Status)
	}

	// A GitHub file page (as opposed to the raw URL) returns HTML with 200 OK; reject it clearly
	// instead of letting the HTML reach the Bicep/JSON parser. Other content types are allowed
	// because valid templates are commonly served as text/plain or application/octet-stream.
	if mediaType, _, mErr := mime.ParseMediaType(resp.Header.Get("Content-Type")); mErr == nil && mediaType == "text/html" {
		return nil, 0, false, fmt.Errorf("the URL %q returned an HTML page rather than a template; use the raw file URL", display)
	}

	// Bound the read so a large or malicious response cannot exhaust memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteTemplateSize+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, false, fmt.Errorf("failed to read template from %q: %w", display, context.Cause(ctx))
		}
		return nil, 0, true, fmt.Errorf("failed to read template from %q: %w", display, urlParseReason(err))
	}
	if int64(len(body)) > maxRemoteTemplateSize {
		return nil, 0, false, fmt.Errorf("template from %q exceeds the maximum allowed size of %d bytes", display, maxRemoteTemplateSize)
	}
	return body, 0, false, nil
}

// parseRetryAfter interprets a Retry-After header value (delay-seconds or HTTP-date). It returns 0
// when the value is absent or cannot be parsed.
func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

// writeBicepConfig places a bicepconfig.json in destDir so extension declarations in a downloaded
// template resolve. It reuses the nearest bicepconfig.json found by searching upward from the
// current working directory, falling back to the default Radius extensions configuration.
func (i *Impl) writeBicepConfig(destDir string) error {
	dest := filepath.Join(destDir, "bicepconfig.json")

	if wd, err := os.Getwd(); err == nil {
		if found := findBicepConfig(i.FileSystem, wd); found != "" {
			data, err := i.FileSystem.ReadFile(found)
			if err != nil {
				return fmt.Errorf("failed to read %q: %w", found, err)
			}
			if err := i.FileSystem.WriteFile(dest, data, 0600); err != nil {
				return fmt.Errorf("failed to write bicepconfig.json: %w", err)
			}
			return nil
		}
	}

	if err := i.FileSystem.WriteFile(dest, []byte(setup.GetVersionedBicepConfig()), 0600); err != nil {
		return fmt.Errorf("failed to write bicepconfig.json: %w", err)
	}
	return nil
}

// findBicepConfig walks up from startDir looking for a bicepconfig.json, mirroring how the Bicep
// compiler discovers configuration. It returns "" if none is found.
func findBicepConfig(fs filesystem.FileSystem, startDir string) string {
	dir := startDir
	for {
		candidate := filepath.Join(dir, "bicepconfig.json")
		if _, err := fs.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Call runs `bicep` with the given arguments.
func (i *Impl) Call(args ...string) ([]byte, error) {
	return runBicepRaw(args...)
}

// ConvertToMapStringInterface takes in a map of strings to maps of strings to any type and returns a map of strings to any
// type, with the values of the inner maps being the values of the returned map. No errors are returned.
func ConvertToMapStringInterface(in map[string]map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range in {
		result[k] = v["value"]
	}
	return result
}
