use shibuya;

CREATE TABLE IF NOT EXISTS collection_run_history (
    run_id INT UNSIGNED NOT NULL UNIQUE,
    collection_id INT UNSIGNED NOT NULL,
    started_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    end_time TIMESTAMP NULL DEFAULT NULL,
    key (collection_id, started_time),
    key (collection_id, run_id)
)CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS collection_run (
    id INT unsigned NOT NULL AUTO_INCREMENT PRIMARY KEY,
    collection_id INT unsigned NOT NULL UNIQUE
)CHARSET=utf8mb4;