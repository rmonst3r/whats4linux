package cache

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	query "github.com/lugvitc/whats4linux/internal/query"
	_ "github.com/mattn/go-sqlite3"
)

type ImageCache struct {
	db        *sql.DB
	imagesDir string
	getStmt   *sql.Stmt // Prepared statement for single image retrieval
	saveStmt  *sql.Stmt // Prepared statement for saving images
	mu        sync.Mutex
	lastEvict time.Time
}

const maxImageCacheBytes int64 = 512 << 20

type ImageMeta struct {
	MessageID string
	SHA256    string
	Mime      string
	Width     int
	Height    int
	CreatedAt int64
}

// NewImageCache creates a new image cache instance
func NewImageCache() (*ImageCache, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache directory: %v", err)
	}

	baseDir := filepath.Join(cacheDir, "whats4linux")
	imagesDir := filepath.Join(baseDir, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create images directory: %v", err)
	}

	dbPath := filepath.Join(baseDir, "idxdb")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create idxdb directory: %v", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	if _, err := db.Exec(query.CreateImageIndexTable); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %v", err)
	}

	getStmt, err := db.Prepare(query.GetImageByID)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to prepare get statement: %v", err)
	}

	saveStmt, err := db.Prepare(query.SaveImageIndex)
	if err != nil {
		getStmt.Close()
		db.Close()
		return nil, fmt.Errorf("failed to prepare save statement: %v", err)
	}

	ic := &ImageCache{
		db:        db,
		imagesDir: imagesDir,
		getStmt:   getStmt,
		saveStmt:  saveStmt,
	}
	if err := ic.evictToSize(maxImageCacheBytes); err != nil {
		log.Println("image cache eviction failed:", err)
	}
	ic.lastEvict = time.Now()

	return ic, nil
}

// SaveImage saves an image to cache and creates an index entry
func (ic *ImageCache) SaveImage(messageID string, data []byte, mime string, width, height int) (string, error) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	h := sha256.Sum256(data)
	hashStr := hex.EncodeToString(h[:])

	ext := mimeToExt(mime)
	path := filepath.Join(ic.imagesDir, hashStr+ext)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return "", fmt.Errorf("failed to write image file: %v", err)
		}
	}

	_, err := ic.saveStmt.Exec(messageID, hashStr, mime, width, height, time.Now().Unix())
	if err != nil {
		return "", fmt.Errorf("failed to insert image index: %v", err)
	}
	if time.Since(ic.lastEvict) >= 5*time.Minute {
		if err := ic.evictToSizeLocked(maxImageCacheBytes); err != nil {
			log.Println("image cache eviction failed:", err)
		}
		ic.lastEvict = time.Now()
	}

	return hashStr, nil
}

type evictionCandidate struct {
	messageID string
	sha256    string
	mime      string
}

func (ic *ImageCache) evictToSize(maxBytes int64) error {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.evictToSizeLocked(maxBytes)
}

func (ic *ImageCache) evictToSizeLocked(maxBytes int64) error {
	entries, err := os.ReadDir(ic.imagesDir)
	if err != nil {
		return err
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err == nil {
			total += info.Size()
		}
	}
	if total <= maxBytes {
		return nil
	}

	rows, err := ic.db.Query(`SELECT message_id, sha256, mime FROM image_index ORDER BY created_at ASC`)
	if err != nil {
		return err
	}
	var candidates []evictionCandidate
	for rows.Next() {
		var candidate evictionCandidate
		if err := rows.Scan(&candidate.messageID, &candidate.sha256, &candidate.mime); err != nil {
			_ = rows.Close()
			return err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, candidate := range candidates {
		if total <= maxBytes {
			break
		}
		if _, err := ic.db.Exec(query.DeleteImageIndex, candidate.messageID); err != nil {
			return err
		}
		var references int
		if err := ic.db.QueryRow(
			`SELECT COUNT(*) FROM image_index WHERE sha256 = ? AND mime = ?`,
			candidate.sha256,
			candidate.mime,
		).Scan(&references); err != nil {
			return err
		}
		if references != 0 {
			continue
		}
		path := filepath.Join(ic.imagesDir, candidate.sha256+mimeToExt(candidate.mime))
		if info, err := os.Stat(path); err == nil {
			total -= info.Size()
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// GetImageByMessageID retrieves image metadata by message ID
func (ic *ImageCache) GetImageByMessageID(messageID string) (*ImageMeta, error) {
	var meta ImageMeta
	err := ic.getStmt.QueryRow(messageID).Scan(
		&meta.MessageID,
		&meta.SHA256,
		&meta.Mime,
		&meta.Width,
		&meta.Height,
		&meta.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &meta, err
}

// GetImagesByMessageIDs retrieves multiple image metadata by message IDs (batch)
func (ic *ImageCache) GetImagesByMessageIDs(messageIDs []string) (map[string]*ImageMeta, error) {
	if len(messageIDs) == 0 {
		return make(map[string]*ImageMeta), nil
	}

	placeholders := make([]string, len(messageIDs))
	args := make([]any, len(messageIDs))
	for i, id := range messageIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	q := query.GetImagesByIDsPrefix + strings.Join(placeholders, ",") + ")"
	rows, err := ic.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*ImageMeta, len(messageIDs))
	for rows.Next() {
		var meta ImageMeta
		if err := rows.Scan(
			&meta.MessageID,
			&meta.SHA256,
			&meta.Mime,
			&meta.Width,
			&meta.Height,
			&meta.CreatedAt,
		); err != nil {
			return nil, err
		}
		result[meta.MessageID] = &meta
	}
	return result, rows.Err()
}

// GetImageFilePath returns the file path for a cached image by message ID
func (ic *ImageCache) GetImageFilePath(messageID string) (string, error) {
	meta, err := ic.GetImageByMessageID(messageID)
	if err != nil {
		return "", err
	}
	if meta == nil {
		return "", fmt.Errorf("image not found for message ID: %s", messageID)
	}

	return meta.SHA256 + mimeToExt(meta.Mime), nil
}

// ReadImageByMessageID reads an image by message ID
func (ic *ImageCache) ReadImageByMessageID(messageID string) ([]byte, string, error) {
	meta, err := ic.GetImageByMessageID(messageID)
	if err != nil {
		return nil, "", err
	}
	if meta == nil {
		return nil, "", fmt.Errorf("image not found for message ID: %s", messageID)
	}

	path := filepath.Join(ic.imagesDir, meta.SHA256+mimeToExt(meta.Mime))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image file: %v", err)
	}

	return data, meta.Mime, nil
}

// SaveAvatar saves an avatar image to cache using JID as the key
func (ic *ImageCache) SaveAvatar(jid string, data []byte, mime string) (string, error) {
	avatarKey := "avatar_" + jid
	return ic.SaveImage(avatarKey, data, mime, 0, 0)
}

// DeleteAvatar deletes an avatar image from cache by JID
func (ic *ImageCache) DeleteAvatar(jid string) error {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	avatarKey := "avatar_" + jid
	meta, err := ic.GetImageByMessageID(avatarKey)
	if err != nil || meta == nil {
		return err
	}
	if _, err = ic.db.Exec(query.DeleteImageIndex, avatarKey); err != nil {
		return fmt.Errorf("failed to delete avatar index: %v", err)
	}

	var references int
	if err := ic.db.QueryRow(
		`SELECT COUNT(*) FROM image_index WHERE sha256 = ? AND mime = ?`,
		meta.SHA256,
		meta.Mime,
	).Scan(&references); err != nil {
		return err
	}
	if references != 0 {
		return nil
	}

	filePath := filepath.Join(ic.imagesDir, meta.SHA256+mimeToExt(meta.Mime))
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete avatar file: %v", err)
	}
	return nil
}

// GetAvatarFilePath returns the file path for a cached avatar by JID
func (ic *ImageCache) GetAvatarFilePath(jid string) (string, error) {
	avatarKey := "avatar_" + jid
	return ic.GetImageFilePath(avatarKey)
}

// ReadAvatarByJID reads an avatar image by JID
func (ic *ImageCache) ReadAvatarByJID(jid string) ([]byte, string, error) {
	avatarKey := "avatar_" + jid
	return ic.ReadImageByMessageID(avatarKey)
}

// Close closes the database connection and prepared statements
func (ic *ImageCache) Close() error {
	if ic.getStmt != nil {
		ic.getStmt.Close()
	}
	if ic.saveStmt != nil {
		ic.saveStmt.Close()
	}
	return ic.db.Close()
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}
