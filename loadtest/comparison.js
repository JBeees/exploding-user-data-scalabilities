import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { SharedArray } from 'k6/data';

// ─── Load User IDs ────────────────────────────────────────────────────────────
const USER_IDS = new SharedArray('userIDs', function () {
  return JSON.parse(open('./user_ids.json'));
});

// ─── Custom Metrics — V0 (tanpa optimasi) ────────────────────────────────────
const v0ErrorRate    = new Rate('v0_error_rate');
const v0CreateTrend  = new Trend('v0_txn_create_ms', true);
const v0GetTrend     = new Trend('v0_txn_get_ms', true);
const v0BalanceTrend = new Trend('v0_balance_ms', true);

// ─── Custom Metrics — V1 (dengan optimasi) ────────────────────────────────────
const v1ErrorRate    = new Rate('v1_error_rate');
const v1CreateTrend  = new Trend('v1_txn_create_ms', true);
const v1GetTrend     = new Trend('v1_txn_get_ms', true);
const v1BalanceTrend = new Trend('v1_balance_ms', true);
const v1CacheHit     = new Rate('v1_cache_hit_rate');
const v1RateLimit    = new Counter('v1_rate_limited');

// ─── Config ──────────────────────────────────────────────────────────────────
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const VUS      = parseInt(__ENV.VUS || '100');
const DURATION = __ENV.DURATION || '60s';

export const options = {
  scenarios: {
    // Jalankan kedua versi bersamaan dengan VU yang sama
    v0_no_optimization: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '20s', target: VUS },
        { duration: DURATION, target: VUS },
        { duration: '10s', target: 0 },
      ],
      gracefulRampDown: '10s',
      exec: 'testV0',
    },
    v1_with_optimization: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '20s', target: VUS },
        { duration: DURATION, target: VUS },
        { duration: '10s', target: 0 },
      ],
      gracefulRampDown: '10s',
      exec: 'testV1',
    },
  },

  thresholds: {
    // V1 harus jauh lebih baik dari V0
    'v1_error_rate':   ['rate<0.01'],   // error V1 < 1%
    'v1_balance_ms':   ['p(95)<200'],   // p95 V1 < 200ms
    'v1_txn_get_ms':   ['p(95)<300'],   // p95 V1 < 300ms
    'v1_cache_hit_rate': ['rate>0.5'],  // cache hit V1 > 50%
  },
};

// ─── Shared transaction IDs ───────────────────────────────────────────────────
let v0TxnIDs = [];
let v1TxnIDs = [];

// ─── Setup ───────────────────────────────────────────────────────────────────
export function setup() {
  const health = http.get(`${BASE_URL}/health`);
  if (health.status !== 200) {
    console.error('❌ API tidak bisa diakses!');
    return;
  }
  console.log(`✅ API ready`);
  console.log(`👥 ${USER_IDS.length} user IDs loaded`);
  console.log(`⚙️  VUs: ${VUS} | Duration: ${DURATION}`);
  console.log(`\n📊 Testing:`);
  console.log(`   V0 (no optimization): ${BASE_URL}/v0/...`);
  console.log(`   V1 (with optimization): ${BASE_URL}/...`);
}

// ─── V0: Test TANPA optimasi ─────────────────────────────────────────────────
export function testV0() {
  const rand = Math.random();

  if (rand < 0.20) {
    v0CreateTransaction();
  } else if (rand < 0.60) {
    v0GetTransaction();
  } else {
    v0GetBalance();
  }

  sleep(Math.random() * 0.5 + 0.1);
}

function v0CreateTransaction() {
  const userID = randomUser();
  const start = Date.now();

  const res = http.post(`${BASE_URL}/v0/transactions`,
    JSON.stringify({
      user_id: userID,
      type: randomChoice(['credit', 'debit', 'transfer']),
      amount: Math.floor(Math.random() * 500000) + 10000,
      description: `V0 load test - VU ${__VU}`,
    }),
    { headers: { 'Content-Type': 'application/json' }, timeout: '15s' }
  );
  v0CreateTrend.add(Date.now() - start);

  const ok = check(res, {
    'V0 POST /transactions: status 200': (r) => r.status === 200,
  });
  v0ErrorRate.add(!ok);

  if (res.status === 200) {
    try {
      const body = JSON.parse(res.body);
      if (body.id) {
        v0TxnIDs.push(body.id);
        if (v0TxnIDs.length > 200) v0TxnIDs.shift();
      }
    } catch (e) {}
  }
}

function v0GetTransaction() {
  if (v0TxnIDs.length === 0) { v0CreateTransaction(); return; }
  const txnID = v0TxnIDs[Math.floor(Math.random() * v0TxnIDs.length)];
  const start = Date.now();

  const res = http.get(`${BASE_URL}/v0/transactions/${txnID}`, { timeout: '10s' });
  v0GetTrend.add(Date.now() - start);

  const ok = check(res, {
    'V0 GET /transactions/:id: status 200 or 404': (r) => r.status === 200 || r.status === 404,
  });
  v0ErrorRate.add(res.status >= 500);
}

function v0GetBalance() {
  const start = Date.now();
  const res = http.get(`${BASE_URL}/v0/users/${randomUser()}/balance`, { timeout: '10s' });
  v0BalanceTrend.add(Date.now() - start);

  const ok = check(res, {
    'V0 GET /users/:id/balance: status 200 or 404': (r) => r.status === 200 || r.status === 404,
  });
  v0ErrorRate.add(res.status >= 500);
}

// ─── V1: Test DENGAN optimasi ────────────────────────────────────────────────
export function testV1() {
  const rand = Math.random();

  if (rand < 0.20) {
    v1CreateTransaction();
  } else if (rand < 0.60) {
    v1GetTransaction();
  } else {
    v1GetBalance();
  }

  sleep(Math.random() * 0.5 + 0.1);
}

function v1CreateTransaction() {
  const userID = randomUser();
  const start = Date.now();

  const res = http.post(`${BASE_URL}/transactions`,
    JSON.stringify({
      user_id: userID,
      type: randomChoice(['credit', 'debit', 'transfer']),
      amount: Math.floor(Math.random() * 500000) + 10000,
      description: `V1 load test - VU ${__VU}`,
    }),
    { headers: { 'Content-Type': 'application/json' }, timeout: '10s' }
  );
  v1CreateTrend.add(Date.now() - start);

  if (res.status === 429) v1RateLimit.add(1);

  const ok = check(res, {
    'V1 POST /transactions: status 202': (r) => r.status === 202,
  });
  v1ErrorRate.add(!ok && res.status !== 429); // 429 bukan error, itu proteksi

  if (res.status === 202) {
    try {
      const body = JSON.parse(res.body);
      if (body.id) {
        v1TxnIDs.push(body.id);
        if (v1TxnIDs.length > 200) v1TxnIDs.shift();
      }
    } catch (e) {}
  }
}

function v1GetTransaction() {
  if (v1TxnIDs.length === 0) { v1CreateTransaction(); return; }
  const txnID = v1TxnIDs[Math.floor(Math.random() * v1TxnIDs.length)];
  const start = Date.now();

  const res = http.get(`${BASE_URL}/transactions/${txnID}`, { timeout: '5s' });
  v1GetTrend.add(Date.now() - start);

  check(res, {
    'V1 GET /transactions/:id: status 200 or 404': (r) => r.status === 200 || r.status === 404,
  });

  if (res.status === 200) {
    v1CacheHit.add(res.headers['X-Cache'] === 'HIT' ? 1 : 0);
  }
  v1ErrorRate.add(res.status >= 500);
}

function v1GetBalance() {
  const start = Date.now();
  const res = http.get(`${BASE_URL}/users/${randomUser()}/balance`, { timeout: '5s' });
  v1BalanceTrend.add(Date.now() - start);

  check(res, {
    'V1 GET /users/:id/balance: status 200 or 404': (r) => r.status === 200 || r.status === 404,
  });

  if (res.status === 200) {
    v1CacheHit.add(res.headers['X-Cache'] === 'HIT' ? 1 : 0);
  }
  v1ErrorRate.add(res.status >= 500);
}

// ─── Helpers ─────────────────────────────────────────────────────────────────
function randomUser() {
  return USER_IDS[Math.floor(Math.random() * USER_IDS.length)];
}

function randomChoice(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

// ─── Teardown ─────────────────────────────────────────────────────────────────
export function teardown() {
  console.log('\n════════════════════════════════════════════');
  console.log('  HASIL PERBANDINGAN V0 vs V1');
  console.log('════════════════════════════════════════════');
  console.log('Lihat detail di output terminal di atas.');
  console.log('Cek Grafana: http://localhost:3000');
  console.log('\nMetrics yang dibandingkan:');
  console.log('  v0_balance_ms    vs  v1_balance_ms');
  console.log('  v0_txn_get_ms    vs  v1_txn_get_ms');
  console.log('  v0_txn_create_ms vs  v1_txn_create_ms');
  console.log('  v0_error_rate    vs  v1_error_rate');
  console.log('════════════════════════════════════════════\n');
}
