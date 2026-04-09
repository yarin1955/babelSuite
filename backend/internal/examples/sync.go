package examples

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	examplesRoot := examplefs.ResolveRoot()

	service := suites.NewService()
	for _, suite := range service.List() {
		base := joinPath("oci-suites", suite.ID)
		readmePath := joinPath(base, "README.md")
		readmeContent := renderSuiteReadme(suite)
		if content, ok := workspaceFileContent(examplesRoot, readmePath); ok {
			readmeContent = ensureTrailingNewline(content)
		}
		suiteStarPath := joinPath(base, "suite.star")
		suiteStarContent := ensureTrailingNewline(suite.SuiteStar)
		if content, ok := workspaceFileContent(examplesRoot, suiteStarPath); ok {
			suiteStarContent = ensureTrailingNewline(content)
		}
		files = append(files,
			RenderedFile{Path: readmePath, Content: readmeContent},
			RenderedFile{Path: suiteStarPath, Content: suiteStarContent},
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
	for _, path := range []string{
		filepath.Join(examplesRoot, "oci-modules", "runtime"),
		filepath.Join(examplesRoot, "runtime"),
	} {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}

	matches, err := filepath.Glob(filepath.Join(examplesRoot, "oci-suites", "*", "gateway"))
	if err != nil {
		return err
	}
	for _, match := range matches {
		if err := os.RemoveAll(match); err != nil {
			return err
		}
	}

	suitesRoot := filepath.Join(examplesRoot, "oci-suites")
	if err := filepath.WalkDir(suitesRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		segments := strings.Split(filepath.ToSlash(path), "/")
		for _, segment := range segments {
			if segment != "mock" {
				continue
			}
			return os.Remove(path)
		}
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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

func workspaceFileContent(examplesRoot, relativePath string) (string, bool) {
	if strings.TrimSpace(examplesRoot) == "" {
		return "", false
	}

	body, err := os.ReadFile(filepath.Join(examplesRoot, filepath.FromSlash(relativePath)))
	if err != nil {
		return "", false
	}
	return string(body), true
}
