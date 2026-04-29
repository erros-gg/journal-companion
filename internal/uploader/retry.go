package uploader

// TODO Phase 4: implement an offline queue with exponential backoff.
//
// When the server is unreachable or returns 5xx, uploads should be persisted
// to SQLite (status = "pending") and retried on next app launch or reconnect.
// Retry schedule: 5s, 30s, 2min, 10min, then give up after 24h.
