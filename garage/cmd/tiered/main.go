// GARAGE Tiered Storage Service
//
// Provides three-tier storage:
//   Tier 0: Local cache (instant, offline)
//   Tier 1: R2 (hot, free egress)
//   Tier 2: B2 (cold, cheapest)
//
// Usage:
//   tiered serve          Start HTTP proxy server
//   tiered sync           Sync local → R2 (background)
//   tiered archive        Archive R2 → B2 (old files)
//   tiered status         Show tier status
//
// Multi-device sync (optional):
//   Set PBHA_URL to enable PocketBase tracking for multi-device sync
//   PBHA_URL              PocketBase URL (e.g., http://localhost:8090)
//   PBHA_ADMIN_EMAIL      Admin email
//   PBHA_ADMIN_PASS       Admin password
//   DEVICE_NAME           This device's name (defaults to hostname)

package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/joeblew999/xplat/garage/pkg/pbclient"
	_ "github.com/rclone/rclone/backend/b2"
	_ "github.com/rclone/rclone/backend/local"
	_ "github.com/rclone/rclone/backend/s3"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configfile"
	"github.com/rclone/rclone/fs/object"
	"github.com/rclone/rclone/fs/operations"
	_ "modernc.org/sqlite"
)

type TierConfig struct {
	// Local cache
	LocalPath    string
	LocalMaxSize int64 // bytes

	// R2 (Tier 1)
	R2AccountID string
	R2Bucket    string
	R2AccessKey string
	R2SecretKey string

	// B2 (Tier 2)
	B2Account string
	B2Key     string
	B2Bucket  string

	// Tiering policy
	ArchiveAfterDays int // Move R2 → B2 after N days
}

type TieredStorage struct {
	local      fs.Fs
	r2         fs.Fs
	b2         fs.Fs
	db         *sql.DB
	config     TierConfig
	pb         *pbclient.Client     // PocketBase HTTP client for metadata
	nats       *pbclient.NATSClient // NATS client for real-time events
	deviceName string
}

func NewTieredStorage(cfg TierConfig) (*TieredStorage, error) {
	ctx := context.Background()

	// Initialize rclone config
	configfile.Install()

	// Tier 0: Local filesystem
	local, err := fs.NewFs(ctx, cfg.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("local fs: %w", err)
	}

	// Tier 1: R2 (S3-compatible)
	// Format: :s3,provider=Cloudflare,...:bucket
	// Must quote endpoint URL to prevent ':' in https:// being parsed as path separator
	// Must add no_check_bucket=true for bucket-scoped API tokens
	r2Spec := fmt.Sprintf(":s3,provider=Cloudflare,access_key_id=%s,secret_access_key=%s,endpoint='https://%s.r2.cloudflarestorage.com',region=auto,no_check_bucket=true:%s",
		cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2AccountID, cfg.R2Bucket)
	r2, err := fs.NewFs(ctx, r2Spec)
	if err != nil {
		return nil, fmt.Errorf("r2 fs: %w", err)
	}

	// Tier 2: B2
	var b2 fs.Fs
	if cfg.B2Account != "" && cfg.B2Key != "" {
		b2Spec := fmt.Sprintf(":b2,account=%s,key=%s:%s", cfg.B2Account, cfg.B2Key, cfg.B2Bucket)
		b2, err = fs.NewFs(ctx, b2Spec)
		if err != nil {
			log.Printf("Warning: B2 not configured: %v", err)
		}
	}

	// Initialize SQLite for tier tracking
	dbPath := filepath.Join(cfg.LocalPath, ".garage-tiers.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS file_tier (
			key TEXT PRIMARY KEY,
			size INTEGER,
			hash TEXT,
			created_at INTEGER,
			last_accessed INTEGER,
			tier INTEGER DEFAULT 0,
			is_local BOOLEAN DEFAULT TRUE,
			is_r2 BOOLEAN DEFAULT FALSE,
			is_b2 BOOLEAN DEFAULT FALSE,
			r2_synced_at INTEGER,
			b2_archived_at INTEGER
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}

	ts := &TieredStorage{
		local:  local,
		r2:     r2,
		b2:     b2,
		db:     db,
		config: cfg,
	}

	// Initialize PocketBase client for multi-device sync (optional)
	if pbURL := os.Getenv("PBHA_URL"); pbURL != "" {
		deviceName := os.Getenv("DEVICE_NAME")
		if deviceName == "" {
			hostname, _ := os.Hostname()
			deviceName = fmt.Sprintf("%s-%s", hostname, runtime.GOOS)
		}
		ts.deviceName = deviceName

		pb := pbclient.NewClient(pbURL, deviceName)

		// Authenticate
		email := os.Getenv("PBHA_ADMIN_EMAIL")
		pass := os.Getenv("PBHA_ADMIN_PASS")
		if email != "" && pass != "" {
			if err := pb.Authenticate(email, pass); err != nil {
				log.Printf("Warning: PocketBase auth failed: %v (multi-device sync disabled)", err)
			} else {
				ts.pb = pb
				// Register this device
				if err := pb.RegisterDevice("cli", runtime.GOOS); err != nil {
					log.Printf("Warning: Failed to register device: %v", err)
				}
				log.Printf("Multi-device sync enabled: %s -> %s", deviceName, pbURL)
			}
		}

		// Connect to NATS for real-time events (control plane)
		natsURL := os.Getenv("NATS_URL")
		if natsURL == "" {
			// Default: PocketBase-HA runs NATS on port 4222
			natsURL = "nats://localhost:4222"
		}
		natsClient, err := pbclient.NewNATSClient(natsURL, deviceName)
		if err != nil {
			log.Printf("Warning: NATS connect failed: %v (real-time sync disabled)", err)
		} else {
			ts.nats = natsClient
			log.Printf("NATS connected: %s", natsURL)

			// Subscribe to file changes from other devices
			ts.nats.SubscribeToChanges(func(event *pbclient.FileEvent) {
				log.Printf("NATS: %s changed %s (v%d) on device %s",
					event.Type, event.Path, event.Version, event.DeviceName)
				// TODO: Trigger local sync if needed
			})

			// Subscribe to sync completion events
			ts.nats.SubscribeToSyncs(func(event *pbclient.FileEvent) {
				log.Printf("NATS: %s synced to R2 by %s", event.Path, event.DeviceName)
			})
		}
	}

	return ts, nil
}

// computeHash computes MD5 hash of a file for change detection
func computeHash(r io.Reader) string {
	h := md5.New()
	io.Copy(h, r)
	return hex.EncodeToString(h.Sum(nil))
}

// Get retrieves a file, checking tiers in order: local → R2 → B2
func (t *TieredStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	// 1. Check local cache
	if obj, err := t.local.NewObject(ctx, key); err == nil {
		t.updateAccess(key)
		return obj.Open(ctx)
	}

	// 2. Check R2, cache locally
	if obj, err := t.r2.NewObject(ctx, key); err == nil {
		// Copy to local cache
		if err := operations.CopyFile(ctx, t.local, t.r2, key, key); err != nil {
			log.Printf("Warning: failed to cache from R2: %v", err)
		}
		t.updateTier(key, 1, true, true, false)
		return obj.Open(ctx)
	}

	// 3. Check B2, restore to R2 and local
	if t.b2 != nil {
		if obj, err := t.b2.NewObject(ctx, key); err == nil {
			// Restore: B2 → R2 → Local
			if err := operations.CopyFile(ctx, t.r2, t.b2, key, key); err != nil {
				log.Printf("Warning: failed to restore to R2: %v", err)
			}
			if err := operations.CopyFile(ctx, t.local, t.r2, key, key); err != nil {
				log.Printf("Warning: failed to cache locally: %v", err)
			}
			t.updateTier(key, 0, true, true, true)
			return obj.Open(ctx)
		}
	}

	return nil, fs.ErrorObjectNotFound
}

// Put stores a file locally and queues background sync to R2
func (t *TieredStorage) Put(ctx context.Context, key string, data io.Reader, size int64) error {
	// Buffer data to compute hash and then write
	var buf bytes.Buffer
	tee := io.TeeReader(data, &buf)
	hash := computeHash(tee)

	// Write to local (instant for user)
	info := object.NewStaticObjectInfo(key, time.Now(), size, true, nil, nil)
	obj, err := t.local.Put(ctx, &buf, info)
	if err != nil {
		return fmt.Errorf("local put: %w", err)
	}

	// Track in local database
	_, err = t.db.Exec(`
		INSERT OR REPLACE INTO file_tier (key, size, hash, created_at, last_accessed, is_local)
		VALUES (?, ?, ?, ?, ?, TRUE)
	`, key, obj.Size(), hash, time.Now().Unix(), time.Now().Unix())
	if err != nil {
		log.Printf("Warning: failed to track file: %v", err)
	}

	// Track in PocketBase for multi-device sync
	if t.pb != nil {
		go t.trackInPocketBase(key, obj.Size(), hash)
	}

	// Background sync to R2 (async)
	go t.syncToR2(key)

	return nil
}

// trackInPocketBase creates/updates file record in PocketBase for multi-device tracking
func (t *TieredStorage) trackInPocketBase(key string, size int64, hash string) {
	existing, err := t.pb.GetFile(key)
	if err != nil {
		log.Printf("Warning: PB lookup failed for %s: %v", key, err)
		return
	}

	var version int
	if existing == nil {
		// New file
		file := &pbclient.GarageFile{
			Path:           key,
			Filename:       filepath.Base(key),
			Size:           size,
			Hash:           hash,
			Tier:           0,
			CurrentVersion: 1,
		}
		if err := t.pb.CreateFile(file); err != nil {
			log.Printf("Warning: PB create failed for %s: %v", key, err)
			return
		}
		version = 1

		// Create first version
		fileVersion := &pbclient.FileVersion{
			FilePath:   key,
			VersionNum: 1,
			Size:       size,
			Hash:       hash,
		}
		t.pb.CreateVersion(fileVersion)
	} else if existing.Hash != hash {
		// File changed - create new version
		version = existing.CurrentVersion + 1
		existing.Size = size
		existing.Hash = hash
		existing.CurrentVersion = version
		if err := t.pb.UpdateFile(existing); err != nil {
			log.Printf("Warning: PB update failed for %s: %v", key, err)
			return
		}

		fileVersion := &pbclient.FileVersion{
			FilePath:   key,
			VersionNum: version,
			Size:       size,
			Hash:       hash,
		}
		t.pb.CreateVersion(fileVersion)
	} else {
		version = existing.CurrentVersion
	}

	// Update device cache
	t.pb.UpdateDeviceCache(key, version, filepath.Join(t.config.LocalPath, key), true)

	// Publish NATS event for real-time sync to other devices
	if t.nats != nil {
		if err := t.nats.PublishFileChanged(key, hash, size, version); err != nil {
			log.Printf("Warning: NATS publish failed: %v", err)
		}
	}
}

// syncToR2 copies a file from local to R2 in the background
func (t *TieredStorage) syncToR2(key string) {
	ctx := context.Background()
	if err := operations.CopyFile(ctx, t.r2, t.local, key, key); err != nil {
		log.Printf("Sync to R2 failed for %s: %v", key, err)
		return
	}

	// Get hash for NATS event
	var hash string
	t.db.QueryRow(`SELECT hash FROM file_tier WHERE key = ?`, key).Scan(&hash)

	_, _ = t.db.Exec(`
		UPDATE file_tier SET is_r2 = TRUE, r2_synced_at = ? WHERE key = ?
	`, time.Now().Unix(), key)

	// Update PocketBase tier info
	if t.pb != nil {
		if file, err := t.pb.GetFile(key); err == nil && file != nil {
			file.Tier = 1
			file.R2Key = key
			t.pb.UpdateFile(file)
		}
		// Mark as synced (not dirty)
		t.pb.UpdateDeviceCache(key, 0, filepath.Join(t.config.LocalPath, key), false)
	}

	// Publish NATS event - file is now available in R2
	if t.nats != nil {
		t.nats.PublishFileSynced(key, hash, key)
	}

	log.Printf("Synced to R2: %s", key)
}

// Archive moves old files from R2 to B2
func (t *TieredStorage) Archive(ctx context.Context) error {
	if t.b2 == nil {
		return fmt.Errorf("B2 not configured")
	}

	cutoff := time.Now().AddDate(0, 0, -t.config.ArchiveAfterDays).Unix()

	rows, err := t.db.Query(`
		SELECT key FROM file_tier
		WHERE is_r2 = TRUE AND is_b2 = FALSE AND last_accessed < ?
	`, cutoff)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			continue
		}

		// Copy R2 → B2
		if err := operations.CopyFile(ctx, t.b2, t.r2, key, key); err != nil {
			log.Printf("Archive failed for %s: %v", key, err)
			continue
		}

		// Delete from R2
		if obj, err := t.r2.NewObject(ctx, key); err == nil {
			_ = obj.Remove(ctx)
		}

		// Update tracking
		_, _ = t.db.Exec(`
			UPDATE file_tier SET tier = 2, is_r2 = FALSE, is_b2 = TRUE, b2_archived_at = ?
			WHERE key = ?
		`, time.Now().Unix(), key)

		// Update PocketBase tier info
		if t.pb != nil {
			if file, err := t.pb.GetFile(key); err == nil && file != nil {
				file.Tier = 2
				file.R2Key = ""
				file.B2Key = key
				t.pb.UpdateFile(file)
			}
		}

		log.Printf("Archived to B2: %s", key)
	}

	return nil
}

// EvictLocal removes least-recently-used files from local cache
func (t *TieredStorage) EvictLocal(ctx context.Context, targetSize int64) error {
	// Get files ordered by last access, oldest first
	rows, err := t.db.Query(`
		SELECT key, size FROM file_tier
		WHERE is_local = TRUE AND is_r2 = TRUE
		ORDER BY last_accessed ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var currentSize int64
	_ = t.db.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM file_tier WHERE is_local = TRUE`).Scan(&currentSize)

	for rows.Next() && currentSize > targetSize {
		var key string
		var size int64
		if err := rows.Scan(&key, &size); err != nil {
			continue
		}

		// Delete local copy (it's still in R2)
		if obj, err := t.local.NewObject(ctx, key); err == nil {
			if err := obj.Remove(ctx); err == nil {
				_, _ = t.db.Exec(`UPDATE file_tier SET is_local = FALSE WHERE key = ?`, key)
				currentSize -= size
				log.Printf("Evicted from local: %s (%d bytes)", key, size)
			}
		}
	}

	return nil
}

func (t *TieredStorage) updateAccess(key string) {
	_, _ = t.db.Exec(`UPDATE file_tier SET last_accessed = ? WHERE key = ?`, time.Now().Unix(), key)
}

// SyncLocalToR2 syncs all local files to R2
func (t *TieredStorage) SyncLocalToR2(ctx context.Context) error {
	// List all local files
	entries, err := t.local.List(ctx, "")
	if err != nil {
		return fmt.Errorf("listing local: %w", err)
	}

	for _, entry := range entries {
		obj, ok := entry.(fs.Object)
		if !ok {
			continue // skip directories
		}

		key := obj.Remote()

		// Copy to R2
		if err := operations.CopyFile(ctx, t.r2, t.local, key, key); err != nil {
			log.Printf("Failed to sync %s: %v", key, err)
			continue
		}

		// Update tracking
		_, _ = t.db.Exec(`
			INSERT OR REPLACE INTO file_tier (key, size, is_local, is_r2, last_accessed, r2_synced_at)
			VALUES (?, ?, TRUE, TRUE, ?, ?)
		`, key, obj.Size(), time.Now().Unix(), time.Now().Unix())

		log.Printf("Synced: %s", key)
	}

	return nil
}

func (t *TieredStorage) updateTier(key string, tier int, local, r2, b2 bool) {
	_, _ = t.db.Exec(`
		INSERT OR REPLACE INTO file_tier (key, tier, is_local, is_r2, is_b2, last_accessed)
		VALUES (?, ?, ?, ?, ?, ?)
	`, key, tier, local, r2, b2, time.Now().Unix())
}

// Status returns tier statistics
func (t *TieredStorage) Status() map[string]interface{} {
	var localCount, r2Count, b2Count int64
	var localSize, r2Size, b2Size int64

	t.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size), 0) FROM file_tier WHERE is_local = TRUE`).Scan(&localCount, &localSize)
	t.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size), 0) FROM file_tier WHERE is_r2 = TRUE`).Scan(&r2Count, &r2Size)
	t.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size), 0) FROM file_tier WHERE is_b2 = TRUE`).Scan(&b2Count, &b2Size)

	return map[string]interface{}{
		"local": map[string]int64{"count": localCount, "size_bytes": localSize},
		"r2":    map[string]int64{"count": r2Count, "size_bytes": r2Size},
		"b2":    map[string]int64{"count": b2Count, "size_bytes": b2Size},
	}
}

func (t *TieredStorage) Close() {
	if t.nats != nil {
		t.nats.Close()
	}
	t.db.Close()
}

// HTTP handlers for proxy server
func (t *TieredStorage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path[1:] // Remove leading /
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		reader, err := t.Get(ctx, key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer reader.Close()
		io.Copy(w, reader)

	case http.MethodPut:
		err := t.Put(ctx, key, r.Body, r.ContentLength)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusCreated)

	case http.MethodDelete:
		// Delete from all tiers
		if obj, _ := t.local.NewObject(ctx, key); obj != nil {
			obj.Remove(ctx)
		}
		if obj, _ := t.r2.NewObject(ctx, key); obj != nil {
			obj.Remove(ctx)
		}
		if t.b2 != nil {
			if obj, _ := t.b2.NewObject(ctx, key); obj != nil {
				obj.Remove(ctx)
			}
		}
		t.db.Exec(`DELETE FROM file_tier WHERE key = ?`, key)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", 405)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: tiered <command>")
		fmt.Println("Commands: serve, sync, archive, status")
		os.Exit(1)
	}

	cfg := TierConfig{
		LocalPath:        os.Getenv("GARAGE_LOCAL_PATH"),
		LocalMaxSize:     10 * 1024 * 1024 * 1024, // 10GB default
		R2AccountID:      os.Getenv("R2_ACCOUNT_ID"),
		R2Bucket:         os.Getenv("R2_BUCKET"),
		R2AccessKey:      os.Getenv("R2_ACCESS_KEY"),
		R2SecretKey:      os.Getenv("R2_SECRET_KEY"),
		B2Account:        os.Getenv("B2_ACCOUNT"),
		B2Key:            os.Getenv("B2_KEY"),
		B2Bucket:         os.Getenv("B2_BUCKET"),
		ArchiveAfterDays: 30,
	}

	if cfg.LocalPath == "" {
		cfg.LocalPath = "./cache"
	}

	// Ensure local cache directory exists
	os.MkdirAll(cfg.LocalPath, 0755)

	storage, err := NewTieredStorage(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}
	defer storage.Close()

	switch os.Args[1] {
	case "serve":
		port := os.Getenv("TIERED_PORT")
		if port == "" {
			port = "8091"
		}
		log.Printf("Starting tiered storage proxy on :%s", port)
		log.Printf("  Local: %s", cfg.LocalPath)
		log.Printf("  R2: %s/%s", cfg.R2AccountID, cfg.R2Bucket)
		if cfg.B2Bucket != "" {
			log.Printf("  B2: %s", cfg.B2Bucket)
		}
		log.Fatal(http.ListenAndServe(":"+port, storage))

	case "sync":
		log.Println("Syncing local files to R2...")
		ctx := context.Background()
		if err := storage.SyncLocalToR2(ctx); err != nil {
			log.Fatalf("Sync failed: %v", err)
		}
		log.Println("Sync complete")

	case "archive":
		log.Println("Archiving old files to B2...")
		if err := storage.Archive(context.Background()); err != nil {
			log.Fatalf("Archive failed: %v", err)
		}
		log.Println("Archive complete")

	case "status":
		status := storage.Status()
		fmt.Println("=== GARAGE Tier Status ===")
		for tier, data := range status {
			d := data.(map[string]int64)
			fmt.Printf("  %s: %d files, %d bytes\n", tier, d["count"], d["size_bytes"])
		}

	default:
		log.Fatalf("Unknown command: %s", os.Args[1])
	}
}
