package sntpd

import (
	"io/ioutil"
	"strconv"
	"strings"
)

func (s *Service) LoadDrift() (err error) {
	var p []byte
	p, err = ioutil.ReadFile(s.cfg.DriftFile)
	if err != nil {
		return
	}
	fp := strings.Replace(string(p), "\n", "", 1)
	s.drift, err = strconv.ParseFloat(fp, 64)
	return
}

func (s *Service) WriteDrift() (err error) {
	var dst []byte
	dst = strconv.AppendFloat(dst, s.drift, 'f', -1, 64)
	return ioutil.WriteFile(s.cfg.DriftFile, dst, 0644)
}
