package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Put(ctx context.Context, in PutInput) (*PutResult, error) {
	if in.GameID == "" || in.PlayerID == "" || in.Slot == "" {
		return nil, &httpProblem{status: 400, message: "game_id, player_id, slot wajib diisi."}
	}
	enc := in.Encoding
	if enc == "" {
		enc = "identity"
	}
	if enc != "identity" && enc != "gzip" {
		return nil, &httpProblem{status: 415, message: "encoding tidak didukung: " + enc}
	}
	data := in.Data
	if enc == "gzip" {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(data); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
		data = buf.Bytes()
	}
	if in.ContentType == "" {
		in.ContentType = "application/octet-stream"
	}

	// mulai transaksi sebelum cek existing (hindari TOCTOU)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var currentVersion int64
	var exists bool
	err = tx.QueryRow(ctx, `
		select version from saves where game_id=$1::uuid and player_id=$2::uuid and slot=$3
		for update`, in.GameID, in.PlayerID, in.Slot).Scan(&currentVersion)
	if err == nil {
		exists = true
	} else if err != pgx.ErrNoRows {
		return nil, err
	}

	if in.IfMatch > 0 {
		if !exists {
			return nil, &httpProblem{status: 412, message: "slot belum ada; If-Match tidak berlaku."}
		}
		if in.IfMatch != currentVersion {
			return nil, &httpProblem{status: 409, message: fmt.Sprintf("versi tidak cocok: klien=%d server=%d", in.IfMatch, currentVersion)}
		}
	} else if exists {
		// tanpa If-Match: tulis baru (latest wins) — izinkan, tapi naikkan version
	}

	newVersion := int64(1)
	if exists {
		newVersion = currentVersion + 1
	}

	checksum := sha256Hex(data)
	blobRef, err := s.writeBlob(in.GameID, in.PlayerID, in.Slot, newVersion, data)
	if err != nil {
		return nil, err
	}

	if exists {
		_, err = tx.Exec(ctx, `
			update saves set version=$1, blob_ref=$2, checksum=$3, size_bytes=$4,
				encoding=$5, is_deleted=false, updated_at=now()
			where game_id=$6::uuid and player_id=$7::uuid and slot=$8`,
			newVersion, blobRef, checksum, len(data), enc, in.GameID, in.PlayerID, in.Slot)
	} else {
		_, err = tx.Exec(ctx, `
			insert into saves (game_id, player_id, slot, version, blob_ref, checksum, size_bytes, encoding)
			values ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8)`,
			in.GameID, in.PlayerID, in.Slot, newVersion, blobRef, checksum, len(data), enc)
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `
		insert into save_versions (save_id, version, blob_ref, checksum, size_bytes, encoding)
		select id, $1, $2, $3, $4, $5 from saves where game_id=$6::uuid and player_id=$7::uuid and slot=$8`,
		newVersion, blobRef, checksum, len(data), enc, in.GameID, in.PlayerID, in.Slot)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// pruning versi lama
	s.pruneVersions(ctx, in.GameID, in.PlayerID, in.Slot)

	return &PutResult{Slot: in.Slot, Version: newVersion, Checksum: checksum, Size: int64(len(data)), Encoding: enc}, nil
}

func (s *Service) Get(ctx context.Context, gameID, playerID, slot string) (*SlotMeta, error) {
	var meta SlotMeta
	var blobRef, enc string
	var isDeleted bool
	err := s.pool.QueryRow(ctx, `
		select version, blob_ref, checksum, size_bytes, encoding, updated_at, is_deleted
		from saves where game_id=$1::uuid and player_id=$2::uuid and slot=$3`,
		gameID, playerID, slot).Scan(&meta.Version, &blobRef, &meta.Checksum, &meta.Size, &enc, &meta.UpdatedAt, &isDeleted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &httpProblem{status: 404, message: "slot tidak ditemukan."}
		}
		return nil, err
	}
	if isDeleted {
		return nil, &httpProblem{status: 404, message: "slot sudah dihapus."}
	}
	data, err := s.readBlob(blobRef)
	if err != nil {
		return nil, err
	}
	if enc == "gzip" {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		data, err = io.ReadAll(gz)
		if err != nil {
			return nil, err
		}
	}
	meta.Slot = slot
	meta.Encoding = enc
	meta.IsDeleted = isDeleted
	meta.Data = data
	return &meta, nil
}

func (s *Service) ListSlots(ctx context.Context, gameID, playerID string) ([]SlotMeta, error) {
	rows, err := s.pool.Query(ctx, `
		select slot, version, blob_ref, checksum, size_bytes, encoding, updated_at, is_deleted
		from saves where game_id=$1::uuid and player_id=$2::uuid and is_deleted=false
		order by updated_at desc`, gameID, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SlotMeta
	for rows.Next() {
		var m SlotMeta
		var blobRef, enc string
		var isDeleted bool
		if err := rows.Scan(&m.Slot, &m.Version, &blobRef, &m.Checksum, &m.Size, &enc, &m.UpdatedAt, &isDeleted); err != nil {
			return nil, err
		}
		m.Encoding = enc
		m.IsDeleted = isDeleted
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) ListVersions(ctx context.Context, gameID, playerID, slot string) ([]VersionInfo, error) {
	rows, err := s.pool.Query(ctx, `
		select sv.version, sv.checksum, sv.size_bytes, sv.encoding, sv.created_at
		from save_versions sv
		join saves s on s.id = sv.save_id
		where s.game_id=$1::uuid and s.player_id=$2::uuid and s.slot=$3
		order by sv.version desc`, gameID, playerID, slot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VersionInfo
	for rows.Next() {
		var v VersionInfo
		if err := rows.Scan(&v.Version, &v.Checksum, &v.Size, &v.Encoding, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) Delete(ctx context.Context, gameID, playerID, slot string) error {
	ct, err := s.pool.Exec(ctx, `
		update saves set is_deleted=true, updated_at=now()
		where game_id=$1::uuid and player_id=$2::uuid and slot=$3 and is_deleted=false`,
		gameID, playerID, slot)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return &httpProblem{status: 404, message: "slot tidak ditemukan."}
	}
	return nil
}

func (s *Service) Delta(ctx context.Context, gameID, playerID string, since time.Time) ([]SlotMeta, error) {
	rows, err := s.pool.Query(ctx, `
		select slot, version, blob_ref, checksum, size_bytes, encoding, updated_at, is_deleted
		from saves where game_id=$1::uuid and player_id=$2::uuid and updated_at > $3
		order by updated_at asc`, gameID, playerID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SlotMeta
	for rows.Next() {
		var m SlotMeta
		var blobRef, enc string
		if err := rows.Scan(&m.Slot, &m.Version, &blobRef, &m.Checksum, &m.Size, &enc, &m.UpdatedAt, &m.IsDeleted); err != nil {
			return nil, err
		}
		m.Encoding = enc
		out = append(out, m)
	}
	return out, rows.Err()
}

// --- blob store ---

func (s *Service) blobPath(gameID, playerID, slot string, version int64) string {
	safe := func(x string) string {
		// remove path separators and dangerous components
		x = strings.ReplaceAll(x, "/", "_")
		x = strings.ReplaceAll(x, "\\", "_")
		x = strings.ReplaceAll(x, "..", "_")
		if x == "" {
			x = "_"
		}
		r := []rune(x)
		if len(r) > 64 {
			r = r[:64]
		}
		return string(r)
	}
	return filepath.Join(s.blobDir, safe(gameID), safe(playerID), safe(slot), fmt.Sprintf("v%d.bin", version))
}

func (s *Service) writeBlob(gameID, playerID, slot string, version int64, data []byte) (string, error) {
	p := s.blobPath(gameID, playerID, slot, version)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return "", err
	}
	return p, nil
}

func (s *Service) readBlob(ref string) ([]byte, error) {
	return os.ReadFile(ref) // #nosec G304
}

func (s *Service) pruneVersions(ctx context.Context, gameID, playerID, slot string) {
	rows, err := s.pool.Query(ctx, `
		select sv.id, sv.blob_ref, sv.version
		from save_versions sv
		join saves sa on sa.id = sv.save_id
		where sa.game_id=$1::uuid and sa.player_id=$2::uuid and sa.slot=$3
		order by sv.version desc`, gameID, playerID, slot)
	if err != nil {
		return
	}
	defer rows.Close()
	type v struct {
		id  int64
		ref string
		ver int64
	}
	var all []v
	for rows.Next() {
		var x v
		if err := rows.Scan(&x.id, &x.ref, &x.ver); err == nil {
			all = append(all, x)
		}
	}
	if len(all) <= s.keepVersions {
		return
	}
	toDelete := all[s.keepVersions:]
	ids := make([]int64, 0, len(toDelete))
	for _, x := range toDelete {
		ids = append(ids, x.id)
	}
	_, _ = s.pool.Exec(ctx, "delete from save_versions where id = any($1)", ids)
	for _, x := range toDelete {
		_ = os.Remove(x.ref)
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
