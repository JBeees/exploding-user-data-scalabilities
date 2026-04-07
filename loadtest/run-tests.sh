#!/bin/bash
# run-tests.sh — Jalankan semua skenario load test secara berurutan
# Usage: chmod +x run-tests.sh && ./run-tests.sh

set -e

BASE_URL=${BASE_URL:-"http://localhost:8080"}
OUTPUT_DIR="./results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

mkdir -p "$OUTPUT_DIR"

echo "=================================================="
echo "  Peak Load Management - Load Test Runner"
echo "  $(date)"
echo "  Target: $BASE_URL"
echo "=================================================="

# Cek API up dulu
echo ""
echo "🔍 Checking API health..."
HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/health")
if [ "$HEALTH" != "200" ]; then
  echo "❌ API tidak bisa diakses (HTTP $HEALTH). Jalankan 'docker compose up -d' dulu."
  exit 1
fi
echo "✅ API healthy"

# ─── Skenario 1: Baseline ────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 SKENARIO 1/4: BASELINE (50 VU, 60s)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
k6 run \
  --env SCENARIO=baseline \
  --env BASE_URL="$BASE_URL" \
  --out json="$OUTPUT_DIR/baseline_${TIMESTAMP}.json" \
  --summary-export="$OUTPUT_DIR/baseline_summary_${TIMESTAMP}.json" \
  scenario.js
echo "✅ Baseline selesai. Tunggu 30 detik sebelum skenario berikutnya..."
sleep 30

# ─── Skenario 2: Peak ─────────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 SKENARIO 2/4: PEAK LOAD (300 VU, 120s)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
k6 run \
  --env SCENARIO=peak \
  --env BASE_URL="$BASE_URL" \
  --out json="$OUTPUT_DIR/peak_${TIMESTAMP}.json" \
  --summary-export="$OUTPUT_DIR/peak_summary_${TIMESTAMP}.json" \
  scenario.js
echo "✅ Peak selesai. Tunggu 30 detik..."
sleep 30

# ─── Skenario 3: Spike ────────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 SKENARIO 3/4: SPIKE (50→500→50 VU)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
k6 run \
  --env SCENARIO=spike \
  --env BASE_URL="$BASE_URL" \
  --out json="$OUTPUT_DIR/spike_${TIMESTAMP}.json" \
  --summary-export="$OUTPUT_DIR/spike_summary_${TIMESTAMP}.json" \
  scenario.js
echo "✅ Spike selesai. Tunggu 30 detik..."
sleep 30

# ─── Skenario 4: Stress ───────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 SKENARIO 4/4: STRESS (ramp up ke 1000 VU)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
k6 run \
  --env SCENARIO=stress \
  --env BASE_URL="$BASE_URL" \
  --out json="$OUTPUT_DIR/stress_${TIMESTAMP}.json" \
  --summary-export="$OUTPUT_DIR/stress_summary_${TIMESTAMP}.json" \
  scenario.js

# ─── Summary ──────────────────────────────────────────────────────────────────
echo ""
echo "=================================================="
echo "  ✅ SEMUA SKENARIO SELESAI"
echo "  📁 Hasil tersimpan di: $OUTPUT_DIR/"
echo "  📊 Grafana: http://localhost:3000"
echo "=================================================="
echo ""
echo "File hasil:"
ls -lh "$OUTPUT_DIR/"*"${TIMESTAMP}"* 2>/dev/null || echo "(tidak ada file output)"
