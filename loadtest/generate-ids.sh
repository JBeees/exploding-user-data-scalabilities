#!/bin/bash
# generate-ids.sh
# Ambil user ID asli dari DB dan simpan ke user_ids.json
# Jalankan ini SEBELUM k6 run scenario.js
#
# Usage: chmod +x generate-ids.sh && ./generate-ids.sh

set -e

OUTPUT_FILE="./user_ids.json"
DB_CONTAINER="plm_postgres"
DB_USER="plm_user"
DB_NAME="peakload_db"

echo "⏳ Mengambil user ID dari database..."

# Ambil user IDs dan format jadi JSON array
IDS=$(docker exec "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" \
  -t -A \
  -c "SELECT id FROM users WHERE status = 'active' ORDER BY RANDOM();")

# Konversi ke JSON array
echo "[" > "$OUTPUT_FILE"
FIRST=true
while IFS= read -r id; do
  [ -z "$id" ] && continue
  if [ "$FIRST" = true ]; then
    echo "  \"$id\"" >> "$OUTPUT_FILE"
    FIRST=false
  else
    echo "  ,\"$id\"" >> "$OUTPUT_FILE"
  fi
done <<< "$IDS"
echo "]" >> "$OUTPUT_FILE"

COUNT=$(grep -c '"' "$OUTPUT_FILE" || true)
echo "✅ Berhasil! $COUNT user ID disimpan ke $OUTPUT_FILE"
echo ""
echo "Sekarang jalankan:"
echo "  k6 run --env SCENARIO=baseline scenario.js"
