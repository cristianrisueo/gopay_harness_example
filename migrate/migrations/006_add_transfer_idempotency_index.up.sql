CREATE UNIQUE INDEX CONCURRENTLY uq_transfers_sender_idempotency_key ON transfers (sender_id, idempotency_key);
