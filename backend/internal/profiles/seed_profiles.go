package profiles

import (
	"os"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/demofs"
	"github.com/babelsuite/babelsuite/internal/examplefs"
	"github.com/babelsuite/babelsuite/internal/suites"
)

func seedSuiteProfiles(definition suites.Definition) []Record {
	if !demofs.Enabled() {
		return workspaceSuiteProfiles(definition)
	}

	document, err := loadSeedProfilesDocument()
	if err != nil || document.Suites == nil {
		return nil
	}

	records := document.Suites[strings.TrimSpace(definition.ID)].Profiles
	return cloneRecords(records)
}

func workspaceSuiteProfiles(definition suites.Definition) []Record {
	records := make([]Record, 0, len(definition.Profiles))
	for _, profile := range definition.Profiles {
		content := readWorkspaceProfileYAML(definition.ID, profile.FileName)
		records = append(records, Record{
			ID:          profileIDFromFileName(profile.FileName),
			Name:        firstNonEmpty(strings.TrimSpace(profile.Label), labelFromFileName(profile.FileName)),
			FileName:    profile.FileName,
			Description: strings.TrimSpace(profile.Description),
			Scope:       scopeFromFileName(profile.FileName),
			YAML:        content,
			Default:     profile.Default,
			Launchable:  true,
			UpdatedAt:   time.Now().UTC(),
		})
	}
	normalizeRecords(records)
	return records
}

func loadSeedProfilesDocument() (Document, error) {
	var document Document

	manifest, err := demofs.LoadManifest()
	if err != nil {
		return document, err
	}

	document, err = demofs.LoadJSON[Document](manifest.ProfilesFile)
	if err != nil {
		return Document{}, err
	}

	normalizeDocument(&document)
	return document, nil
}

func readWorkspaceProfileYAML(suiteID, fileName string) string {
	path := examplefs.SuiteFilePath(suiteID, "profiles/"+strings.TrimSpace(fileName))
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
