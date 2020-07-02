use shibuya;
CREATE TABLE IF NOT EXISTS running_plan (
    collection_id INT UNSIGNED NOT NULL,
    plan_id INT UNSIGNED NOT NULL,
    url varchar(500) NOT NULL,
    UNIQUE (collection_id, plan_id)
) CHARSET=utf8mb4;
