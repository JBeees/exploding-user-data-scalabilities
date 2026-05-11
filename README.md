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

---

## Server Infrastructure & Public Observability

This system is designed to run on a Linux-based server (Ubuntu Server CLI recommended). It utilizes **CasaOS** as a web-based management interface and **Cloudflare Tunnel** to securely expose monitoring dashboards and APIs via HTTPS without opening local router ports (Zero Trust Network Access).

### 1. CasaOS Installation (Server Management)
CasaOS is used to monitor server hardware status and manage Docker containers effortlessly.
Run the following command on your Ubuntu Server terminal:

```bash
curl -fsSL [https://get.casaos.io](https://get.casaos.io) | sudo bash
Once the installation is complete, CasaOS can be accessed locally via a web browser at [IP_ADDRESS]
````
---

## Public Network Configuration (Cloudflare Tunnel)
Note: The cukurukuk.web.id domain is exclusively configured on our team's Primary Node server. The steps below serve as a guide if you wish to replicate the tunneling architecture using your own domain.

##### Download and install the cloudflared CLI
curl -L --output cloudflared.deb [https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb](https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb)
sudo dpkg -i cloudflared.deb

#### Log in to your Cloudflare account
cloudflared tunnel login

#### Create a new tunnel
cloudflared tunnel create <your-tunnel-name>

#### Route DNS to your domain
cloudflared tunnel route dns <your-tunnel-name> api.your-domain.com

--- 
## Load Testing Guide (K6) 

### Load testing can be executed in two different scenarios:

#### Public Functionality & Security Testing
Point the target URL in the scenario.js file to the encrypted public domain (e.g., https://api.cukurukuk.web.id). Note: This will introduce CPU overhead on the server due to the tunneling encryption.

#### Pure Scalability Testing (Recommended)
Point the target URL in the scenario.js file directly to the server's Private IP (e.g., http://192.168.x.x:8080) within the same WiFi/LAN network. This bypasses the external network encryption bottleneck, yielding pure application performance metrics.


