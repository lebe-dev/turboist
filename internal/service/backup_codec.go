package service

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrBadBackup is returned when the input bytes are not a recognizable backup.
var ErrBadBackup = errors.New("backup: invalid payload")

// Marshal serializes the payload into a single JSON document. The encoder is
// configured to keep IDs as raw numbers, which json.Marshal already does.
func (p *BackupPayload) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

// DecodeBackup parses a backup payload. It transparently accepts either a
// plain JSON document or a gzipped one (detected via the 0x1f 0x8b magic).
func DecodeBackup(raw []byte) (*BackupPayload, error) {
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: gzip header: %v", ErrBadBackup, err)
		}
		defer func() { _ = zr.Close() }()
		decoded, err := io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("%w: gzip body: %v", ErrBadBackup, err)
		}
		raw = decoded
	}
	var p BackupPayload
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadBackup, err)
	}
	if p.Version != BackupSchemaVersion {
		return nil, fmt.Errorf("%w: unsupported version %d (want %d)", ErrBadBackup, p.Version, BackupSchemaVersion)
	}
	return &p, nil
}
