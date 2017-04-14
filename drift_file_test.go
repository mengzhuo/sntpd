package sntpd

import (
	"io/ioutil"
	"os"
	"testing"
)

const (
	TestDriftFilename = ".test_sntpd.drift"
)

func TestDriftWrite(t *testing.T) {

	s := NewService("")
	s.cfg = DefaultConfig()
	s.cfg.DriftFile = TestDriftFilename
	s.drift = 3.141592653

	err := s.WriteDrift()
	if err != nil {
		t.Error(err)
	}

	p, err := ioutil.ReadFile(TestDriftFilename)
	if err != nil {
		t.Error(err)
	}
	if string(p) != "3.141592653" {
		t.Errorf("write not match, got=%s", string(p))
	}
	os.Remove(TestDriftFilename)
}

func TestLoadDrift(t *testing.T) {
	s := NewService("")
	s.cfg = DefaultConfig()
	s.cfg.DriftFile = TestDriftFilename

	s.drift = 5.43
	s.WriteDrift()
	s.drift = -1
	err := s.LoadDrift()
	if err != nil {
		t.Error(err)
	}
	if s.drift != 5.43 {
		t.Error("load not match, got=%f", s.drift)
	}
	os.Remove(TestDriftFilename)
}
