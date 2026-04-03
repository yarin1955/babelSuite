package profiles

import (
	"errors"
	"strings"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func NewService(base suiteReader, store Store) *Service {
	if base == nil {
		base = suites.NewService()
	}
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{
		base:  base,
		store: store,
	}
}

func (s *Service) List() []suites.Definition {
	definitions := s.base.List()
	document := s.safeLoadDocument()

	for index := range definitions {
		records := s.mergeSuiteProfiles(definitions[index], document)
		definitions[index].Profiles = toLaunchOptions(records)
	}

	return definitions
}

func (s *Service) Get(id string) (*suites.Definition, error) {
	definition, err := s.base.Get(id)
	if err != nil {
		return nil, err
	}

	document := s.safeLoadDocument()
	definition.Profiles = toLaunchOptions(s.mergeSuiteProfiles(*definition, document))
	return definition, nil
}

func (s *Service) ListSuiteSummaries() ([]SuiteSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	definitions := s.base.List()
	summaries := make([]SuiteSummary, 0, len(definitions))
	for _, definition := range definitions {
		records := s.mergeSuiteProfiles(definition, document)
		_, defaultProfileFileName := defaultProfile(records)
		summaries = append(summaries, SuiteSummary{
			ID:                     definition.ID,
			Title:                  definition.Title,
			Description:            definition.Description,
			Repository:             definition.Repository,
			ProfileCount:           len(records),
			LaunchableCount:        countLaunchable(records),
			DefaultProfileFileName: defaultProfileFileName,
		})
	}

	return summaries, nil
}

func (s *Service) GetSuiteProfiles(suiteID string) (*SuiteProfiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definition, err := s.lookupSuite(suiteID)
	if err != nil {
		return nil, err
	}

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	records := s.mergeSuiteProfiles(*definition, document)
	return buildSuiteProfiles(*definition, records), nil
}

func (s *Service) CreateProfile(suiteID string, request UpsertRequest) (*SuiteProfiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definition, err := s.lookupSuite(suiteID)
	if err != nil {
		return nil, err
	}

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	records := s.mergeSuiteProfiles(*definition, document)
	record := Record{
		ID:         uniqueProfileID(records, request.FileName),
		Launchable: true,
	}
	if err := applyUpsertRequest(&record, request, records, ""); err != nil {
		return nil, err
	}

	records = append(records, record)
	normalizeRecords(records)
	writeSuiteProfiles(document, definition.ID, records)
	if err := s.store.Save(document); err != nil {
		return nil, err
	}

	return buildSuiteProfiles(*definition, records), nil
}

func (s *Service) UpdateProfile(suiteID, profileID string, request UpsertRequest) (*SuiteProfiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definition, err := s.lookupSuite(suiteID)
	if err != nil {
		return nil, err
	}

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	records := s.mergeSuiteProfiles(*definition, document)
	index := indexOfProfile(records, profileID)
	if index == -1 {
		return nil, ErrProfileNotFound
	}

	record := records[index]
	if err := applyUpsertRequest(&record, request, records, record.ID); err != nil {
		return nil, err
	}
	if !record.Launchable {
		record.Default = false
	}
	records[index] = record
	normalizeRecords(records)
	writeSuiteProfiles(document, definition.ID, records)
	if err := s.store.Save(document); err != nil {
		return nil, err
	}

	return buildSuiteProfiles(*definition, records), nil
}

func (s *Service) DeleteProfile(suiteID, profileID string) (*SuiteProfiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definition, err := s.lookupSuite(suiteID)
	if err != nil {
		return nil, err
	}

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	records := s.mergeSuiteProfiles(*definition, document)
	index := indexOfProfile(records, profileID)
	if index == -1 {
		return nil, ErrProfileNotFound
	}
	if !records[index].Launchable {
		return nil, errors.New("Base profiles cannot be deleted.")
	}
	if countLaunchable(records) <= 1 {
		return nil, errors.New("At least one launchable profile must remain.")
	}

	records = append(records[:index], records[index+1:]...)
	normalizeRecords(records)
	writeSuiteProfiles(document, definition.ID, records)
	if err := s.store.Save(document); err != nil {
		return nil, err
	}

	return buildSuiteProfiles(*definition, records), nil
}

func (s *Service) SetDefaultProfile(suiteID, profileID string) (*SuiteProfiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definition, err := s.lookupSuite(suiteID)
	if err != nil {
		return nil, err
	}

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	records := s.mergeSuiteProfiles(*definition, document)
	index := indexOfProfile(records, profileID)
	if index == -1 {
		return nil, ErrProfileNotFound
	}
	if !records[index].Launchable {
		return nil, errors.New("Base profiles cannot be used as the default launch profile.")
	}

	for recordIndex := range records {
		records[recordIndex].Default = records[recordIndex].ID == profileID && records[recordIndex].Launchable
	}
	normalizeRecords(records)
	writeSuiteProfiles(document, definition.ID, records)
	if err := s.store.Save(document); err != nil {
		return nil, err
	}

	return buildSuiteProfiles(*definition, records), nil
}

func (s *Service) lookupSuite(suiteID string) (*suites.Definition, error) {
	definition, err := s.base.Get(strings.TrimSpace(suiteID))
	if err != nil {
		if errors.Is(err, suites.ErrNotFound) {
			return nil, ErrSuiteNotFound
		}
		return nil, err
	}
	return definition, nil
}

func (s *Service) safeLoadDocument() *Document {
	s.mu.Lock()
	defer s.mu.Unlock()

	document, err := s.store.Load()
	if err != nil {
		fallback := defaultDocument()
		return &fallback
	}
	return document
}
