package suites

import "testing"

func TestValidateDefinitionRejectsBlockedRootFile(t *testing.T) {
	suite := validTestSuiteDefinition()
	suite.SeedSources = append(suite.SeedSources, SourceFile{
		Path:    "payload.exe",
		Content: "not-a-real-binary",
	})
	suite.SourceFiles = buildSourceFiles(suite, nil)

	err := ValidateDefinition(suite)
	if err == nil || err.Error() != `source path "payload.exe" uses a blocked file type` {
		t.Fatalf("expected blocked root file error, got %v", err)
	}
}

func TestValidateDefinitionRejectsUnexpectedRootFile(t *testing.T) {
	suite := validTestSuiteDefinition()
	suite.SeedSources = append(suite.SeedSources, SourceFile{
		Path:    "payload.txt",
		Content: "unexpected",
	})
	suite.SourceFiles = buildSourceFiles(suite, nil)

	err := ValidateDefinition(suite)
	if err == nil || err.Error() != `root file "payload.txt" is not allowed in a suite package` {
		t.Fatalf("expected unexpected root file error, got %v", err)
	}
}

func TestValidateDefinitionRejectsPathTraversal(t *testing.T) {
	suite := validTestSuiteDefinition()
	suite.Folders = append(suite.Folders, FolderEntry{
		Name:  "scripts",
		Files: []string{"../escape.sh"},
	})
	suite.SourceFiles = buildSourceFiles(suite, nil)

	err := ValidateDefinition(suite)
	if err == nil || err.Error() != `source path "scripts/../escape.sh" must be normalized` {
		t.Fatalf("expected path normalization error, got %v", err)
	}
}

func TestValidateDefinitionRejectsHiddenSegments(t *testing.T) {
	suite := validTestSuiteDefinition()
	suite.Folders = append(suite.Folders, FolderEntry{
		Name:  "scripts",
		Files: []string{".hidden/bootstrap.sh"},
	})
	suite.SourceFiles = buildSourceFiles(suite, nil)

	err := ValidateDefinition(suite)
	if err == nil || err.Error() != `source path "scripts/.hidden/bootstrap.sh" contains a hidden segment` {
		t.Fatalf("expected hidden segment error, got %v", err)
	}
}

func TestValidateDefinitionRejectsBinaryContent(t *testing.T) {
	suite := validTestSuiteDefinition()
	suite.SourceFiles = append(suite.SourceFiles, SourceFile{
		Path:    "scripts/bootstrap.sh",
		Content: "echo hello\x00world",
	})

	err := ValidateDefinition(suite)
	if err == nil || err.Error() != `source file "scripts/bootstrap.sh" appears to contain binary content` {
		t.Fatalf("expected binary content error, got %v", err)
	}
}

func TestHydrateSuitesSkipsInvalidDefinitions(t *testing.T) {
	valid := validTestSuiteDefinition()
	invalid := validTestSuiteDefinition()
	invalid.ID = "bad-suite"
	invalid.Title = "Bad Suite"
	invalid.SeedSources = append(invalid.SeedSources, SourceFile{
		Path:    "payload.txt",
		Content: "definitely-not-safe",
	})

	result := hydrateSuites(map[string]Definition{
		valid.ID:   valid,
		invalid.ID: invalid,
	})

	if len(result) != 1 {
		t.Fatalf("expected 1 hydrated suite, got %d", len(result))
	}
	if _, ok := result[valid.ID]; !ok {
		t.Fatalf("expected %q to remain available", valid.ID)
	}
	if _, ok := result[invalid.ID]; ok {
		t.Fatalf("did not expect %q to survive validation", invalid.ID)
	}
}

func validTestSuiteDefinition() Definition {
	return Definition{
		ID:          "safe-suite",
		Title:       "Safe Suite",
		Repository:  "localhost:5000/testing/safe-suite",
		Provider:    "Workspace",
		Version:     "workspace",
		SuiteStar:   "api = container.run(name=\"api\")\n",
		SeedSources: []SourceFile{{Path: "README.md", Content: "safe suite"}},
		Folders: []FolderEntry{
			{Name: "profiles", Files: []string{"local.yaml"}},
			{Name: "scripts", Files: []string{"bootstrap.sh"}},
		},
	}
}
