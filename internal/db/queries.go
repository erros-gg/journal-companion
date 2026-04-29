package db

import "time"

// Upload mirrors a row from the uploads table.
type Upload struct {
	ID                    string
	Label                 string
	FilePath              string
	FileHash              string
	FileSize              int64
	Status                string
	QueuedAt              time.Time
	CompletedAt           *time.Time
	ObservationsAccepted  *int
	ObservationsRejected  *int
	ObservationsDuplicate *int
	Warnings              *string // JSON array
	ErrorCode             *string
	ErrorUserMessage      *string
}

// RecordUpload inserts a new upload row with status "pending".
// Called immediately after the companion receives an upload_id from the server.
// Phase 2 implementation.
func (db *DB) RecordUpload(u Upload) error {
	_, err := db.Exec(`
		INSERT INTO uploads (id, label, file_path, file_hash, file_size, status, queued_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
		u.ID, u.Label, u.FilePath, u.FileHash, u.FileSize, u.QueuedAt,
	)
	return err
}

// UpdateUploadResult writes the polling outcome back to an existing row.
// Phase 2 implementation.
func (db *DB) UpdateUploadResult(id, status string, completedAt time.Time, u Upload) error {
	_, err := db.Exec(`
		UPDATE uploads SET
			status = ?, completed_at = ?,
			observations_accepted = ?, observations_rejected = ?, observations_duplicate = ?,
			warnings = ?, error_code = ?, error_user_message = ?
		WHERE id = ?`,
		status, completedAt,
		u.ObservationsAccepted, u.ObservationsRejected, u.ObservationsDuplicate,
		u.Warnings, u.ErrorCode, u.ErrorUserMessage,
		id,
	)
	return err
}

// WatchHash returns the last-seen hash for path, or ("", nil) if unknown.
func (db *DB) WatchHash(path string) (string, error) {
	var hash string
	err := db.QueryRow(`SELECT last_hash FROM watch_state WHERE path = ?`, path).Scan(&hash)
	if err != nil {
		return "", nil // not found is not an error
	}
	return hash, nil
}

// SetWatchHash records the most recent hash seen for path.
func (db *DB) SetWatchHash(path, hash, uploadID string) error {
	_, err := db.Exec(`
		INSERT INTO watch_state (path, last_hash, last_upload_id, last_seen_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET
			last_hash = excluded.last_hash,
			last_upload_id = excluded.last_upload_id,
			last_seen_at = excluded.last_seen_at`,
		path, hash, uploadID,
	)
	return err
}

// RecentUploads returns the most recent uploads, newest first.
func (db *DB) RecentUploads(limit int) ([]Upload, error) {
	rows, err := db.Query(`
		SELECT id, label, file_path, file_hash, file_size, status,
		       queued_at, completed_at,
		       observations_accepted, observations_rejected, observations_duplicate,
		       warnings, error_code, error_user_message
		FROM uploads
		ORDER BY queued_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uploads []Upload
	for rows.Next() {
		var u Upload
		if err := rows.Scan(
			&u.ID, &u.Label, &u.FilePath, &u.FileHash, &u.FileSize, &u.Status,
			&u.QueuedAt, &u.CompletedAt,
			&u.ObservationsAccepted, &u.ObservationsRejected, &u.ObservationsDuplicate,
			&u.Warnings, &u.ErrorCode, &u.ErrorUserMessage,
		); err != nil {
			return nil, err
		}
		uploads = append(uploads, u)
	}
	return uploads, rows.Err()
}
