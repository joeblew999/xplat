// Package pbclient - NATS integration for real-time sync events
//
// PocketBase-HA embeds NATS, so we connect directly to it for:
//   - Publishing file change events from this device
//   - Subscribing to changes from other devices
//
// Subjects:
//   garage.files.changed.<device>  - A device changed a file
//   garage.files.synced.<device>   - A device finished syncing to R2
//   garage.sync.request            - Request sync from all devices

package pbclient

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSClient handles real-time sync events via NATS
type NATSClient struct {
	conn       *nats.Conn
	deviceName string
	handlers   map[string]func(*FileEvent)
}

// FileEvent represents a file change event
type FileEvent struct {
	Type       string `json:"type"` // "created", "updated", "deleted", "synced"
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	Version    int    `json:"version"`
	DeviceName string `json:"device_name"`
	Timestamp  int64  `json:"timestamp"`
}

// NewNATSClient creates a NATS client connected to PocketBase-HA's embedded NATS
func NewNATSClient(natsURL, deviceName string) (*NATSClient, error) {
	opts := []nats.Option{
		nats.Name(fmt.Sprintf("garage-%s", deviceName)),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1), // Unlimited reconnects
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Printf("NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("NATS reconnected to %s", nc.ConnectedUrl())
		}),
	}

	conn, err := nats.Connect(natsURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	return &NATSClient{
		conn:       conn,
		deviceName: deviceName,
		handlers:   make(map[string]func(*FileEvent)),
	}, nil
}

// PublishFileChanged announces a file change to other devices
func (n *NATSClient) PublishFileChanged(path, hash string, size int64, version int) error {
	event := FileEvent{
		Type:       "changed",
		Path:       path,
		Hash:       hash,
		Size:       size,
		Version:    version,
		DeviceName: n.deviceName,
		Timestamp:  time.Now().Unix(),
	}
	return n.publish(fmt.Sprintf("garage.files.changed.%s", n.deviceName), event)
}

// PublishFileSynced announces that a file was synced to R2
func (n *NATSClient) PublishFileSynced(path, hash string, r2Key string) error {
	event := FileEvent{
		Type:       "synced",
		Path:       path,
		Hash:       hash,
		DeviceName: n.deviceName,
		Timestamp:  time.Now().Unix(),
	}
	return n.publish(fmt.Sprintf("garage.files.synced.%s", n.deviceName), event)
}

// PublishFileDeleted announces a file deletion
func (n *NATSClient) PublishFileDeleted(path string) error {
	event := FileEvent{
		Type:       "deleted",
		Path:       path,
		DeviceName: n.deviceName,
		Timestamp:  time.Now().Unix(),
	}
	return n.publish(fmt.Sprintf("garage.files.changed.%s", n.deviceName), event)
}

// SubscribeToChanges subscribes to file changes from all other devices
func (n *NATSClient) SubscribeToChanges(handler func(*FileEvent)) error {
	// Subscribe to all device changes using wildcard
	_, err := n.conn.Subscribe("garage.files.changed.*", func(msg *nats.Msg) {
		var event FileEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("NATS: failed to decode event: %v", err)
			return
		}

		// Ignore our own events
		if event.DeviceName == n.deviceName {
			return
		}

		handler(&event)
	})
	return err
}

// SubscribeToSyncs subscribes to sync completion events
func (n *NATSClient) SubscribeToSyncs(handler func(*FileEvent)) error {
	_, err := n.conn.Subscribe("garage.files.synced.*", func(msg *nats.Msg) {
		var event FileEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("NATS: failed to decode sync event: %v", err)
			return
		}

		if event.DeviceName == n.deviceName {
			return
		}

		handler(&event)
	})
	return err
}

// RequestSync sends a sync request to all devices
func (n *NATSClient) RequestSync() error {
	event := FileEvent{
		Type:       "sync_request",
		DeviceName: n.deviceName,
		Timestamp:  time.Now().Unix(),
	}
	return n.publish("garage.sync.request", event)
}

// SubscribeToSyncRequests subscribes to sync requests from other devices
func (n *NATSClient) SubscribeToSyncRequests(handler func(*FileEvent)) error {
	_, err := n.conn.Subscribe("garage.sync.request", func(msg *nats.Msg) {
		var event FileEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("NATS: failed to decode sync request: %v", err)
			return
		}

		if event.DeviceName == n.deviceName {
			return
		}

		handler(&event)
	})
	return err
}

func (n *NATSClient) publish(subject string, event FileEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return n.conn.Publish(subject, data)
}

// Close closes the NATS connection
func (n *NATSClient) Close() {
	n.conn.Close()
}

// IsConnected returns true if connected to NATS
func (n *NATSClient) IsConnected() bool {
	return n.conn.IsConnected()
}
