# Exploding User Data Scalabilities

> Prototype architecture for **exploding user data scalabilities** based on CIMB Niaga case study.  
> Simulates handling a surge of 1 million transactions/hour that caused crashes in the legacy system.

**Team:** Cukurukuk — Universitas Brawijaya, Faculty of Computer Science 2026  
**Topic:** B.4 Exploding User Data Scalabilities

---

## Prerequisites

Ensure that Docker is installed.

```bash
docker --version
docker compose version
```

---

## Setup

### 1. Clone repository

```bash
git clone https://github.com/JBeees/exploding-user-data-scalabilities.git
cd exploding-user-data-scalabilities
```

### 2. Create configuration file

```bash
cp .env.example .env
```

Open `.env` and fill in the values:

### 3. Start all services

```bash
docker compose up -d postgres postgres_replica redis rabbitmq prometheus grafana
```

### 4. Check all services are running

```bash
docker compose ps
```

Expected output — all services `healthy`:

---

## Verify Installation

### Check database & seed data

```bash
docker exec -it plm_postgres psql -U plm_user -d peakload_db \
  -c "SELECT COUNT(*) FROM users; SELECT COUNT(*) FROM transactions;"
```

### Check Redis

```bash
docker exec -it plm_redis redis-cli ping
```

### Check RabbitMQ

```bash
docker exec -it plm_rabbitmq rabbitmq-diagnostics ping
```


