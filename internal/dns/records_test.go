package dns

import (
	"testing"
)

func newStoreWithRecords(records map[string]string) *RecordStore {
	s := &RecordStore{
		byFile: make(map[string]RecordFile),
		merged: make(map[string]string),
		empty:  make(chan struct{}),
	}
	for k, v := range records {
		s.merged[k] = v
	}
	return s
}

func TestLookup_ExactMatch(t *testing.T) {
	s := newStoreWithRecords(map[string]string{
		"api.gck.local": "10.0.0.1",
	})
	ip, ok := s.Lookup("api.gck.local")
	if !ok || ip != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %q (ok=%v)", ip, ok)
	}
}

func TestLookup_ExactMatchCaseInsensitive(t *testing.T) {
	s := newStoreWithRecords(map[string]string{
		"api.gck.local": "10.0.0.1",
	})
	ip, ok := s.Lookup("API.GCK.LOCAL")
	if !ok || ip != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %q (ok=%v)", ip, ok)
	}
}

func TestLookup_WildcardMatchSingleLabel(t *testing.T) {
	s := newStoreWithRecords(map[string]string{
		"*.kafka.gck.local": "10.0.0.2",
	})
	ip, ok := s.Lookup("demo.kafka.gck.local")
	if !ok || ip != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.2, got %q (ok=%v)", ip, ok)
	}
}

func TestLookup_WildcardMatchHyphenatedLabel(t *testing.T) {
	s := newStoreWithRecords(map[string]string{
		"*.kafka.gck.local": "10.0.0.2",
	})
	ip, ok := s.Lookup("broker-0-demo.kafka.gck.local")
	if !ok || ip != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.2, got %q (ok=%v)", ip, ok)
	}
}

func TestLookup_ExactTakesPriorityOverWildcard(t *testing.T) {
	s := newStoreWithRecords(map[string]string{
		"*.kafka.gck.local":   "10.0.0.2",
		"demo.kafka.gck.local": "10.0.0.3",
	})
	ip, ok := s.Lookup("demo.kafka.gck.local")
	if !ok || ip != "10.0.0.3" {
		t.Fatalf("expected exact match 10.0.0.3, got %q (ok=%v)", ip, ok)
	}
}

func TestLookup_NoMatch(t *testing.T) {
	s := newStoreWithRecords(map[string]string{
		"*.kafka.gck.local": "10.0.0.2",
	})
	_, ok := s.Lookup("api.gck.local")
	if ok {
		t.Fatal("expected no match for api.gck.local against *.kafka.gck.local")
	}
}

func TestLookup_NoMatchBareHostname(t *testing.T) {
	s := newStoreWithRecords(map[string]string{
		"*.kafka.gck.local": "10.0.0.2",
	})
	_, ok := s.Lookup("kafka.gck.local")
	if ok {
		t.Fatal("expected no match for kafka.gck.local (wildcard requires a label before the pattern)")
	}
}

func TestLookup_EmptyStore(t *testing.T) {
	s := newStoreWithRecords(map[string]string{})
	_, ok := s.Lookup("anything.gck.local")
	if ok {
		t.Fatal("expected no match in empty store")
	}
}

func TestCheckEmpty_NeverHadRecords(t *testing.T) {
	s := NewRecordStore(t.TempDir())
	_ = s.Load()
	select {
	case <-s.Empty():
		t.Fatal("empty channel should not be closed when store never had records")
	default:
	}
}

func TestCheckEmpty_RecordsRemovedAfterExisting(t *testing.T) {
	s := NewRecordStore(t.TempDir())
	s.mu.Lock()
	s.byFile["test.json"] = RecordFile{Records: map[string]string{"a.gck.local": "1.2.3.4"}}
	s.checkEmpty()
	s.mu.Unlock()

	if s.hadRecords != true {
		t.Fatal("hadRecords should be true after adding records")
	}

	s.mu.Lock()
	delete(s.byFile, "test.json")
	s.checkEmpty()
	s.mu.Unlock()

	select {
	case <-s.Empty():
	default:
		t.Fatal("empty channel should be closed after all records are removed")
	}
}
