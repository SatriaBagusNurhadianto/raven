package storage

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type httpProblem struct {
	status  int
	message string
}

func (e *httpProblem) Error() string { return e.message }

var ErrNotFound = errors.New("tidak ditemukan")

type PutInput struct {
	GameID      string
	PlayerID    string
	Slot        string
	Data        []byte
	ContentType string
	IfMatch     int64
	HasIfMatch  bool
	Encoding    string
}

type PutResult struct {
	Slot     string
	Version  int64
	Checksum string
	Size     int64
	Encoding string
}

type SlotMeta struct {
	Slot      string
	Version   int64
	Checksum  string
	Size      int64
	Encoding  string
	UpdatedAt time.Time
	IsDeleted bool
	Data      []byte
}

type VersionInfo struct {
	Version   int64
	Checksum  string
	Size      int64
	Encoding  string
	CreatedAt time.Time
}

type Service struct {
	pool         *pgxpool.Pool
	blobDir      string
	keepVersions int
}

func NewService(pool *pgxpool.Pool, blobDir string, keepVersions int) *Service {
	return &Service{pool: pool, blobDir: blobDir, keepVersions: keepVersions}
}

// Service implementations live in service.go.
