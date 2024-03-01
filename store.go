package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

/* Storage */
type Store struct {
	Dir string
}

type CacheKey struct {
	Hash string
}

func (c *Store) KeyFrom(command string, args []string) CacheKey {
	concatCmd := command + " " + strings.Join(args, " ")
	h := sha256.Sum256([]byte(concatCmd))
	return CacheKey{
		Hash: fmt.Sprintf("%x", h[:]),
	}
}

func (s *Store) stdoutPath(key CacheKey) string {
	return filepath.Join(s.KeyDir(key), "out")
}

func (s *Store) stderrPath(key CacheKey) string {
	return filepath.Join(s.KeyDir(key), "err")
}

func (s *Store) exitcodePath(key CacheKey) string {
	return filepath.Join(s.KeyDir(key), "status")
}

func (s *Store) WriteToCache(key CacheKey, stdout, stderr []byte, exitCode int) error {
	cacheDir := s.KeyDir(key)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	if err := os.WriteFile(s.stdoutPath(key), stdout, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(s.stderrPath(key), stderr, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(s.exitcodePath(key), []byte(fmt.Sprint(exitCode)), 0644); err != nil {
		return err
	}

	return nil
}

func (s *Store) ReadFromCache(key CacheKey) (stdout, stderr []byte, exitCode int, err error) {
	stdout, err = os.ReadFile(s.stdoutPath(key))
	if err != nil {
		return
	}
	stderr, err = os.ReadFile(s.stderrPath(key))
	if err != nil {
		return
	}
	exitCodeBytes, err := os.ReadFile(s.exitcodePath(key))
	if err != nil {
		return
	}
	exitCode, err = strconv.Atoi(string(exitCodeBytes))
	return
}
