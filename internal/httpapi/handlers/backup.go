package handlers

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/lebe-dev/turboist/internal/httpapi"
	"github.com/lebe-dev/turboist/internal/service"
)

// maxBackupUploadBytes caps the restore payload size. 64 MiB is generous for a
// fully-populated todoist-like dataset (gzipped) while still preventing trivial
// memory-exhaustion attempts against the in-memory decode path. Declared as
// var (not const) so tests can shrink it to verify the size guard without
// allocating dozens of megabytes per assertion.
var maxBackupUploadBytes = 64 * 1024 * 1024

// BackupHandler exposes export / restore of the full dataset.
//
//	GET  /api/v1/backup?settings=1 -> gzipped JSON snapshot; downloads as turboist-backup-YYYYMMDD.json
//	POST /api/v1/restore           -> wipes user data and imports the supplied payload (plain or gzipped)
//
// Both endpoints require a JWT session — restore is destructive, and export
// reveals the entire dataset, so neither is appropriate for long-lived API
// tokens. The group middleware in main.go enforces this on both routes.
type BackupHandler struct {
	svc *service.BackupService
}

func NewBackupHandler(svc *service.BackupService) *BackupHandler {
	return &BackupHandler{svc: svc}
}

func (h *BackupHandler) Register(r fiber.Router) {
	r.Get("/backup", h.export)
	r.Post("/restore", h.restore)
}

func (h *BackupHandler) export(c fiber.Ctx) error {
	includeSettings := boolQuery(c, "settings")
	logEntry(c, "handler.Backup.Export", slog.Bool("include_settings", includeSettings))
	payload, err := h.svc.Export(c.Context(), service.ExportOptions{IncludeSettings: includeSettings})
	if err != nil {
		return httpapi.ErrInternal("export backup").WithCause(err)
	}
	raw, err := payload.Marshal()
	if err != nil {
		return httpapi.ErrInternal("encode backup").WithCause(err)
	}

	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return httpapi.ErrInternal("init gzip").WithCause(err)
	}
	if _, err := zw.Write(raw); err != nil {
		return httpapi.ErrInternal("compress backup").WithCause(err)
	}
	if err := zw.Close(); err != nil {
		return httpapi.ErrInternal("finalize gzip").WithCause(err)
	}

	filename := fmt.Sprintf("turboist-backup-%s.json", time.Now().UTC().Format("20060102"))
	c.Set("Content-Type", "application/json")
	c.Set("Content-Encoding", "gzip")
	c.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Set("Cache-Control", "no-store")
	logMutation(c, "handler.Backup.Export",
		slog.Int("raw_bytes", len(raw)),
		slog.Int("gzip_bytes", buf.Len()),
		slog.Bool("include_settings", includeSettings),
	)
	return c.Send(buf.Bytes())
}

func (h *BackupHandler) restore(c fiber.Ctx) error {
	body := c.Body()
	logEntry(c, "handler.Backup.Restore", slog.Int("payload_bytes", len(body)))
	if len(body) == 0 {
		logValidation(c, "handler.Backup.Restore", "empty body")
		return httpapi.ErrValidation("empty body")
	}
	if len(body) > maxBackupUploadBytes {
		logValidation(c, "handler.Backup.Restore", "payload too large", slog.Int("payload_bytes", len(body)))
		return httpapi.ErrValidation("payload too large")
	}
	payload, err := service.DecodeBackup(body)
	if err != nil {
		// DecodeBackup wraps every failure with ErrBadBackup, so any error is
		// a client problem (malformed payload / unsupported version).
		logValidation(c, "handler.Backup.Restore", "decode failed", slog.String("err", err.Error()))
		return httpapi.ErrValidation("invalid backup file")
	}
	if err := h.svc.Restore(c.Context(), payload); err != nil {
		if errors.Is(err, service.ErrBadBackup) {
			logValidation(c, "handler.Backup.Restore", "bad backup", slog.String("err", err.Error()))
			return httpapi.ErrValidation("invalid backup file")
		}
		return httpapi.ErrInternal("restore backup").WithCause(err)
	}
	logMutation(c, "handler.Backup.Restore", slog.Int("payload_bytes", len(body)))
	return c.SendStatus(fiber.StatusNoContent)
}

func boolQuery(c fiber.Ctx, key string) bool {
	v := c.Query(key)
	return v == "1" || v == "true" || v == "yes"
}
