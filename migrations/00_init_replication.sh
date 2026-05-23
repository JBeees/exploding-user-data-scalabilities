#!/bin/bash
set -e

# Tambahkan baris konfigurasi replikasi untuk replica_user di pg_hba.conf
echo "host replication replica_user 0.0.0.0/0 trust" >> "$PGDATA/pg_hba.conf"

# Muat ulang konfigurasi PostgreSQL untuk menerapkan perubahan
pg_ctl reload
