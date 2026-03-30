package support

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/babelsuite/babelsuite/pkg/apiclient"
)

func SplitReference(input string) (repository string, version string) {
	value := strings.TrimSpace(input)
	lastSlash := strings.LastIndex(value, "/")
	lastColon := strings.LastIndex(value, ":")
	if lastColon > lastSlash {
		return value[:lastColon], value[lastColon+1:]
	}
	return value, ""
}

func RepositoryPath(repository string) string {
	repository = strings.TrimSpace(repository)
	if repository == "" {
		return ""
	}
	if slash := strings.Index(repository, "/"); slash >= 0 {
		host := repository[:slash]
		if strings.Contains(host, ".") || strings.Contains(host, ":") || strings.EqualFold(host, "localhost") {
			return strings.Trim(repository[slash+1:], "/")
		}
	}
	return strings.Trim(repository, "/")
}

func LastRepositorySegment(repository string) string {
	path := RepositoryPath(repository)
	if path == "" {
		path = strings.Trim(repository, "/")
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func ResolveLaunchTarget(target string, suites []apiclient.LaunchSuite) (*apiclient.LaunchSuite, error) {
	base, _ := SplitReference(target)
	matches := make([]apiclient.LaunchSuite, 0, 1)
	for _, suite := range suites {
		switch {
		case normalizeMatch(suite.ID) == normalizeMatch(target), normalizeMatch(suite.ID) == normalizeMatch(base):
			clone := suite
			return &clone, nil
		case normalizeMatch(suite.Repository) == normalizeMatch(target), normalizeMatch(suite.Repository) == normalizeMatch(base):
			clone := suite
			return &clone, nil
		case normalizeMatch(RepositoryPath(suite.Repository)) == normalizeMatch(base):
			matches = append(matches, suite)
		}
	}
	if len(matches) == 1 {
		clone := matches[0]
		return &clone, nil
	}
	return nil, fmt.Errorf("could not resolve runnable suite %q", target)
}

func ResolveCatalogTarget(target string, packages []apiclient.CatalogPackage) (*apiclient.CatalogPackage, error) {
	base, _ := SplitReference(target)
	matches := make([]apiclient.CatalogPackage, 0, 1)
	for _, item := range packages {
		switch {
		case normalizeMatch(item.ID) == normalizeMatch(target), normalizeMatch(item.ID) == normalizeMatch(base):
			clone := item
			return &clone, nil
		case normalizeMatch(item.Repository) == normalizeMatch(target), normalizeMatch(item.Repository) == normalizeMatch(base):
			clone := item
			return &clone, nil
		case normalizeMatch(RepositoryPath(item.Repository)) == normalizeMatch(base):
			matches = append(matches, item)
		}
	}
	if len(matches) == 1 {
		clone := matches[0]
		return &clone, nil
	}
	return nil, fmt.Errorf("could not resolve catalog package %q", target)
}

func ResolveSuiteTarget(target string, suites []apiclient.SuiteDefinition) (*apiclient.SuiteDefinition, error) {
	base, _ := SplitReference(target)
	matches := make([]apiclient.SuiteDefinition, 0, 1)
	for _, suite := range suites {
		switch {
		case normalizeMatch(suite.ID) == normalizeMatch(target), normalizeMatch(suite.ID) == normalizeMatch(base):
			clone := suite
			return &clone, nil
		case normalizeMatch(suite.Repository) == normalizeMatch(target), normalizeMatch(suite.Repository) == normalizeMatch(base):
			clone := suite
			return &clone, nil
		case normalizeMatch(RepositoryPath(suite.Repository)) == normalizeMatch(base):
			matches = append(matches, suite)
		}
	}
	if len(matches) == 1 {
		clone := matches[0]
		return &clone, nil
	}
	return nil, fmt.Errorf("could not resolve suite %q", target)
}

func DefaultForkDestination(target string, suite *apiclient.SuiteDefinition) string {
	base, _ := SplitReference(target)
	name := strings.TrimSpace(LastRepositorySegment(base))
	if name != "" {
		return name
	}
	if suite != nil {
		return suite.ID
	}
	return "suite-copy"
}

func WriteSuiteFiles(root string, files []apiclient.SuiteSourceFile, force bool) (int, error) {
	if len(files) == 0 {
		return 0, errors.New("suite does not expose source files")
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(absoluteRoot, 0o755); err != nil {
		return 0, err
	}

	written := 0
	rootPrefix := strings.ToLower(absoluteRoot + string(os.PathSeparator))

	for _, file := range files {
		relative := filepath.Clean(filepath.FromSlash(strings.TrimSpace(file.Path)))
		if relative == "." || strings.HasPrefix(relative, "..") {
			return written, fmt.Errorf("refusing to write unsafe path %q", file.Path)
		}

		targetPath := filepath.Join(absoluteRoot, relative)
		absoluteTarget, err := filepath.Abs(targetPath)
		if err != nil {
			return written, err
		}
		targetLower := strings.ToLower(absoluteTarget)
		if targetLower != strings.ToLower(absoluteRoot) && !strings.HasPrefix(targetLower, rootPrefix) {
			return written, fmt.Errorf("refusing to write outside %s", absoluteRoot)
		}

		if !force {
			if _, err := os.Stat(absoluteTarget); err == nil {
				return written, fmt.Errorf("file already exists: %s", absoluteTarget)
			}
		}

		if err := os.MkdirAll(filepath.Dir(absoluteTarget), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(absoluteTarget, []byte(file.Content), 0o644); err != nil {
			return written, err
		}
		written++
	}

	return written, nil
}

func normalizeMatch(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
