// Package uploader handles file compression, HTTP upload, and result polling (Phase 2).
//
// Protocol summary:
//   POST /api/companion/upload
//     Headers: Authorization, Content-Type: application/octet-stream,
//              Content-Encoding: gzip, X-Companion-Label, X-Companion-Filename,
//              X-Companion-Filesize-Original, X-Companion-Filehash-Original,
//              X-Companion-Companion-Version, X-Companion-OS
//     Body: gzip-compressed raw file bytes
//     Response 202: { uploadId, statusUrl, queuedAt }
//
//   GET /api/companion/upload/:id  — poll until status != "queued"|"parsing"
//     Backoff: every 2s for first 30s, then every 10s up to 5 min total.
package uploader

// TODO Phase 2: implement Client, Upload(path, label, token string) (UploadResult, error)
