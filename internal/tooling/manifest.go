// Package tooling manages the pinned command-line tools used by Radius builds.
package tooling

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

const manifestSchemaVersion = 1

var (
	platformPattern = regexp.MustCompile(`^(linux|darwin)_(amd64|arm64)$`)
	namePattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	makePattern     = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	hashPattern     = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Manifest is the source of truth for the versions, release sources, assets,
// and checksums of the external tools used by the repository.
type Manifest struct {
	SchemaVersion int      `yaml:"schemaVersion"`
	Platforms     []string `yaml:"platforms"`
	Tools         []Tool   `yaml:"tools"`
}

// Tool describes one pinned external tool.
type Tool struct {
	Name             string              `yaml:"name"`
	MakePrefix       string              `yaml:"makePrefix"`
	Version          string              `yaml:"version"`
	Update           *bool               `yaml:"update,omitempty"`
	Notes            string              `yaml:"notes,omitempty"`
	Source           Source              `yaml:"source"`
	DownloadTemplate string              `yaml:"downloadTemplate,omitempty"`
	Platforms        map[string]Platform `yaml:"platforms,omitempty"`
	ChecksumSource   ChecksumSource      `yaml:"checksumSource"`
	VersionFiles     []VersionFile       `yaml:"versionFiles,omitempty"`
}

// Source describes how the updater discovers a tool's latest version.
type Source struct {
	Type       string `yaml:"type"`
	Repository string `yaml:"repository,omitempty"`
	TagPrefix  string `yaml:"tagPrefix,omitempty"`
	LatestURL  string `yaml:"latestURL"`
}

// Platform describes a release asset and its pinned checksum for one target
// platform. Asset names may contain manifest template variables.
type Platform struct {
	Asset    string `yaml:"asset"`
	Checksum string `yaml:"checksum"`
	OS       string `yaml:"os,omitempty"`
	Arch     string `yaml:"arch,omitempty"`
}

// VersionFile describes a repository file that must stay synchronized with a
// tool version when the tool is not only consumed through Make.
type VersionFile struct {
	Path   string `yaml:"path"`
	Format string `yaml:"format"`
	Prefix string `yaml:"prefix,omitempty"`
	Suffix string `yaml:"suffix,omitempty"`
}

// ChecksumSource describes where a release asset's SHA-256 checksum comes from.
type ChecksumSource struct {
	Type              string `yaml:"type"`
	FileTemplate      string `yaml:"fileTemplate,omitempty"`
	OrderFileTemplate string `yaml:"orderFileTemplate,omitempty"`
	URLTemplate       string `yaml:"urlTemplate,omitempty"`
	Format            string `yaml:"format,omitempty"`
	Integrity         string `yaml:"integrity,omitempty"`
}

// LoadManifest reads and validates a tool manifest.
func LoadManifest(path string) (Manifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read tool manifest: %w", err)
	}

	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse tool manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Manifest{}, fmt.Errorf("parse tool manifest: multiple YAML documents are not supported")
		}
		return Manifest{}, fmt.Errorf("parse trailing tool manifest content: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("validate tool manifest: %w", err)
	}
	return manifest, nil
}

// Validate checks that a manifest contains enough information for both Make
// generation and source verification.
func (m Manifest) Validate() error {
	if m.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %d", m.SchemaVersion)
	}
	if len(m.Platforms) == 0 {
		return fmt.Errorf("at least one platform is required")
	}

	seenPlatforms := make(map[string]struct{}, len(m.Platforms))
	for _, platform := range m.Platforms {
		if !platformPattern.MatchString(platform) {
			return fmt.Errorf("invalid platform %q", platform)
		}
		if _, exists := seenPlatforms[platform]; exists {
			return fmt.Errorf("duplicate platform %q", platform)
		}
		seenPlatforms[platform] = struct{}{}
	}

	if len(m.Tools) == 0 {
		return fmt.Errorf("at least one tool is required")
	}
	seenNames := make(map[string]struct{}, len(m.Tools))
	seenPrefixes := make(map[string]struct{}, len(m.Tools))
	for _, tool := range m.Tools {
		if !namePattern.MatchString(tool.Name) {
			return fmt.Errorf("invalid tool name %q", tool.Name)
		}
		if !makePattern.MatchString(tool.MakePrefix) {
			return fmt.Errorf("invalid Make prefix %q for %s", tool.MakePrefix, tool.Name)
		}
		if _, exists := seenNames[tool.Name]; exists {
			return fmt.Errorf("duplicate tool %q", tool.Name)
		}
		if _, exists := seenPrefixes[tool.MakePrefix]; exists {
			return fmt.Errorf("duplicate Make prefix %q", tool.MakePrefix)
		}
		seenNames[tool.Name] = struct{}{}
		seenPrefixes[tool.MakePrefix] = struct{}{}
		if strings.TrimSpace(tool.Version) == "" {
			return fmt.Errorf("version is required for %s", tool.Name)
		}
		if strings.TrimSpace(tool.Source.Type) == "" || strings.TrimSpace(tool.Source.LatestURL) == "" {
			return fmt.Errorf("source type and latestURL are required for %s", tool.Name)
		}
		if err := validateSource(tool); err != nil {
			return err
		}
		if err := validateChecksumSource(tool); err != nil {
			return err
		}
		if err := validateVersionFiles(tool); err != nil {
			return err
		}

		if tool.ChecksumSource.Type == "none" {
			if len(tool.Platforms) != 0 {
				return fmt.Errorf("tool %s has platforms but no checksum source", tool.Name)
			}
			continue
		}
		if strings.TrimSpace(tool.DownloadTemplate) == "" {
			return fmt.Errorf("downloadTemplate is required for %s", tool.Name)
		}
		if len(tool.Platforms) != len(m.Platforms) {
			return fmt.Errorf("tool %s must define every manifest platform", tool.Name)
		}
		for _, platform := range m.Platforms {
			entry, exists := tool.Platforms[platform]
			if !exists {
				return fmt.Errorf("tool %s is missing platform %s", tool.Name, platform)
			}
			if strings.TrimSpace(entry.Asset) == "" {
				return fmt.Errorf("tool %s has no asset for %s", tool.Name, platform)
			}
			if !hashPattern.MatchString(entry.Checksum) {
				return fmt.Errorf("tool %s has invalid SHA-256 for %s", tool.Name, platform)
			}
			if err := validateToolURLs(tool, platform); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateToolURLs(tool Tool, platform string) error {
	values, err := tool.TemplateValues(platform, tool.Version)
	if err != nil {
		return err
	}
	asset, err := ExpandTemplate(values["asset"], values)
	if err != nil {
		return fmt.Errorf("expand asset for %s %s: %w", tool.Name, platform, err)
	}
	values["asset"] = asset

	if err := validateHTTPSTemplate(tool.Name+" downloadTemplate", tool.DownloadTemplate, values); err != nil {
		return err
	}
	if tool.ChecksumSource.Type == "url-file" {
		if err := validateHTTPSTemplate(tool.Name+" checksumSource.urlTemplate", tool.ChecksumSource.URLTemplate, values); err != nil {
			return err
		}
	}
	return nil
}

func validateHTTPSTemplate(name, template string, values map[string]string) error {
	expanded, err := ExpandTemplate(template, values)
	if err != nil {
		return fmt.Errorf("expand %s: %w", name, err)
	}
	parsedURL, err := url.Parse(expanded)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return fmt.Errorf("%s must expand to a valid HTTPS URL", name)
	}
	return nil
}

func validateSource(tool Tool) error {
	switch tool.Source.Type {
	case "github-release", "stable-text", "hashicorp-checkpoint":
	default:
		return fmt.Errorf("unsupported version source %q for %s", tool.Source.Type, tool.Name)
	}
	if tool.Source.Type == "github-release" && strings.TrimSpace(tool.Source.Repository) == "" {
		return fmt.Errorf("repository is required for GitHub tool %s", tool.Name)
	}

	parsedURL, err := url.Parse(tool.Source.LatestURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return fmt.Errorf("latestURL for %s must be a valid HTTPS URL", tool.Name)
	}
	return nil
}

func validateChecksumSource(tool Tool) error {
	source := tool.ChecksumSource
	switch source.Type {
	case "github-release-file":
		if strings.TrimSpace(tool.Source.Repository) == "" {
			return fmt.Errorf("repository is required for GitHub checksum source %s", tool.Name)
		}
		if source.FileTemplate == "" || source.Format == "" {
			return fmt.Errorf("GitHub checksum file and format are required for %s", tool.Name)
		}
		if !isSupportedChecksumFormat(source.Format, true) {
			return fmt.Errorf("unsupported checksum format %q for %s", source.Format, tool.Name)
		}
		if source.Format == "yq" && source.OrderFileTemplate == "" {
			return fmt.Errorf("yq checksum order file is required for %s", tool.Name)
		}
	case "url-file":
		if source.URLTemplate == "" || source.Format == "" {
			return fmt.Errorf("checksum URL and format are required for %s", tool.Name)
		}
		if !isSupportedChecksumFormat(source.Format, false) {
			return fmt.Errorf("unsupported checksum format %q for %s", source.Format, tool.Name)
		}
	case "download":
	case "none":
		if source.Integrity == "" {
			return fmt.Errorf("integrity description is required for %s", tool.Name)
		}
	default:
		return fmt.Errorf("unsupported checksum source %q for %s", source.Type, tool.Name)
	}
	return nil
}

func isSupportedChecksumFormat(format string, allowYQ bool) bool {
	switch format {
	case "standard", "basename", "first":
		return true
	case "yq":
		return allowYQ
	default:
		return false
	}
}

func validateVersionFiles(tool Tool) error {
	for _, versionFile := range tool.VersionFiles {
		cleanPath := filepath.ToSlash(filepath.Clean(versionFile.Path))
		if cleanPath == "." || cleanPath == ".." || strings.HasPrefix(cleanPath, "../") || filepath.IsAbs(versionFile.Path) {
			return fmt.Errorf("version file path %q for %s must stay within the repository", versionFile.Path, tool.Name)
		}
		switch versionFile.Format {
		case "plain":
			if versionFile.Prefix != "" || versionFile.Suffix != "" {
				return fmt.Errorf("plain version file %q for %s cannot have a prefix or suffix", versionFile.Path, tool.Name)
			}
		case "replace":
			if versionFile.Prefix == "" || versionFile.Suffix == "" {
				return fmt.Errorf("replace version file %q for %s needs a prefix and suffix", versionFile.Path, tool.Name)
			}
		default:
			return fmt.Errorf("unsupported version file format %q for %s", versionFile.Format, tool.Name)
		}
	}
	return nil
}

// UpdatesEnabled reports whether the updater may advance the tool's version.
func (t Tool) UpdatesEnabled() bool {
	return t.Update == nil || *t.Update
}

// Platform returns a platform entry by name.
func (t Tool) Platform(name string) (Platform, bool) {
	entry, ok := t.Platforms[name]
	return entry, ok
}

// TemplateValues builds the values used by asset and URL templates.
func (t Tool) TemplateValues(platform, version string) (map[string]string, error) {
	entry, ok := t.Platform(platform)
	if !ok {
		return nil, fmt.Errorf("tool %s has no platform %s", t.Name, platform)
	}
	parts := strings.SplitN(platform, "_", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid platform %q", platform)
	}
	osName := entry.OS
	if osName == "" {
		osName = parts[0]
	}
	archName := entry.Arch
	if archName == "" {
		archName = parts[1]
	}
	return map[string]string{
		"repository":   t.Source.Repository,
		"tag":          t.TagForVersion(version),
		"version":      version,
		"version_no_v": strings.TrimPrefix(version, "v"),
		"asset":        entry.Asset,
		"os":           osName,
		"arch":         archName,
	}, nil
}

// TagForVersion converts a manifest version into its release tag.
func (t Tool) TagForVersion(version string) string {
	return t.Source.TagPrefix + version
}

// VersionFromTag converts a release tag into the manifest's version format.
func (t Tool) VersionFromTag(tag string) string {
	return strings.TrimPrefix(tag, t.Source.TagPrefix)
}

// ExpandTemplate expands the small, explicit template language used by the
// manifest and rejects unknown variables instead of silently producing a bad URL.
func ExpandTemplate(template string, values map[string]string) (string, error) {
	var result strings.Builder
	for index := 0; index < len(template); {
		start := strings.IndexByte(template[index:], '{')
		if start < 0 {
			result.WriteString(template[index:])
			break
		}
		start += index
		result.WriteString(template[index:start])
		end := strings.IndexByte(template[start+1:], '}')
		if end < 0 {
			return "", fmt.Errorf("unterminated template variable in %q", template)
		}
		end += start + 1
		key := template[start+1 : end]
		value, ok := values[key]
		if !ok {
			return "", fmt.Errorf("unknown template variable %q", key)
		}
		result.WriteString(value)
		index = end + 1
	}
	return result.String(), nil
}

// GenerateMake returns the generated Make include containing only metadata
// values. Tool installation recipes remain in build/tools.mk.
func GenerateMake(m Manifest) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}

	var output bytes.Buffer
	output.WriteString("# Code generated by cmd/tool-updater; DO NOT EDIT.\n\n")
	for _, tool := range m.Tools {
		fmt.Fprintf(&output, "%s_VERSION ?= %s\n", tool.MakePrefix, tool.Version)
		for _, platform := range m.Platforms {
			entry, ok := tool.Platform(platform)
			if !ok || entry.Checksum == "" {
				continue
			}
			name := strings.ToUpper(strings.ReplaceAll(platform, "-", "_"))
			fmt.Fprintf(&output, "%s_CHECKSUM_%s ?= %s\n", tool.MakePrefix, name, entry.Checksum)
		}
		output.WriteByte('\n')
	}
	return output.Bytes(), nil
}

// WriteMakeFile writes generated Make metadata only when its contents change.
func WriteMakeFile(path string, manifest Manifest) (bool, error) {
	contents, err := GenerateMake(manifest)
	if err != nil {
		return false, fmt.Errorf("generate Make metadata: %w", err)
	}
	return writeIfChanged(path, contents)
}

// WriteManifest writes a normalized YAML manifest only when its contents
// change.
func WriteManifest(path string, manifest Manifest) (bool, error) {
	if err := manifest.Validate(); err != nil {
		return false, err
	}
	contents, err := updateManifestYAML(path, manifest)
	if err != nil {
		return false, fmt.Errorf("update tool manifest: %w", err)
	}
	return writeIfChanged(path, contents)
}

func updateManifestYAML(path string, manifest Manifest) ([]byte, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if len(document.Content) != 1 {
		return nil, fmt.Errorf("manifest must contain one YAML document")
	}
	root := document.Content[0]
	toolsNode, err := mappingValue(root, "tools")
	if err != nil {
		return nil, err
	}
	if toolsNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("tools must be a YAML sequence")
	}

	for _, tool := range manifest.Tools {
		toolNode, err := findToolNode(toolsNode, tool.Name)
		if err != nil {
			return nil, err
		}
		versionNode, err := mappingValue(toolNode, "version")
		if err != nil {
			return nil, err
		}
		versionNode.Value = tool.Version

		if tool.ChecksumSource.Type == "none" {
			continue
		}
		platformsNode, err := mappingValue(toolNode, "platforms")
		if err != nil {
			return nil, err
		}
		for platform, entry := range tool.Platforms {
			platformNode, err := mappingValue(platformsNode, platform)
			if err != nil {
				return nil, err
			}
			checksumNode, err := mappingValue(platformNode, "checksum")
			if err != nil {
				return nil, err
			}
			checksumNode.Value = entry.Checksum
		}
	}

	updated, err := yaml.Marshal(&document)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	return updated, nil
}

func findToolNode(toolsNode *yaml.Node, name string) (*yaml.Node, error) {
	for _, toolNode := range toolsNode.Content {
		if toolNode.Kind != yaml.MappingNode {
			continue
		}
		nameNode, err := mappingValue(toolNode, "name")
		if err != nil {
			return nil, err
		}
		if nameNode.Value == name {
			return toolNode, nil
		}
	}
	return nil, fmt.Errorf("tool %s is missing from the manifest YAML", name)
}

func mappingValue(node *yaml.Node, key string) (*yaml.Node, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected YAML mapping while looking for %s", key)
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1], nil
		}
	}
	return nil, fmt.Errorf("YAML key %s is missing", key)
}

// WriteTextFile writes a text file only when its contents change.
func WriteTextFile(path, contents string) (bool, error) {
	return writeIfChanged(path, []byte(contents))
}

// SyncVersionFiles updates the declared consumers of a tool version. Plain
// files are replaced completely; replace files update the value between their
// declared prefix and suffix exactly once.
func SyncVersionFiles(root string, tool Tool) error {
	for _, versionFile := range tool.VersionFiles {
		filePath := filepath.Join(root, filepath.FromSlash(versionFile.Path))
		contents, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read version consumer %s: %w", versionFile.Path, err)
		}

		var updated string
		text := string(contents)
		switch versionFile.Format {
		case "plain":
			updated = tool.Version + "\n"
		case "replace":
			if strings.Count(text, versionFile.Prefix) != 1 {
				return fmt.Errorf("version prefix for %s must occur exactly once in %s", tool.Name, versionFile.Path)
			}
			prefixStart := strings.Index(text, versionFile.Prefix)
			valueStart := prefixStart + len(versionFile.Prefix)
			suffixOffset := strings.Index(text[valueStart:], versionFile.Suffix)
			if suffixOffset < 0 {
				return fmt.Errorf("version suffix for %s was not found in %s", tool.Name, versionFile.Path)
			}
			valueEnd := valueStart + suffixOffset
			updated = text[:valueStart] + tool.Version + text[valueEnd:]
		default:
			return fmt.Errorf("unsupported version file format %q", versionFile.Format)
		}
		if _, err := WriteTextFile(filePath, updated); err != nil {
			return fmt.Errorf("write version consumer %s: %w", versionFile.Path, err)
		}
	}
	return nil
}

func writeIfChanged(path string, contents []byte) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, contents) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create directory for %s: %w", path, err)
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), ".tooling-*")
	if err != nil {
		return false, fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return false, fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return false, fmt.Errorf("replace %s: %w", path, err)
	}
	return true, nil
}

func replaceFile(source, destination string) error {
	err := os.Rename(source, destination)
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrExist) {
		return err
	}
	if removeErr := os.Remove(destination); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
		return fmt.Errorf("remove existing destination: %w", removeErr)
	}
	return os.Rename(source, destination)
}
