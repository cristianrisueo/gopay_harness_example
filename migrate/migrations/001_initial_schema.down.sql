-- Drop indexes
DROP INDEX IF EXISTS idx_transfers_status;
DROP INDEX IF EXISTS idx_transfers_created_at;
DROP INDEX IF EXISTS idx_transfers_receiver_id;
DROP INDEX IF EXISTS idx_transfers_sender_id;
DROP INDEX IF EXISTS idx_wallets_user_id;
DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS idx_users_email;

-- Drop tables (in reverse order due to foreign keys)
DROP TABLE IF EXISTS transfers;
DROP TABLE IF EXISTS wallets;
DROP TABLE IF EXISTS users;

-- Drop extension
DROP EXTENSION IF EXISTS "uuid-ossp";
