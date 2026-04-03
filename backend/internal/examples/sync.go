package examples

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/internal/catalog"
	"github.com/babelsuite/babelsuite/internal/examplefs"
	"github.com/babelsuite/babelsuite/internal/examplegen"
	"github.com/babelsuite/babelsuite/internal/suites"
)

type RenderedFile struct {
	Path    string
	Content string
}

func RenderWorkspaceFiles() []RenderedFile {
	files := make([]RenderedFile, 0)

	service := suites.NewService()
	for _, suite := range service.List() {
		base := joinPath("oci-suites", suite.ID)
		files = append(files,
			RenderedFile{Path: joinPath(base, "README.md"), Content: renderSuiteReadme(suite)},
			RenderedFile{Path: joinPath(base, "suite.star"), Content: ensureTrailingNewline(suite.SuiteStar)},
		)
		for _, file := range examplegen.GeneratedSourceFiles(suite) {
			if !shouldWriteExampleSourceFile(file.Path) {
				continue
			}
			files = append(files, RenderedFile{
				Path:    joinPath(base, file.Path),
				Content: ensureTrailingNewline(file.Content),
			})
		}
	}

	for _, module := range catalog.SeedStdlibPackages() {
		base := joinPath("oci-modules", moduleDirectoryName(module))
		files = append(files,
			RenderedFile{Path: joinPath(base, "README.md"), Content: renderModuleReadme(module)},
			RenderedFile{Path: joinPath(base, "module.yaml"), Content: renderModuleMetadata(module)},
			RenderedFile{Path: joinPath(base, "usage.star"), Content: renderModuleUsage(module)},
		)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

func SyncWorkspace(repoRoot string) (int, error) {
	files := RenderWorkspaceFiles()
	examplesRoot := examplefs.ResolveRootFromRepo(repoRoot)
	if err := cleanupGeneratedSuiteArtifacts(examplesRoot); err != nil {
		return 0, err
	}
	for _, file := range files {
		target := filepath.Join(examplesRoot, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o644); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

func renderSuiteReadme(suite suites.Definition) string {
	lines := []string{
		suite.Title,
		"",
		suite.Description,
		"",
		"Structure",
		"",
		"- `suite.star`: declarative topology",
	}
	for _, folder := range suite.Folders {
		if !shouldDescribeExampleFolder(folder.Name) {
			continue
		}
		lines = append(lines, fmt.Sprintf("- `%s/`: %s", folder.Name, folder.Description))
	}
	return strings.Join(lines, "\n") + "\n"
}

func shouldWriteExampleSourceFile(path string) bool {
	normalized := filepath.ToSlash(strings.Trim(strings.TrimSpace(path), "/"))
	return !strings.HasPrefix(normalized, "gateway/")
}

func shouldDescribeExampleFolder(name string) bool {
	return strings.TrimSpace(name) != "gateway"
}

func cleanupGeneratedSuiteArtifacts(examplesRoot string) error {
	matches, err := filepath.Glob(filepath.Join(examplesRoot, "oci-suites", "*", "gateway"))
	if err != nil {
		return err
	}
	for _, match := range matches {
		if err := os.RemoveAll(match); err != nil {
			return err
		}
	}
	return nil
}

func renderModuleReadme(module catalog.Package) string {
	lines := []string{
		module.Title,
		"",
		module.Description,
		"",
		"Details",
		"",
		fmt.Sprintf("- Repository: `%s`", module.Repository),
		fmt.Sprintf("- Version: `%s`", module.Version),
		fmt.Sprintf("- Tags: `%s`", strings.Join(module.Tags, "`, `")),
		fmt.Sprintf("- Pull: `%s`", module.PullCommand),
		fmt.Sprintf("- Fork: `%s`", module.ForkCommand),
		"",
		"Usage",
		"",
		"See `usage.star` for a minimal Starlark import example.",
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderModuleMetadata(module catalog.Package) string {
	lines := []string{
		"kind: OCIExampleModule",
		"metadata:",
		fmt.Sprintf("  id: %s", module.ID),
		fmt.Sprintf("  title: %s", module.Title),
		"spec:",
		fmt.Sprintf("  repository: %s", module.Repository),
		fmt.Sprintf("  provider: %s", module.Provider),
		fmt.Sprintf("  version: %s", module.Version),
		"  tags:",
	}
	for _, tag := range module.Tags {
		lines = append(lines, fmt.Sprintf("    - %s", tag))
	}
	lines = append(lines,
		fmt.Sprintf("  description: %s", module.Description),
		fmt.Sprintf("  pullCommand: %s", module.PullCommand),
		fmt.Sprintf("  forkCommand: %s", module.ForkCommand),
	)
	return strings.Join(lines, "\n") + "\n"
}

func renderModuleUsage(module catalog.Package) string {
	loadSymbol := moduleLoadSymbol(module)
	name := moduleDirectoryName(module)

	lines := []string{
		fmt.Sprintf("load(%q, %q)", module.Title, loadSymbol),
		"",
		moduleInvocation(module, loadSymbol, name),
	}
	return strings.Join(lines, "\n") + "\n"
}

func moduleLoadSymbol(module catalog.Package) string {
	switch moduleDirectoryName(module) {
	case "postgres":
		return "pg"
	default:
		return moduleDirectoryName(module)
	}
}

func moduleInvocation(module catalog.Package, loadSymbol, name string) string {
	switch moduleDirectoryName(module) {
	case "postgres":
		return `db = pg(name="db")`
	case "kafka":
		return `broker = kafka(name="kafka")`
	default:
		return fmt.Sprintf(`%s_instance = %s(name=%q)`, strings.ReplaceAll(name, "-", "_"), loadSymbol, name)
	}
}

func moduleDirectoryName(module catalog.Package) string {
	repository := strings.TrimSpace(module.Repository)
	if repository == "" {
		return strings.TrimPrefix(strings.TrimSpace(module.Title), "@babelsuite/")
	}
	parts := strings.Split(strings.Trim(repository, "/"), "/")
	return parts[len(parts)-1]
}

func ensureTrailingNewline(content string) string {
	if strings.HasSuffix(content, "\n") {
		return content
	}
	return content + "\n"
}

func joinPath(parts ...string) string {
	return filepath.ToSlash(filepath.Join(parts...))
}
