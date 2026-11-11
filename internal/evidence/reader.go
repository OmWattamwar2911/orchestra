package evidence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Reader parses and analyzes recorded evidence events.
type Reader struct {
	path string
}

// NewReader creates a reader pointing to an evidence log file or directory.
func NewReader(path string) *Reader {
	return &Reader{path: path}
}

// ReadAll reads all events from the log file or all .jsonl files in the target directory.
func (r *Reader) ReadAll() ([]EventRecord, error) {
	info, err := os.Stat(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat evidence path: %w", err)
	}

	var filesToRead []string
	if info.IsDir() {
		entries, err := os.ReadDir(r.path)
		if err != nil {
			return nil, fmt.Errorf("failed to read evidence directory: %w", err)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				filesToRead = append(filesToRead, filepath.Join(r.path, e.Name()))
			}
		}
	} else {
		filesToRead = append(filesToRead, r.path)
	}

	var records []EventRecord
	for _, fPath := range filesToRead {
		fileRecords, err := readFileRecords(fPath)
		if err != nil {
			return nil, err
		}
		records = append(records, fileRecords...)
	}

	return records, nil
}

func readFileRecords(path string) ([]EventRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open evidence log file %s: %w", path, err)
	}
	defer file.Close()

	var records []EventRecord
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec EventRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("corrupt record in %s on line %d: %w", filepath.Base(path), lineNum, err)
		}
		records = append(records, rec)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning evidence log %s: %w", path, err)
	}

	return records, nil
}
