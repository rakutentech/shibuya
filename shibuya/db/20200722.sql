use shibuya;

CREATE TABLE IF NOT EXISTS collection_launch_history
(
    collection_id INT UNSIGNED NOT NULL,
    context varchar(20) NOT NULL,
    owner VARCHAR(50) NOT NULL,
    engines_count INT UNSIGNED,
    nodes_count INT UNSIGNED,
    started_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    end_time TIMESTAMP NULL DEFAULT NULL,
    key(collection_id, context, end_time)
)CHARSET=utf8mb4;