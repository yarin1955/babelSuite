package profiles

import (
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func (s *Service) mergeSuiteProfiles(definition suites.Definition, document *Document) []Record {
	seed := seedSuiteProfiles(definition)
	stored := []Record{}
	if document != nil && document.Suites != nil {
		stored = append(stored, document.Suites[definition.ID].Profiles...)
	}

	if len(stored) == 0 {
		return cloneRecords(seed)
	}

	merged := make([]Record, 0, len(seed)+len(stored))
	usedStored := make(map[int]struct{}, len(stored))

	for _, seedRecord := range seed {
		matched := false
		for storedIndex, storedRecord := range stored {
			if _, alreadyUsed := usedStored[storedIndex]; alreadyUsed {
				continue
			}
			if storedRecord.ID == seedRecord.ID || (storedRecord.ID == "" && strings.EqualFold(storedRecord.FileName, seedRecord.FileName)) {
				record := storedRecord
				record.ID = firstNonEmpty(record.ID, seedRecord.ID)
				record.Launchable = seedRecord.Launchable
				record.UpdatedAt = nonZeroTime(record.UpdatedAt, seedRecord.UpdatedAt)
				merged = append(merged, normalizeRecord(record))
				usedStored[storedIndex] = struct{}{}
				matched = true
				break
			}
		}
		if !matched {
			merged = append(merged, normalizeRecord(seedRecord))
		}
	}

	for storedIndex, storedRecord := range stored {
		if _, alreadyUsed := usedStored[storedIndex]; alreadyUsed {
			continue
		}
		merged = append(merged, normalizeRecord(storedRecord))
	}

	normalizeRecords(merged)
	return merged
}

func buildSuiteProfiles(definition suites.Definition, records []Record) *SuiteProfiles {
	cloned := cloneRecords(records)
	defaultProfileID, defaultProfileFileName := defaultProfile(cloned)
	return &SuiteProfiles{
		SuiteID:                definition.ID,
		SuiteTitle:             definition.Title,
		SuiteDescription:       definition.Description,
		Repository:             definition.Repository,
		DefaultProfileID:       defaultProfileID,
		DefaultProfileFileName: defaultProfileFileName,
		Profiles:               cloned,
	}
}

func writeSuiteProfiles(document *Document, suiteID string, records []Record) {
	if document.Suites == nil {
		document.Suites = map[string]SuiteDocument{}
	}
	document.Suites[suiteID] = SuiteDocument{
		Profiles: cloneRecords(records),
	}
}

func normalizeDocument(document *Document) {
	if document.Suites == nil {
		document.Suites = map[string]SuiteDocument{}
	}
	for suiteID, suiteDocument := range document.Suites {
		normalizeRecords(suiteDocument.Profiles)
		document.Suites[strings.TrimSpace(suiteID)] = SuiteDocument{
			Profiles: suiteDocument.Profiles,
		}
	}
}

func normalizeRecords(records []Record) {
	defaultSeen := false
	for index := range records {
		records[index] = normalizeRecord(records[index])
		if !records[index].Launchable {
			records[index].Default = false
			continue
		}
		if records[index].Default && !defaultSeen {
			defaultSeen = true
			continue
		}
		if records[index].Default && defaultSeen {
			records[index].Default = false
		}
	}

	if defaultSeen {
		return
	}

	for index := range records {
		if records[index].Launchable {
			records[index].Default = true
			return
		}
	}
}

func normalizeRecord(record Record) Record {
	record.ID = strings.TrimSpace(record.ID)
	record.Name = strings.TrimSpace(record.Name)
	record.FileName = strings.TrimSpace(record.FileName)
	record.Description = strings.TrimSpace(record.Description)
	record.Scope = firstNonEmpty(strings.TrimSpace(record.Scope), scopeFromFileName(record.FileName))
	record.YAML = strings.TrimSpace(record.YAML)
	record.ExtendsID = strings.TrimSpace(record.ExtendsID)
	record.SecretRefs = compactSecretRefs(record.SecretRefs)
	record.UpdatedAt = nonZeroTime(record.UpdatedAt, time.Now().UTC())
	return record
}

func toLaunchOptions(records []Record) []suites.ProfileOption {
	options := make([]suites.ProfileOption, 0, len(records))
	for _, record := range records {
		if !record.Launchable {
			continue
		}
		options = append(options, suites.ProfileOption{
			FileName:    record.FileName,
			Label:       record.Name,
			Description: record.Description,
			Default:     record.Default,
		})
	}
	return options
}

func cloneDocument(document Document) Document {
	clone := defaultDocument()
	for suiteID, suiteDocument := range document.Suites {
		clone.Suites[suiteID] = SuiteDocument{
			Profiles: cloneRecords(suiteDocument.Profiles),
		}
	}
	return clone
}

func cloneRecords(records []Record) []Record {
	clone := make([]Record, len(records))
	for index, record := range records {
		clone[index] = record
		clone[index].SecretRefs = append([]SecretReference{}, record.SecretRefs...)
	}
	return clone
}

func compactSecretRefs(secretRefs []SecretReference) []SecretReference {
	result := make([]SecretReference, 0, len(secretRefs))
	for _, secretRef := range secretRefs {
		key := strings.TrimSpace(secretRef.Key)
		provider := strings.TrimSpace(secretRef.Provider)
		ref := strings.TrimSpace(secretRef.Ref)
		if key == "" && provider == "" && ref == "" {
			continue
		}
		result = append(result, SecretReference{
			Key:      key,
			Provider: provider,
			Ref:      ref,
		})
	}
	return result
}

func defaultDocument() Document {
	return Document{
		Suites: map[string]SuiteDocument{},
	}
}

func defaultProfile(records []Record) (string, string) {
	for _, record := range records {
		if record.Launchable && record.Default {
			return record.ID, record.FileName
		}
	}
	return "", ""
}

func countLaunchable(records []Record) int {
	count := 0
	for _, record := range records {
		if record.Launchable {
			count++
		}
	}
	return count
}

func nonZeroTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}
