#!/bin/bash
# Create PocketBase collections for GARAGE multi-device sync

set -e

PBHA_URL="${PBHA_URL:-http://localhost:8090}"
PBHA_ADMIN_EMAIL="${PBHA_ADMIN_EMAIL:-admin@garage.local}"
PBHA_ADMIN_PASS="${PBHA_ADMIN_PASS:-admin123456}"

echo "=== GARAGE Collection Setup ==="
echo "PocketBase URL: $PBHA_URL"

# Get auth token
echo ""
echo "Authenticating..."
AUTH_RESPONSE=$(curl -s -X POST "$PBHA_URL/api/collections/_superusers/auth-with-password" \
  -H "Content-Type: application/json" \
  -d "{\"identity\": \"$PBHA_ADMIN_EMAIL\", \"password\": \"$PBHA_ADMIN_PASS\"}")

TOKEN=$(echo "$AUTH_RESPONSE" | jq -r '.token')
if [ "$TOKEN" = "null" ] || [ -z "$TOKEN" ]; then
  echo "ERROR: Failed to authenticate"
  echo "$AUTH_RESPONSE"
  exit 1
fi
echo "OK: Authenticated"

# Helper function to create collection
create_collection() {
  local name=$1
  local json=$2

  echo ""
  echo "Creating collection: $name"

  # Check if exists
  EXISTS=$(curl -s "$PBHA_URL/api/collections/$name" -H "Authorization: $TOKEN" | jq -r '.id // empty')
  if [ -n "$EXISTS" ]; then
    echo "  Already exists (id: $EXISTS)"
    return 0
  fi

  RESULT=$(curl -s -X POST "$PBHA_URL/api/collections" \
    -H "Content-Type: application/json" \
    -H "Authorization: $TOKEN" \
    -d "$json")

  ID=$(echo "$RESULT" | jq -r '.id // empty')
  if [ -n "$ID" ]; then
    echo "  Created (id: $ID)"
  else
    echo "  ERROR: $(echo "$RESULT" | jq -r '.message // "Unknown error"')"
    echo "  $RESULT"
  fi
}

# 1. Devices collection - tracks which devices a user has
create_collection "devices" '{
  "name": "devices",
  "type": "base",
  "fields": [
    {"name": "device_name", "type": "text", "required": true},
    {"name": "device_type", "type": "text"},
    {"name": "platform", "type": "text"},
    {"name": "last_seen", "type": "date"},
    {"name": "is_online", "type": "bool"}
  ]
}'

# 2. Garage files collection - file metadata (NOT bytes)
create_collection "garage_files" '{
  "name": "garage_files",
  "type": "base",
  "fields": [
    {"name": "path", "type": "text", "required": true},
    {"name": "filename", "type": "text", "required": true},
    {"name": "size", "type": "number"},
    {"name": "hash", "type": "text"},
    {"name": "mime_type", "type": "text"},
    {"name": "tier", "type": "number"},
    {"name": "r2_key", "type": "text"},
    {"name": "b2_key", "type": "text"},
    {"name": "current_version", "type": "number"},
    {"name": "is_deleted", "type": "bool"}
  ]
}'

# 3. File versions - for conflict tracking
create_collection "file_versions" '{
  "name": "file_versions",
  "type": "base",
  "fields": [
    {"name": "file_path", "type": "text", "required": true},
    {"name": "version_num", "type": "number", "required": true},
    {"name": "device_name", "type": "text"},
    {"name": "size", "type": "number"},
    {"name": "hash", "type": "text"},
    {"name": "r2_key", "type": "text"},
    {"name": "is_conflict", "type": "bool"}
  ]
}'

# 4. Device file cache - which device has which file locally
create_collection "device_cache" '{
  "name": "device_cache",
  "type": "base",
  "fields": [
    {"name": "device_name", "type": "text", "required": true},
    {"name": "file_path", "type": "text", "required": true},
    {"name": "version_num", "type": "number"},
    {"name": "local_path", "type": "text"},
    {"name": "is_dirty", "type": "bool"},
    {"name": "last_synced", "type": "date"}
  ]
}'

echo ""
echo "=== Collection Setup Complete ==="
echo ""
echo "Collections created:"
curl -s "$PBHA_URL/api/collections" -H "Authorization: $TOKEN" | jq -r '.items[] | "  - \(.name) (\(.id))"'
