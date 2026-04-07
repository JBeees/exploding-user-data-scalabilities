import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { SharedArray } from 'k6/data';

// ─── Load User IDs dari file JSON ────────────────────────────────────────────
// Generate dulu dengan: ./generate-ids.sh
const USER_IDS = new SharedArray('userIDs', function () {
  return JSON.parse(open('./user_ids.json'));
});

// ─── Custom Metrics ──────────────────────────────────────────────────────────
const errorRate        = new Rate('error_rate');
const cacheHitRate     = new Rate('cache_hit_rate');
const txnCreateLatency = new Trend('txn_create_latency', true);
const txnGetLatency    = new Trend('txn_get_latency', true);
const balanceLatency   = new Trend('balance_latency', true);
const rateLimitCount   = new Counter('rate_limit_rejected');

// ─── Test Configuration ──────────────────────────────────────────────────────
const SCENARIO = __ENV.SCENARIO || 'baseline';
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

const scenarios = {
  // Skenario 1: Baseline — traffic normal, ukur performa awal
  baseline: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '30s', target: 50  },
      { duration: '60s', target: 50  },
      { duration: '20s', target: 0   },
    ],
    gracefulRampDown: '10s',
  },

  // Skenario 2: Peak — simulasi lonjakan 1 juta transaksi/jam (~278 TPS)
  peak: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '30s', target: 100  },
      { duration: '60s', target: 300  },
      { duration: '120s', target: 300 },
      { duration: '30s', target: 0    },
    ],
    gracefulRampDown: '15s',
  },

  // Skenario 3: Stress — dorong sampai sistem mulai error
  stress: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '30s', target: 100  },
      { duration: '30s', target: 300  },
      { duration: '30s', target: 600  },
      { duration: '30s', target: 1000 },
      { duration: '60s', target: 1000 },
      { duration: '30s', target: 0    },
    ],
    gracefulRampDown: '15s',
  },

  // Skenario 4: Spike — lonjakan tiba-tiba
  spike: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '20s', target: 50   },
      { duration: '5s',  target: 500  },
      { duration: '30s', target: 500  },
      { duration: '5s',  target: 50   },
      { duration: '30s', target: 50   },
      { duration: '10s', target: 0    },
    ],
    gracefulRampDown: '10s',
  },
};

export const options = {
  scenarios: {
    [SCENARIO]: scenarios[SCENARIO],
  },

  // SLO Thresholds
  thresholds: {
    'http_req_duration': [
      'p(50)<200',
      'p(95)<500',
      'p(99)<2000',
    ],
    'error_rate':         ['rate<0.01'],
    'http_req_failed':    ['rate<0.01'],
    'txn_create_latency': ['p(95)<800'],
    'txn_get_latency':    ['p(95)<300'],
    'balance_latency':    ['p(95)<200'],
    'cache_hit_rate':     ['rate>0.5'],
  },
};

// ─── Shared state untuk simpan transaction IDs ───────────────────────────────
let transactionIDs = [];

// ─── Setup ───────────────────────────────────────────────────────────────────
export function setup() {
  const health = http.get(`${BASE_URL}/health`);
  if (health.status !== 200) {
    console.error('❌ API tidak bisa diakses! Pastikan docker compose up sudah jalan.');
  }

  console.log(`✅ API ready. Menjalankan skenario: ${SCENARIO}`);
  console.log(`👥 Loaded ${USER_IDS.length} user IDs dari user_ids.json`);
  console.log(`🎯 SLO Target: p95 < 500ms, error rate < 1%`);

  return { scenario: SCENARIO };
}

// ─── Main Test Function ──────────────────────────────────────────────────────
export default function (data) {
  // Distribusi traffic: 20% write, 40% read txn, 40% read balance
  const rand = Math.random();

  if (rand < 0.20) {
    createTransaction();
  } else if (rand < 0.60) {
    getTransactionStatus();
  } else {
    getUserBalance();
  }

  sleep(Math.random() * 0.5 + 0.1);
}

// ─── Scenario Functions ──────────────────────────────────────────────────────

function createTransaction() {
  const userID = getRandomUserID();
  const payload = JSON.stringify({
    user_id: userID,
    type: randomChoice(['credit', 'debit', 'transfer']),
    amount: Math.floor(Math.random() * 1000000) + 10000,
    description: `Load test - VU ${__VU} iter ${__ITER}`,
  });

  const start = Date.now();
  const res = http.post(`${BASE_URL}/transactions`, payload, {
    headers: { 'Content-Type': 'application/json' },
    timeout: '10s',
    tags: { endpoint: 'create_transaction' },
  });
  txnCreateLatency.add(Date.now() - start);

  const success = check(res, {
    'POST /transactions: status 202': (r) => r.status === 202,
    'POST /transactions: has transaction id': (r) => {
      try { return JSON.parse(r.body).id !== undefined; } catch { return false; }
    },
    'POST /transactions: latency < 1s': (r) => r.timings.duration < 1000,
  });

  if (res.status === 429) rateLimitCount.add(1);
  errorRate.add(!success);

  // Simpan transaction ID untuk dipakai di getTransactionStatus
  if (res.status === 202) {
    try {
      const body = JSON.parse(res.body);
      if (body.id) {
        transactionIDs.push(body.id);
        if (transactionIDs.length > 500) transactionIDs.shift();
      }
    } catch (e) {}
  }
}

function getTransactionStatus() {
  // Pakai transaction ID yang sudah dibuat, skip jika belum ada
  if (transactionIDs.length === 0) {
    createTransaction();
    return;
  }

  const txnID = transactionIDs[Math.floor(Math.random() * transactionIDs.length)];

  const start = Date.now();
  const res = http.get(`${BASE_URL}/transactions/${txnID}`, {
    timeout: '5s',
    tags: { endpoint: 'get_transaction' },
  });
  txnGetLatency.add(Date.now() - start);

  check(res, {
    'GET /transactions/:id: status 200 or 404': (r) => r.status === 200 || r.status === 404,
    'GET /transactions/:id: latency < 500ms': (r) => r.timings.duration < 500,
  });

  if (res.status === 200) {
    cacheHitRate.add(res.headers['X-Cache'] === 'HIT' ? 1 : 0);
  }

  errorRate.add(res.status >= 500);
}

function getUserBalance() {
  const userID = getRandomUserID();

  const start = Date.now();
  const res = http.get(`${BASE_URL}/users/${userID}/balance`, {
    timeout: '5s',
    tags: { endpoint: 'get_balance' },
  });
  balanceLatency.add(Date.now() - start);

  check(res, {
    'GET /users/:id/balance: status 200 or 404': (r) => r.status === 200 || r.status === 404,
    'GET /users/:id/balance: latency < 300ms': (r) => r.timings.duration < 300,
  });

  if (res.status === 200) {
    cacheHitRate.add(res.headers['X-Cache'] === 'HIT' ? 1 : 0);
  }

  errorRate.add(res.status >= 500);
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function getRandomUserID() {
  return USER_IDS[Math.floor(Math.random() * USER_IDS.length)];
}

function randomChoice(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

// ─── Teardown ────────────────────────────────────────────────────────────────
export function teardown(data) {
  console.log(`\n✅ Test selesai: skenario ${data.scenario}`);
  console.log(`📊 Lihat hasil lengkap di Grafana: http://localhost:3000`);
}
