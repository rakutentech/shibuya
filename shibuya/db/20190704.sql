use shibuya;

CREATE TABLE IF NOT EXISTS plan_data (
    filename varchar(191) NOT NULL,
    plan_id INT UNSIGNED NOT NULL,
    PRIMARY KEY (plan_id, filename)
)CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS plan_test_file (
    filename varchar(191) NOT NULL,
    plan_id INT UNSIGNED PRIMARY KEY
)CHARSET=utf8mb4;